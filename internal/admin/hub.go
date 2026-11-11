package admin

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"marquee/internal/ops"
)

// hubURL is the address devices use to reach the hub, for device links: a
// fixed one from the command line, else the one set in the UI, else this
// machine's own IP (detected each time, so it follows a network change).
func (s *Server) hubURL(ctx context.Context) string {
	if s.publicURL != "" {
		return s.publicURL
	}
	if set, err := ops.Load(ctx, s.store); err == nil && set.PublicURL != "" {
		return set.PublicURL
	}
	if urls := s.detectedURLs(); len(urls) > 0 {
		return urls[0]
	}
	return s.playerScheme + "://localhost" + s.portSuffix()
}

// HubURL is hubURL for the startup log.
func (s *Server) HubURL(ctx context.Context) string { return s.hubURL(ctx) }

// detectedURLs are the hub addresses on each of this machine's IPs.
func (s *Server) detectedURLs() []string {
	urls := []string{}
	for _, ip := range ops.LocalIPs() {
		urls = append(urls, s.playerScheme+"://"+ip+s.portSuffix())
	}
	return urls
}

func (s *Server) portSuffix() string {
	if s.playerPort == "" || (s.playerScheme == "http" && s.playerPort == "80") || (s.playerScheme == "https" && s.playerPort == "443") {
		return ""
	}
	return ":" + s.playerPort
}

// audit records an admin action in the database and in the log, so the log
// file tells who did what without opening the database.
func (s *Server) audit(r *http.Request, action string, nodeID int64, detail string) {
	u := userFrom(r)
	s.store.Audit(r.Context(), u.ID, action, nodeID, detail)
	label, ok := actionLabels[action]
	if !ok {
		label = action
	}
	args := []any{"потребител", u.Email}
	if nodeID != 0 {
		if n, err := s.store.Node(r.Context(), nodeID); err == nil {
			args = append(args, "звено", n.Name)
		}
	}
	if detail != "" {
		args = append(args, "какво", detail)
	}
	s.log.Info(label, args...)
}

var actionLabels = map[string]string{
	"campaign.create":  "Създадена кампания",
	"campaign.publish": "Публикувана кампания",
	"campaign.approve": "Одобрена кампания",
	"campaign.reject":  "Отхвърлена кампания",
	"campaign.live":    "Сменена версия на живо",
	"campaign.delete":  "Изтрита кампания",
	"media.upload":     "Качен файл",
	"media.delete":     "Изтрит файл",
	"media.convert":    "Конвертиране на видео",
	"media.render":     "Видео от снимки",
	"node.create":      "Създадено звено",
	"node.rename":      "Преименувано звено",
	"node.delete":      "Изтрито звено",
	"node.poll":        "Сменен график на проверките",
	"device.pair":      "Свързано устройство",
	"device.create":    "Добавено устройство",
	"device.update":    "Променено устройство",
	"user.create":      "Създаден потребител",
	"user.update":      "Променен потребител",
	"staff.create":     "Добавен в екипа",
	"staff.update":     "Променен член на екипа",
	"settings.update":  "Променени настройки",
	"settings.poll":    "Променена проверка по подразбиране",
	"backup.run":       "Ръчно резервно копие",
}

const maxLogLines = 5000

// viewLogs returns the last lines of a day's log for the in-browser viewer.
func (s *Server) viewLogs(w http.ResponseWriter, r *http.Request) {
	if s.logs == nil {
		writeJSON(w, http.StatusOK, map[string]any{"days": []string{}, "lines": []string{}})
		return
	}
	days := s.logs.Days()
	day := r.URL.Query().Get("day")
	if _, err := time.Parse("2006-01-02", day); err != nil {
		day = time.Now().Format("2006-01-02")
	}
	lines := []string{}
	truncated := false
	if f, err := os.Open(s.logs.FileFor(day)); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			lines = append(lines, strings.TrimRight(sc.Text(), "\r"))
			if len(lines) > maxLogLines {
				lines = lines[1:]
				truncated = true
			}
		}
		f.Close()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"day": day, "days": days, "lines": lines, "truncated": truncated, "file": s.logs.FileFor(day),
	})
}

// playerPortOf extracts the port of a listen address like ":8080".
func PlayerPortOf(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	return port
}
