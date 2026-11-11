package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"marquee/internal/store"
)

// Worker processes media one at a time: probing and thumbnails for uploads,
// ffmpeg re-encoding for conversions. One at a time keeps a busy ffmpeg from
// starving the HTTP server on a small machine.
type Worker struct {
	store   *store.Store
	storage *Storage
	tools   Tools
	log     *slog.Logger
	queue   chan int64
	options func(context.Context) Options
}

func NewWorker(st *store.Store, storage *Storage, tools Tools, log *slog.Logger) *Worker {
	return &Worker{store: st, storage: storage, tools: tools, log: log, queue: make(chan int64, 10000),
		options: func(context.Context) Options { return DefaultOptions() }}
}

func (w *Worker) Tools() Tools { return w.tools }

// SetOptions makes the worker read the processing options from f for every
// file, so a change in the settings applies to the next upload.
func (w *Worker) SetOptions(f func(context.Context) Options) { w.options = f }

// Enqueue schedules a media item; it never blocks the caller.
func (w *Worker) Enqueue(id int64) {
	select {
	case w.queue <- id:
	default:
		w.log.Warn("Опашката за обработка е пълна; файлът ще се обработи след рестарт", "файл", id)
	}
}

// Run resumes items left "processing" by a previous run, then serves the queue.
func (w *Worker) Run(ctx context.Context) {
	if pending, err := w.store.MediaByStatus(ctx, "processing"); err == nil {
		for _, m := range pending {
			w.Enqueue(m.ID)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-w.queue:
			if err := w.process(ctx, id); err != nil && !errors.Is(err, context.Canceled) {
				w.log.Error("Файлът не можа да се обработи", "файл", id, "грешка", err)
				w.store.MarkMediaFailed(context.Background(), id, err.Error())
			}
		}
	}
}

