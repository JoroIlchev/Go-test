package media

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// Tools locates ffmpeg/ffprobe: an explicit dir, then tools\ next to the
// executable, then PATH. Empty paths mean the tool is unavailable.
type Tools struct {
	FFmpeg  string
	FFprobe string
}

func FindTools(dir string) Tools {
	var dirs []string
	if dir != "" {
		dirs = append(dirs, dir)
	}
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Join(filepath.Dir(exe), "tools"))
	}
	// Renders run ffmpeg from a scratch directory, so relative paths would break.
	abs := func(p string) string {
		if a, err := filepath.Abs(p); err == nil && p != "" {
			return a
		}
		return p
	}
	find := func(name string) string {
		for _, d := range dirs {
			for _, n := range []string{name + ".exe", name} {
				p := filepath.Join(d, n)
				if st, err := os.Stat(p); err == nil && !st.IsDir() {
					return abs(p)
				}
			}
		}
		p, _ := exec.LookPath(name)
		return abs(p)
	}
	return Tools{FFmpeg: find("ffmpeg"), FFprobe: find("ffprobe")}
}

type ProbeResult struct {
	Format     string // ffprobe format_name, e.g. "mov,mp4,m4a,3gp,3g2,mj2"
	Width      int
	Height     int
	DurationMS int64
	VideoCodec string
	Profile    string
	PixFmt     string
	AudioCodec string
	FPS        float64
	Bitrate    int64
	Rotation   int
}

type ffprobeOut struct {
	Streams []struct {
		CodecType    string `json:"codec_type"`
		CodecName    string `json:"codec_name"`
		Profile      string `json:"profile"`
		PixFmt       string `json:"pix_fmt"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		AvgFrameRate string `json:"avg_frame_rate"`
		Disposition  struct {
			AttachedPic int `json:"attached_pic"`
		} `json:"disposition"`
		SideDataList []struct {
			Rotation int `json:"rotation"`
		} `json:"side_data_list"`
		Tags struct {
			Rotate string `json:"rotate"`
		} `json:"tags"`
	} `json:"streams"`
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		BitRate    string `json:"bit_rate"`
	} `json:"format"`
}

func (t Tools) Probe(ctx context.Context, path string) (*ProbeResult, error) {
	if t.FFprobe == "" {
		return nil, fmt.Errorf("ffprobe not available")
	}
	out, err := command(ctx, t.FFprobe, "-v", "error", "-print_format", "json",
		"-show_format", "-show_streams", path).Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}
	var p ffprobeOut
	if err := json.Unmarshal(out, &p); err != nil {
		return nil, fmt.Errorf("ffprobe output: %w", err)
	}
	r := &ProbeResult{Format: p.Format.FormatName}
	if d, err := strconv.ParseFloat(p.Format.Duration, 64); err == nil {
		r.DurationMS = int64(d * 1000)
	}
	r.Bitrate, _ = strconv.ParseInt(p.Format.BitRate, 10, 64)
	for _, s := range p.Streams {
		switch {
		// Cover art in an mp3 shows up as a video stream; ignore it.
		case s.CodecType == "video" && s.Disposition.AttachedPic == 0 && r.VideoCodec == "":
			r.VideoCodec, r.Profile, r.PixFmt = s.CodecName, s.Profile, s.PixFmt
			r.Width, r.Height = s.Width, s.Height
			r.FPS = parseRate(s.AvgFrameRate)
			if len(s.SideDataList) > 0 {
				r.Rotation = s.SideDataList[0].Rotation
			} else if s.Tags.Rotate != "" {
				r.Rotation, _ = strconv.Atoi(s.Tags.Rotate)
			}
			// A phone video recorded upright is stored landscape with a rotation flag.
			if r.Rotation == 90 || r.Rotation == -90 || r.Rotation == 270 || r.Rotation == -270 {
				r.Width, r.Height = r.Height, r.Width
			}
		case s.CodecType == "audio" && r.AudioCodec == "":
			r.AudioCodec = s.CodecName
		}
	}
	return r, nil
}

func parseRate(s string) float64 {
	num, den, ok := strings.Cut(s, "/")
	if !ok {
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
	n, _ := strconv.ParseFloat(num, 64)
	d, _ := strconv.ParseFloat(den, 64)
	if d == 0 {
		return 0
	}
	return n / d
}

// ImageSize reads only the header of an image.
func ImageSize(path string) (w, h int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	return cfg.Width, cfg.Height, err
}

const thumbWidth = 480

// ImageThumb decodes an image and writes a JPEG thumbnail thumbWidth wide.
func ImageThumb(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > thumbWidth {
		h = h * thumbWidth / w
		w = thumbWidth
	}
	out := image.NewRGBA(image.Rect(0, 0, w, max(h, 1)))
	draw.ApproxBiLinear.Scale(out, out.Bounds(), img, b, draw.Src, nil)
	return writeJPEG(dst, out)
}

func writeJPEG(dst string, img image.Image) error {
	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 80}); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// VideoThumb grabs one frame a little into the video.
func (t Tools) VideoThumb(ctx context.Context, src, dst string, durationMS int64) error {
	if t.FFmpeg == "" {
		return fmt.Errorf("ffmpeg not available")
	}
	at := min(float64(durationMS)/2000, 1.0)
	tmp := dst + ".tmp.jpg"
	out, err := command(ctx, t.FFmpeg, "-v", "error", "-y", "-ss", strconv.FormatFloat(at, 'f', 2, 64),
		"-i", src, "-frames:v", "1", "-vf", "scale="+strconv.Itoa(thumbWidth)+":-2", "-q:v", "4", tmp).CombinedOutput()
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("ffmpeg thumb: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return os.Rename(tmp, dst)
}
