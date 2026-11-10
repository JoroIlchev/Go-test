package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"marquee/internal/store"
)

// Backup copies the database (a consistent snapshot, taken while the hub
// runs) and mirrors the media folder into the backup folder. Media blobs are
// named by their content and never change, so the mirror only copies new
// files; it never deletes, so a file removed from the library stays in the
// backup. Put the backup folder on another disk to survive a disk failure.
type Backup struct {
	st      *store.Store
	log     *slog.Logger
	loc     *time.Location
	dataDir string
	mu      sync.Mutex // one backup at a time
}

// BackupResult is the outcome of the last run, shown in the UI.
type BackupResult struct {
	At          string `json:"at"`
	File        string `json:"file,omitempty"`
	Size        int64  `json:"size,omitempty"`
	MediaCopied int    `json:"media_copied"`
	Error       string `json:"error,omitempty"`
}

type BackupFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	At   string `json:"at"`
}

const lastBackupKey = "backup_last"

func NewBackup(st *store.Store, log *slog.Logger, loc *time.Location, dataDir string) *Backup {
	return &Backup{st: st, log: log, loc: loc, dataDir: dataDir}
}

// Dir is where backups go under the given settings.
func (b *Backup) Dir(set Settings) string {
	if set.BackupDir != "" {
		return set.BackupDir
	}
	return filepath.Join(b.dataDir, "backups")
}

// Run backs up every day at the configured time. Settings are re-read each
// minute, so a change in the UI applies without a restart.
func (b *Backup) Run(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	lastDay := ""
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			set, err := Load(ctx, b.st)
			if err != nil || set.BackupTime == "" {
				continue
			}
			local := now.In(b.loc)
			day := local.Format("2006-01-02")
			if local.Format("15:04") < set.BackupTime || day == lastDay {
				continue
			}
			// Started later than the time (or restarted): one backup per day is enough.
			if last := b.Last(ctx); last.At != "" && last.Error == "" {
				if t, err := time.Parse(time.RFC3339, last.At); err == nil && t.In(b.loc).Format("2006-01-02") == day {
					lastDay = day
					continue
				}
			}
			lastDay = day
			b.Now(ctx)
		}
	}
}

// Now makes a backup immediately and records the result.
func (b *Backup) Now(ctx context.Context) BackupResult {
	b.mu.Lock()
	defer b.mu.Unlock()
	res := BackupResult{At: time.Now().UTC().Format(time.RFC3339)}
	if err := b.run(ctx, &res); err != nil {
		res.Error = err.Error()
		b.log.Error("Резервното копие не успя", "грешка", err)
	} else {
		b.log.Info("Резервно копие", "файл", res.File, "нови медийни файлове", res.MediaCopied)
	}
	js, _ := json.Marshal(res)
	b.st.SetSetting(ctx, lastBackupKey, string(js))
	return res
}

func (b *Backup) run(ctx context.Context, res *BackupResult) error {
	set, err := Load(ctx, b.st)
	if err != nil {
		return err
	}
	dir := b.Dir(set)
	if err := os.MkdirAll(filepath.Join(dir, "media"), 0o750); err != nil {
		return err
	}
	name := "marquee-" + time.Now().In(b.loc).Format("20060102-150405") + ".db"
	path := filepath.Join(dir, name)
	os.Remove(path) // a second backup within the same second replaces the first
	if err := b.st.BackupTo(ctx, path); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	if st, err := os.Stat(path); err == nil {
		res.File, res.Size = name, st.Size()
	}
	n, err := mirror(ctx, filepath.Join(b.dataDir, "media"), filepath.Join(dir, "media"))
	res.MediaCopied = n
	if err != nil {
		return fmt.Errorf("media: %w", err)
	}
	return b.prune(dir, set.BackupKeep)
}

// prune keeps the newest keep database copies.
func (b *Backup) prune(dir string, keep int) error {
	files, err := b.list(dir)
	if err != nil {
		return err
	}
	for _, f := range files[min(keep, len(files)):] {
		if err := os.Remove(filepath.Join(dir, f.Name)); err != nil {
			return err
		}
	}
	return nil
}

// List returns the database copies, newest first.
func (b *Backup) List(ctx context.Context) ([]BackupFile, string, error) {
	set, err := Load(ctx, b.st)
	if err != nil {
		return nil, "", err
	}
	dir := b.Dir(set)
	files, err := b.list(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []BackupFile{}, dir, nil
	}
	return files, dir, err
}

func (b *Backup) list(dir string) ([]BackupFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := []BackupFile{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "marquee-") || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, BackupFile{Name: e.Name(), Size: info.Size(), At: info.ModTime().UTC().Format(time.RFC3339)})
	}
	// Names carry the timestamp, so name order is time order.
	slices.SortFunc(files, func(a, b BackupFile) int { return strings.Compare(b.Name, a.Name) })
	return files, nil
}

func (b *Backup) Last(ctx context.Context) BackupResult {
	var r BackupResult
	if raw, _ := b.st.Setting(ctx, lastBackupKey); raw != "" {
		json.Unmarshal([]byte(raw), &r)
	}
	return r
}

// mirror copies files from src that dst lacks (or has with another size),
// skipping the scratch folder.
func mirror(ctx context.Context, src, dst string) (int, error) {
	copied := 0
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, _ := filepath.Rel(src, path)
		if d.IsDir() {
			if rel == "tmp" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o750)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if st, err := os.Stat(target); err == nil && st.Size() == info.Size() {
			return nil
		}
		if err := copyAtomic(path, target); err != nil {
			return err
		}
		copied++
		return nil
	})
	return copied, err
}

// copyAtomic writes to a temporary name first, so an interrupted copy never
// looks complete.
func copyAtomic(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}