func (w *Worker) process(ctx context.Context, id int64) error {
	m, err := w.store.Media(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if m.Status != "processing" {
		return nil
	}
	if m.SHA256 == "" && m.Recipe != "" {
		return w.render(ctx, m)
	}
	if m.SHA256 == "" && m.SourceID.Valid {
		return w.convert(ctx, m)
	}
	return w.inspect(ctx, m, true)
}

// inspect probes the item's file and marks it ready. For a fresh upload
// (prepare) it first applies the processing options: images larger than the
// screen are shrunk, videos are converted in place when the settings say so.
func (w *Worker) inspect(ctx context.Context, m *store.Media, prepare bool) error {
	path := w.storage.Path(m.SHA256, m.Ext)
	upload := prepare && !m.SourceID.Valid && m.Recipe == ""
	opts := w.optionsFor(ctx, m)

	switch m.Kind {
	case "image":
		wd, ht, err := ImageSize(path)
		if err != nil {
			return fmt.Errorf("Файлът не е валидно изображение: %v", err)
		}
		if upload && !opts.fits(wd, ht) && w.tools.FFmpeg != "" {
			old := *m
			if err := w.shrinkImage(ctx, m, opts); err != nil {
				w.log.Warn("Снимката не можа да се смали; остава оригиналът", "снимка", m.Name, "грешка", err)
				return w.inspectWith(ctx, m)
			}
			note := Warning{Level: "info", Code: "resized",
				Message: fmt.Sprintf("Смалена от %d×%d (%s), за да се побере в екран %d×%d.", wd, ht, formatSize(old.Size), opts.ImageW, opts.ImageH)}
			if err := w.inspectWith(ctx, m, note); err != nil {
				return err
			}
			w.log.Info("Снимката е смалена", "снимка", m.Name, "беше", fmt.Sprintf("%d×%d", wd, ht))
			w.dropUnused(ctx, &old)
			return nil
		}
		return w.inspectWith(ctx, m)
	case "video":
		if upload && opts.VideoAuto != "off" && w.tools.FFmpeg != "" {
			r, err := w.tools.Probe(ctx, path)
			if err == nil && (opts.VideoAuto == "always" || Convertible("video", CheckVideo(m.Ext, r))) {
				w.log.Info("Автоматично конвертиране на видео", "видео", m.Name)
				out, err := w.encode(ctx, path, r.Height > r.Width, opts)
				if err != nil {
					return fmt.Errorf("автоматичното конвертиране не успя: %w", err)
				}
				defer os.Remove(out)
				old := *m
				if err := w.replace(m, out, "mp4"); err != nil {
					return err
				}
				note := Warning{Level: "info", Code: "converted",
					Message: fmt.Sprintf("Конвертирано автоматично за плейърите (оригинал: %s, %s).", strings.ToUpper(old.Ext), formatSize(old.Size))}
				if err := w.inspectWith(ctx, m, note); err != nil {
					return err
				}
				w.dropUnused(ctx, &old)
				return nil
			}
		}
	}
	return w.inspectWith(ctx, m)
}

// optionsFor is how m is prepared: the choice made when it was uploaded,
// else the hub settings.
func (w *Worker) optionsFor(ctx context.Context, m *store.Media) Options {
	if m.Prep != "" {
		var p Prep
		if err := json.Unmarshal([]byte(m.Prep), &p); err == nil {
			if o, err := p.Options(); err == nil {
				return o
			}
		}
		w.log.Warn("Невалиден избор за обработка; ползват се настройките на хъба", "файл", m.Name)
	}
	return w.options(ctx)
}

// shrinkImage fits the item's image into the screen box and makes the
// smaller file its content.
func (w *Worker) shrinkImage(ctx context.Context, m *store.Media, opts Options) error {
	dir, err := w.storage.TempDir("image-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	ext := m.Ext
	if ext == "webp" || ext == "jpeg" {
		ext = "jpg" // WebP is not safe on every player
	}
	out := filepath.Join(dir, "out."+ext)
	if err := w.ffmpeg(ctx, opts.imageArgs(w.storage.Path(m.SHA256, m.Ext), out)); err != nil {
		return err
	}
	return w.replace(m, out, ext)
}

// replace makes the finished file at out the item's content (in memory; the
// row changes when the item is marked ready).
func (w *Worker) replace(m *store.Media, out, ext string) error {
	sha, size, err := w.storage.Adopt(out, ext)
	if err != nil {
		return err
	}
	m.SHA256, m.Size, m.Ext = sha, size, ext
	return nil
}

// dropUnused removes a replaced blob unless another item still serves it.
func (w *Worker) dropUnused(ctx context.Context, old *store.Media) {
	if used, err := w.store.BlobInUse(ctx, old.SHA256, old.Ext); err == nil && !used {
		w.storage.Remove(old.SHA256, old.Ext)
	}
}

func formatSize(n int64) string {
	if n >= 1<<20 {
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%d KB", n>>10)
}

// inspectWith probes the file as it is now and marks the item ready, with
// notes (e.g. "shrunk") in front of the compatibility warnings.
func (w *Worker) inspectWith(ctx context.Context, m *store.Media, notes ...Warning) error {
	path := w.storage.Path(m.SHA256, m.Ext)
	p := store.MediaProbe{SHA256: m.SHA256, Ext: m.Ext, Size: m.Size}
	var warnings []Warning

	switch m.Kind {
	case "image":
		wd, ht, err := ImageSize(path)
		if err != nil {
			return fmt.Errorf("Файлът не е валидно изображение: %v", err)
		}
		p.Width, p.Height = wd, ht
		warnings = CheckImage(m.Ext, wd, ht)
		p.HasThumb = w.thumb(m.SHA256, func(dst string) error { return ImageThumb(path, dst) })
	case "video", "audio":
		r, err := w.tools.Probe(ctx, path)
		if err != nil {
			if w.tools.FFprobe == "" {
				warnings = append(warnings, Warning{Level: "warn", Code: "noffmpeg",
					Message: "ffmpeg не е наличен на сървъра: файлът не е проверен за съвместимост."})
				break
			}
			return fmt.Errorf("Файлът не може да се прочете: %v", err)
		}
		p.Width, p.Height, p.DurationMS = r.Width, r.Height, r.DurationMS
		p.VideoCodec, p.AudioCodec, p.FPS, p.Bitrate = r.VideoCodec, r.AudioCodec, r.FPS, r.Bitrate
		if m.Kind == "video" {
			warnings = CheckVideo(m.Ext, r)
			p.HasThumb = w.thumb(m.SHA256, func(dst string) error {
				return w.tools.VideoThumb(ctx, path, dst, r.DurationMS)
			})
		} else {
			warnings = CheckAudio(m.Ext, r)
		}
	}
	warnings = append(append([]Warning{}, notes...), warnings...)
	js, _ := json.Marshal(warnings)
	p.Warnings = string(js)
	return w.store.MarkMediaReady(ctx, m.ID, p)
}

// thumb creates the thumbnail unless an identical blob already has one.
func (w *Worker) thumb(sha string, create func(dst string) error) bool {
	dst := w.storage.ThumbPath(sha)
	if _, err := os.Stat(dst); err == nil {
		return true
	}
	if err := create(dst); err != nil {
		w.log.Warn("Миниатюрата не се създаде", "sha", sha[:12], "грешка", err)
		return false
	}
	return true
}

// convert re-encodes the source video to H.264/AAC MP4 within 1080p30, the
// format every Android 7 player handles, then inspects the result.
func (w *Worker) convert(ctx context.Context, m *store.Media) error {
	if w.tools.FFmpeg == "" {
		return fmt.Errorf("ffmpeg не е наличен на сървъра")
	}
	src, err := w.store.Media(ctx, m.SourceID.Int64)
	if err != nil {
		return fmt.Errorf("изходният файл липсва: %w", err)
	}
	w.log.Info("Конвертиране на видео", "видео", src.Name, "резултат", m.Name)
	out, err := w.encode(ctx, w.storage.Path(src.SHA256, src.Ext), src.Height > src.Width, w.optionsFor(ctx, src))
	if err != nil {
		return fmt.Errorf("конвертирането не успя: %w", err)
	}
	defer os.Remove(out)
	return w.adopt(ctx, m, out)
}

// encode converts a video for the players into a temporary MP4 (see
// Options.videoArgs); the caller removes or adopts it.
func (w *Worker) encode(ctx context.Context, in string, portrait bool, opts Options) (string, error) {
	tmp, err := w.storage.TempFile("convert-*.mp4")
	if err != nil {
		return "", err
	}
	out := tmp.Name()
	tmp.Close()
	if err := w.ffmpeg(ctx, opts.videoArgs(in, out, portrait)); err != nil {
		os.Remove(out)
		return "", err
	}
	return out, nil
}

// render turns the item's slideshow recipe into an MP4.
func (w *Worker) render(ctx context.Context, m *store.Media) error {
	if w.tools.FFmpeg == "" {
		return fmt.Errorf("ffmpeg не е наличен на сървъра")
	}
	sh, err := ParseSlideshow(m.Recipe)
	if err == nil {
		err = sh.Normalize()
	}
	if err != nil {
		return fmt.Errorf("невалидна рецепта: %v", err)
	}
	file := func(id int64, kind string) (string, error) {
		src, err := w.store.Media(ctx, id)
		if err != nil || src.Kind != kind || src.Status != "ready" {
			return "", fmt.Errorf("файл #%d липсва или още не е готов", id)
		}
		// ffmpeg runs in the job directory, so relative paths would break.
		return filepath.Abs(w.storage.Path(src.SHA256, src.Ext))
	}
	images := make([]string, len(sh.Items))
	for i, it := range sh.Items {
		if images[i], err = file(it.MediaID, "image"); err != nil {
			return err
		}
	}
	audio := ""
	if sh.AudioID != 0 {
		if audio, err = file(sh.AudioID, "audio"); err != nil {
			return err
		}
	}

	dir, err := w.storage.TempDir("render-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "out.mp4")
	job := sh.Job(images, audio, out)
	for name, content := range job.Files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	if sh.HasText() {
		font, err := FindFont(Fonts[sh.Text.Font])
		if err != nil {
			return err
		}
		if err := copyFile(font, filepath.Join(dir, SlideshowFont)); err != nil {
			return err
		}
	}
	w.log.Info("Правене на видео от снимки", "видео", m.Name, "снимки", len(images), "секунди", sh.TotalS())
	cmd := command(ctx, w.tools.FFmpeg, job.Args...)
	cmd.Dir = dir
	if err := run(cmd); err != nil {
		return fmt.Errorf("видеото не можа да се направи: %w", err)
	}
	if n := sh.Loops(); n > 0 {
		// Repeat by copying the encoded stream: no quality loss, takes seconds.
		looped := filepath.Join(dir, "looped.mp4")
		if err := w.ffmpeg(ctx, []string{"-v", "error", "-y", "-stream_loop", strconv.Itoa(n), "-i", out,
			"-map", "0", "-c", "copy", "-movflags", "+faststart", looped}); err != nil {
			return fmt.Errorf("повторението на видеото не успя: %w", err)
		}
		out = looped
	}
	return w.adopt(ctx, m, out)
}

func (w *Worker) ffmpeg(ctx context.Context, args []string) error {
	return run(command(ctx, w.tools.FFmpeg, args...))
}

// run executes cmd and folds the tail of its output into the error.
func run(cmd *exec.Cmd) error {
	outp, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(outp))
		if len(msg) > 500 {
			msg = msg[len(msg)-500:]
		}
		return fmt.Errorf("%v: %s", err, msg)
	}
	return nil
}

// adopt stores a freshly encoded MP4 as the item's content and inspects it.
func (w *Worker) adopt(ctx context.Context, m *store.Media, out string) error {
	if err := w.replace(m, out, "mp4"); err != nil {
		return err
	}
	return w.inspectWith(ctx, m)
}

// FindFont locates a caption font: tools/fonts next to the exe first, so a
// server without the Windows fonts can be given them, then the system fonts.
func FindFont(file string) (string, error) {
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Join(filepath.Dir(exe), "tools", "fonts"))
	}
	if win := os.Getenv("WINDIR"); win != "" {
		dirs = append(dirs, filepath.Join(win, "Fonts"))
	}
	dirs = append(dirs, "/usr/share/fonts/truetype/msttcorefonts")
	for _, d := range dirs {
		p := filepath.Join(d, file)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("шрифтът %s не е намерен (сложете го в tools\\fonts)", file)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
