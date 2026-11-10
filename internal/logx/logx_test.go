package logx

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFormat(t *testing.T) {
	var b strings.Builder
	log := slog.New(New(&b, slog.LevelInfo))
	log.Info("Устройството получи ново съдържание", "устройство", "Кафе › Бар", "кампании", `Меню "лято" v2`)
	log.Warn("Без подробности")
	log.Error("Грешка", "грешка", errors.New("ред1\nред2"), "готово", false)
	log.Debug("скрито")

	lines := strings.Split(strings.TrimRight(b.String(), "\r\n"), "\r\n")
	if len(lines) != 3 {
		t.Fatalf("lines: %q", lines)
	}
	if _, err := time.Parse("2006-01-02 15:04:05", lines[0][:19]); err != nil {
		t.Errorf("time: %q", lines[0])
	}
	want := `ИНФО      Устройството получи ново съдържание      устройство="Кафе › Бар" кампании="Меню 'лято' v2"`
	if !strings.HasSuffix(lines[0], want) {
		t.Errorf("info line:\n%s\nwant suffix:\n%s", lines[0], want)
	}
	if !strings.HasSuffix(lines[1], "ВНИМАНИЕ  Без подробности") {
		t.Errorf("warn line: %q", lines[1])
	}
	if !strings.Contains(lines[2], `грешка="ред1 ⏎ ред2" готово=не`) {
		t.Errorf("error line: %q", lines[2])
	}
}

func TestDailyFile(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "marquee-2020-01-01.log")
	os.WriteFile(old, []byte("x"), 0o640)
	d, err := NewDailyFile(dir, 30)
	if err != nil {
		t.Fatal(err)
	}
	d.Write([]byte("ред\r\n"))
	d.Close()
	today := d.FileFor(time.Now().Format("2006-01-02"))
	if b, err := os.ReadFile(today); err != nil || string(b) != "ред\r\n" {
		t.Errorf("today's file: %q %v", b, err)
	}
	if _, err := os.Stat(old); err == nil {
		t.Error("old log not pruned")
	}
	if days := d.Days(); len(days) != 1 {
		t.Errorf("days: %v", days)
	}
}
