// Package logx writes the hub's log for people: one line per event, aligned
// columns, local time, and a new file every day.
//
//	2026-11-10 15:01:42  ИНФО      Устройството получи ново съдържание   устройство="Кафе › Бар" кампании="Обедно меню v2"
package logx

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	timeFormat = "2006-01-02 15:04:05"
	msgWidth   = 40
)

// Handler is a slog.Handler producing the format above.
type Handler struct {
	mu    *sync.Mutex
	w     io.Writer
	level slog.Leveler
	attrs []slog.Attr
	group string
}

func New(w io.Writer, level slog.Leveler) *Handler {
	return &Handler{mu: &sync.Mutex{}, w: w, level: level}
}

func (h *Handler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level.Level() }

func (h *Handler) WithAttrs(as []slog.Attr) slog.Handler {
	c := *h
	c.attrs = append(slices.Clone(h.attrs), h.qualify(as)...)
	return &c
}

func (h *Handler) WithGroup(name string) slog.Handler {
	c := *h
	if c.group != "" {
		name = c.group + "." + name
	}
	c.group = name
	return &c
}

func (h *Handler) qualify(as []slog.Attr) []slog.Attr {
	if h.group == "" {
		return as
	}
	out := make([]slog.Attr, len(as))
	for i, a := range as {
		out[i] = slog.Attr{Key: h.group + "." + a.Key, Value: a.Value}
	}
	return out
}

func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	t := r.Time
	if t.IsZero() {
		t = time.Now()
	}
	b.WriteString(t.Format(timeFormat))
	b.WriteString("  ")
	b.WriteString(pad(levelName(r.Level), 9))
	b.WriteString(" ")
	attrs := slices.Clone(h.attrs)
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, h.qualify([]slog.Attr{a})...)
		return true
	})
	if len(attrs) == 0 {
		b.WriteString(r.Message)
	} else {
		b.WriteString(pad(r.Message, msgWidth))
		for _, a := range attrs {
			writeAttr(&b, "", a)
		}
	}
	b.WriteString("\r\n") // Notepad-friendly
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

func writeAttr(b *strings.Builder, prefix string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	key := prefix + a.Key
	if a.Value.Kind() == slog.KindGroup {
		for _, g := range a.Value.Group() {
			writeAttr(b, key+".", g)
		}
		return
	}
	b.WriteString(" ")
	b.WriteString(key)
	b.WriteString("=")
	b.WriteString(quote(valueString(a.Value)))
}

func valueString(v slog.Value) string {
	switch v.Kind() {
	case slog.KindTime:
		return v.Time().Format(timeFormat)
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindBool:
		if v.Bool() {
			return "да"
		}
		return "не"
	}
	return v.String()
}

// quote wraps values with spaces or quotes so every line splits cleanly into
// key=value pairs; newlines never break a line in two.
func quote(s string) string {
	s = strings.NewReplacer("\r\n", " ⏎ ", "\n", " ⏎ ", "\r", " ").Replace(s)
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, " \t\"=") {
		return `"` + strings.ReplaceAll(s, `"`, `'`) + `"`
	}
	return s
}

func levelName(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "ГРЕШКА"
	case l >= slog.LevelWarn:
		return "ВНИМАНИЕ"
	case l >= slog.LevelInfo:
		return "ИНФО"
	}
	return "ДЕБЪГ"
}

func pad(s string, n int) string {
	if k := utf8.RuneCountInString(s); k < n {
		return s + strings.Repeat(" ", n-k)
	}
	return s
}

// DailyFile writes to dir/marquee-YYYY-MM-DD.log, switching files at local
// midnight and deleting files older than keepDays.
type DailyFile struct {
	dir      string
	keepDays int
	mu       sync.Mutex
	day      string
	f        *os.File
}

func NewDailyFile(dir string, keepDays int) (*DailyFile, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	return &DailyFile{dir: dir, keepDays: keepDays}, nil
}

func (d *DailyFile) Dir() string { return d.dir }

// FileFor is the log file of a day ("2006-01-02").
func (d *DailyFile) FileFor(day string) string {
	return filepath.Join(d.dir, "marquee-"+day+".log")
}

func (d *DailyFile) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	day := time.Now().Format("2006-01-02")
	if d.f == nil || day != d.day {
		if d.f != nil {
			d.f.Close()
		}
		f, err := os.OpenFile(d.FileFor(day), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
		if err != nil {
			return 0, err
		}
		d.f, d.day = f, day
		d.prune()
	}
	return d.f.Write(p)
}

func (d *DailyFile) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.f == nil {
		return nil
	}
	return d.f.Close()
}

// Days lists the days with a log file, newest first.
func (d *DailyFile) Days() []string {
	entries, _ := os.ReadDir(d.dir)
	var days []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, "marquee-") && strings.HasSuffix(n, ".log") {
			day := strings.TrimSuffix(strings.TrimPrefix(n, "marquee-"), ".log")
			if _, err := time.Parse("2006-01-02", day); err == nil {
				days = append(days, day)
			}
		}
	}
	slices.Sort(days)
	slices.Reverse(days)
	return days
}

func (d *DailyFile) prune() {
	cutoff := time.Now().AddDate(0, 0, -d.keepDays).Format("2006-01-02")
	for _, day := range d.Days() {
		if day < cutoff {
			os.Remove(d.FileFor(day))
		}
	}
}

// Fanout writes every line to all writers; a failing one (say, a console
// that is gone) does not stop the others.
type Fanout []io.Writer

func (f Fanout) Write(p []byte) (int, error) {
	for _, w := range f {
		w.Write(p)
	}
	return len(p), nil
}
