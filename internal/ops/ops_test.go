package ops

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/base64"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"marquee/internal/store"
)

func TestComputeUsage(t *testing.T) {
	sofia, _ := time.LoadLocation("Europe/Sofia")
	at := func(s string) time.Time {
		v, _ := time.ParseInLocation("2006-01-02 15:04", s, sofia)
		return v
	}
	c := func(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: true} }
	events := []store.HistoryEvent{
		// Device 1: active for client 7 since October, all of November.
		{DeviceID: 1, ClientID: c(7), Status: "active", At: at("2026-10-15 10:00")},
		// Device 2: paired on the 10th, blocked on the 12th at 01:00: days 10, 11, 12.
		{DeviceID: 2, Status: "pending", At: at("2026-11-01 09:00")},
		{DeviceID: 2, ClientID: c(7), Status: "active", At: at("2026-11-10 23:00")},
		{DeviceID: 2, ClientID: c(7), Status: "blocked", At: at("2026-11-12 01:00")},
		// Device 3: moved from client 7 to client 8 on the 20th at noon: counts for both that day.
		{DeviceID: 3, ClientID: c(7), Status: "active", At: at("2026-11-18 08:00")},
		{DeviceID: 3, ClientID: c(8), Status: "active", At: at("2026-11-20 12:00")},
		// After the period: ignored.
		{DeviceID: 4, ClientID: c(8), Status: "active", At: at("2026-12-02 08:00")},
	}
	u := ComputeUsage(events, at("2026-11-01 00:00"), at("2026-12-01 00:00"), sofia)
	if got := *u[7]; got.DeviceDays != 30+3+3 || got.Devices != 3 || got.ActiveEnd != 1 {
		t.Errorf("client 7: %+v", got)
	}
	if got := *u[8]; got.DeviceDays != 11 || got.Devices != 1 || got.ActiveEnd != 1 {
		t.Errorf("client 8: %+v", got)
	}
}

func newStore(t *testing.T) *store.Store {
	st, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestMonitorAlerts(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	smtp := fakeSMTP(t)

	client, _ := st.CreateNode(ctx, nil, "client", "Иванов")
	site, _ := st.CreateNode(ctx, client, "site", "Кафе")
	dev, _ := st.CreateLinkDevice(ctx, site, "Бар")
	if _, err := st.CreateUser(ctx, "ivan@x.bg", "Иван", "x", false, &client.ID, nil,
		[]store.Role{{NodeID: client.ID, Role: "admin"}}); err != nil {
		t.Fatal(err)
	}
	set := Defaults()
	host, port, _ := net.SplitHostPort(smtp.addr)
	set.SMTPHost, set.SMTPFrom, set.AlertEmails, set.AlertAdmins = host, "hub@x.bg", []string{"ops@x.bg"}, true
	set.SMTPPort = atoi(port)
	if err := Save(ctx, st, set); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	m := NewMonitor(st, slog.New(slog.NewTextHandler(io.Discard, nil)), time.UTC)
	m.started = now.Add(-time.Hour)

	// Was due 5 minutes ago: within the 10-minute grace.
	st.SetNextCheck(ctx, dev.ID, now.Add(-5*time.Minute))
	if err := m.Check(ctx, now); err != nil {
		t.Fatal(err)
	}
	if smtp.count() != 0 {
		t.Fatal("alerted within the grace period")
	}
	// Was due 12 minutes ago and never came.
	st.SetNextCheck(ctx, dev.ID, now.Add(-12*time.Minute))
	if err := m.Check(ctx, now); err != nil {
		t.Fatal(err)
	}
	msgs := smtp.take()
	if len(msgs) != 2 {
		t.Fatalf("want one message each to ops and the client admin, got %d", len(msgs))
	}
	body := msgs[0]
	if !strings.Contains(body, "Иванов › Кафе › Бар") || !strings.Contains(body, "Не се свързаха") {
		t.Errorf("message:\n%s", body)
	}
	// Not repeated while still offline.
	m.Check(ctx, now.Add(time.Minute))
	if smtp.count() != 0 {
		t.Fatal("alert repeated")
	}
	// Comes back.
	st.TouchDevice(ctx, dev.ID, "", "")
	m.Check(ctx, now.Add(2*time.Minute))
	if msgs := smtp.take(); len(msgs) != 2 || !strings.Contains(msgs[0], "Отново се свързаха") {
		t.Fatalf("back online: %v", msgs)
	}

	// Checks missed while the hub itself was down do not count.
	m.started = now.Add(-time.Minute)
	st.SetNextCheck(ctx, dev.ID, now.Add(-30*time.Minute))
	m.Check(ctx, now)
	if smtp.count() != 0 {
		t.Fatal("alerted for a check missed during hub downtime")
	}
}

func TestBackup(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	st, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.CreateNode(ctx, nil, "client", "Иванов")
	os.MkdirAll(filepath.Join(dataDir, "media", "ab"), 0o750)
	os.MkdirAll(filepath.Join(dataDir, "media", "tmp"), 0o750)
	os.WriteFile(filepath.Join(dataDir, "media", "ab", "abc.jpg"), []byte("jpeg"), 0o644)
	os.WriteFile(filepath.Join(dataDir, "media", "tmp", "upload-1"), []byte("half"), 0o644)

	set := Defaults()
	set.BackupKeep = 2
	set.BackupDir = filepath.Join(t.TempDir(), "elsewhere")
	Save(ctx, st, set)
	b := NewBackup(st, slog.New(slog.NewTextHandler(io.Discard, nil)), time.UTC, dataDir)

	res := b.Now(ctx)
	if res.Error != "" || res.MediaCopied != 1 || res.Size == 0 {
		t.Fatalf("first: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(set.BackupDir, "media", "tmp", "upload-1")); err == nil {
		t.Error("scratch files backed up")
	}
	// The copy is a working database.
	copyDir := t.TempDir()
	os.Rename(filepath.Join(set.BackupDir, res.File), filepath.Join(copyDir, "marquee.db"))
	restored, err := store.Open(ctx, copyDir)
	if err != nil {
		t.Fatal(err)
	}
	if cl, _ := restored.Clients(ctx); len(cl) != 1 || cl[0].Name != "Иванов" {
		t.Errorf("restored clients: %v", cl)
	}
	restored.Close()

	if res := b.Now(ctx); res.MediaCopied != 0 {
		t.Errorf("unchanged media copied again: %+v", res)
	}
	// Keep only the newest two database copies.
	time.Sleep(1100 * time.Millisecond) // names have second resolution
	for _, name := range []string{"marquee-20200101-030000.db", "marquee-20200102-030000.db"} {
		os.WriteFile(filepath.Join(set.BackupDir, name), []byte("old"), 0o644)
	}
	b.Now(ctx)
	files, _, _ := b.List(ctx)
	if len(files) != 2 || strings.HasPrefix(files[1].Name, "marquee-2020") {
		t.Errorf("after pruning: %+v", files)
	}
}

// fakeSMTP is a minimal SMTP server that records message bodies.
type smtpServer struct {
	addr string
	msgs chan string
}

func (s *smtpServer) count() int { return len(s.msgs) }

func (s *smtpServer) take() []string {
	var out []string
	for {
		select {
		case m := <-s.msgs:
			out = append(out, m)
		default:
			return out
		}
	}
}

func fakeSMTP(t *testing.T) *smtpServer {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	s := &smtpServer{addr: ln.Addr().String(), msgs: make(chan string, 100)}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				r := bufio.NewReader(c)
				say := func(l string) { io.WriteString(c, l+"\r\n") }
				say("220 fake")
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					cmd := strings.ToUpper(strings.TrimSpace(line))
					switch {
					case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
						say("250 fake")
					case cmd == "DATA":
						say("354 go")
						var data strings.Builder
						for {
							l, err := r.ReadString('\n')
							if err != nil || l == ".\r\n" {
								break
							}
							data.WriteString(l)
						}
						s.msgs <- decodeBody(data.String())
						say("250 ok")
					case cmd == "QUIT":
						say("221 bye")
						return
					default:
						say("250 ok")
					}
				}
			}()
		}
	}()
	return s
}

func decodeBody(msg string) string {
	_, body, _ := strings.Cut(msg, "\r\n\r\n")
	b, _ := base64.StdEncoding.DecodeString(strings.ReplaceAll(body, "\r\n", ""))
	return string(b)
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}
