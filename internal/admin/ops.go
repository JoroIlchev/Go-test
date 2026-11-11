package admin

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"marquee/internal/auth"
	"marquee/internal/ops"
	"marquee/internal/poll"
	"marquee/internal/store"
)

type pollDTO struct {
	Own         *poll.Policy `json:"own"`       // set on this node; nil = inherited
	Effective   poll.Policy  `json:"effective"` // what the devices here use
	Description string       `json:"description"`
	FromNodeID  int64        `json:"from_node_id"` // where Effective comes from; 0 = hub default
	FromName    string       `json:"from_name"`
	CanEdit     bool         `json:"can_edit"`
}

func (s *Server) nodePoll(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	n := s.pathNode(w, r)
	if n == nil {
		return
	}
	if !auth.Can(u, n, auth.PermView) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	chain := n.AncestorIDs()
	set, err := s.store.NodePolls(r.Context(), chain)
	if err != nil {
		s.fail(w, err)
		return
	}
	def, _ := ops.DefaultPoll(r.Context(), s.store, s.defaultPoll)
	eff, from := poll.Resolve(chain, set, def)
	out := pollDTO{Effective: eff, Description: eff.Describe(), FromNodeID: from, CanEdit: auth.Can(u, n, auth.PermAdmin)}
	if from == n.ID {
		out.Own = &eff
	}
	if from != 0 {
		if fn, err := s.store.Node(r.Context(), from); err == nil {
			out.FromName = fn.Name
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) setNodePoll(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	n := s.pathNode(w, r)
	if n == nil {
		return
	}
	if !auth.Can(u, n, auth.PermAdmin) {
		forbidden(w)
		return
	}
	var in struct {
		Policy *poll.Policy `json:"policy"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	raw := ""
	if in.Policy != nil {
		if err := in.Policy.Normalize(); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		raw = in.Policy.JSON()
	}
	if err := s.store.SetNodePoll(r.Context(), n.ID, raw, u.ID); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "node.poll", n.ID, raw)
	s.nodePoll(w, r)
}

// setDefaultPoll changes how often devices check where no node above them
// has its own schedule; null returns to the -refresh value.
func (s *Server) setDefaultPoll(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Policy *poll.Policy `json:"policy"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := ops.SetDefaultPoll(r.Context(), s.store, in.Policy); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	detail := "по подразбиране (-refresh)"
	if in.Policy != nil {
		detail = in.Policy.Describe()
	}
	s.audit(r, "settings.poll", 0, detail)
	s.getSettings(w, r)
}

func (s *Server) pathNode(w http.ResponseWriter, r *http.Request) *store.Node {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return nil
	}
	n, err := s.store.Node(r.Context(), id)
	if err != nil {
		s.storeErr(w, err)
		return nil
	}
	return n
}

// staffWith wraps pages a manager sees only with the given option.
func (s *Server) staffWith(allowed func(u *store.User) bool) func(http.HandlerFunc) http.Handler {
	return func(h http.HandlerFunc) http.Handler {
		return s.authed(func(w http.ResponseWriter, r *http.Request) {
			if u := userFrom(r); !u.IsSuper && !(u.IsManager && allowed(u)) {
				forbidden(w)
				return
			}
			h(w, r)
		})
	}
}

// superOnly wraps hub-wide endpoints.
func (s *Server) superOnly(h http.HandlerFunc) http.Handler {
	return s.authed(func(w http.ResponseWriter, r *http.Request) {
		if !userFrom(r).IsSuper {
			forbidden(w)
			return
		}
		h(w, r)
	})
}

type settingsDTO struct {
	ops.Settings
	HasPassword bool             `json:"has_password"`
	BackupPath  string           `json:"backup_path"` // the folder actually used
	LastBackup  ops.BackupResult `json:"last_backup"`
	Backups     []ops.BackupFile `json:"backups"`
	Default     poll.Policy      `json:"default_poll"`
	DefaultText string           `json:"default_poll_text"`
	DefaultOwn  bool             `json:"default_poll_own"` // set here, not by -refresh
	// The hub address in device links: what is used now, the candidates on
	// this machine, and whether a command-line flag fixes it.
	HubURL      string   `json:"hub_url"`
	HubPrivate  bool     `json:"hub_private"`
	Detected    []string `json:"detected_urls"`
	HubURLFixed bool     `json:"hub_url_fixed"`
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	set, err := ops.Load(r.Context(), s.store)
	if err != nil {
		s.fail(w, err)
		return
	}
	def, own := ops.DefaultPoll(r.Context(), s.store, s.defaultPoll)
	out := settingsDTO{Settings: set, HasPassword: set.SMTPPass != "", Default: def, DefaultText: def.Describe(), DefaultOwn: own,
		HubURL: s.hubURL(r.Context()), Detected: s.detectedURLs(), HubURLFixed: s.publicURL != ""}
	out.HubPrivate = ops.IsPrivate(out.HubURL)
	out.SMTPPass = ""
	if s.backup != nil {
		files, dir, err := s.backup.List(r.Context())
		if err != nil {
			s.log.Warn("Списъкът с резервни копия не се чете", "грешка", err)
		}
		out.Backups, out.BackupPath, out.LastBackup = files, dir, s.backup.Last(r.Context())
	}
	if out.Backups == nil {
		out.Backups = []ops.BackupFile{}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var in ops.Settings
	if !readJSON(w, r, &in) {
		return
	}
	prev, err := ops.Load(r.Context(), s.store)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := in.Normalize(prev); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if in.PublicURL, err = ops.NormalizeHubURL(in.PublicURL, s.playerPort); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := ops.Save(r.Context(), s.store, in); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "settings.update", 0, "")
	s.getSettings(w, r)
}

func (s *Server) testMail(w http.ResponseWriter, r *http.Request) {
	var in struct{ To string }
	if !readJSON(w, r, &in) {
		return
	}
	set, err := ops.Load(r.Context(), s.store)
	if err != nil {
		s.fail(w, err)
		return
	}
	to := strings.TrimSpace(in.To)
	if to == "" {
		to = userFrom(r).Email
	}
	err = set.SendMail(r.Context(), []string{to}, "marquee: пробно писмо",
		"Ако четете това, известията от marquee стигат до вас.\n\n-- \nmarquee")
	if err != nil {
		// The SMTP server's own words are the most useful diagnosis here.
		writeErr(w, http.StatusBadGateway, "mail: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) backupNow(w http.ResponseWriter, r *http.Request) {
	if s.backup == nil {
		writeErr(w, http.StatusServiceUnavailable, "backups are not available")
		return
	}
	res := s.backup.Now(r.Context())
	s.audit(r, "backup.run", 0, res.File)
	writeJSON(w, http.StatusOK, res)
}

type usageRow struct {
	ops.Usage
	Name string `json:"name"`
}

// billing reports device usage per client for a calendar month.
func (s *Server) billing(w http.ResponseWriter, r *http.Request) {
	month := r.URL.Query().Get("month")
	if month == "" {
		month = time.Now().In(s.loc).Format("2006-01")
	}
	from, err := time.ParseInLocation("2006-01", month, s.loc)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "month must be YYYY-MM")
		return
	}
	to := from.AddDate(0, 1, 0)
	// The current month so far; history has second resolution, so include
	// the current second.
	if now := time.Now().Truncate(time.Second).Add(time.Second); to.After(now) {
		to = now
	}
	events, err := s.store.DeviceHistory(r.Context(), to)
	if err != nil {
		s.fail(w, err)
		return
	}
	usage := ops.ComputeUsage(events, from, to, s.loc)
	clients, err := s.store.Clients(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	rows := []usageRow{}
	u := userFrom(r)
	for _, c := range clients {
		if !auth.ManagesClient(u, c.ID) {
			continue
		}
		row := usageRow{Usage: ops.Usage{ClientID: c.ID}, Name: c.Name}
		if u := usage[c.ID]; u != nil {
			row.Usage = *u
		}
		rows = append(rows, row)
	}
	slices.SortFunc(rows, func(a, b usageRow) int { return strings.Compare(a.Name, b.Name) })

	if r.URL.Query().Get("format") == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="marquee-%s.csv"`, month))
		w.Write([]byte{0xEF, 0xBB, 0xBF}) // Excel needs the BOM to read UTF-8 Cyrillic
		cw := csv.NewWriter(w)
		cw.Comma = ';' // Excel with Bulgarian regional settings
		cw.Write([]string{"Клиент", "Устройство-дни", "Устройства през месеца", "Активни в края"})
		for _, row := range rows {
			cw.Write([]string{row.Name, strconv.Itoa(row.DeviceDays), strconv.Itoa(row.Devices), strconv.Itoa(row.ActiveEnd)})
		}
		cw.Flush()
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"month": month, "from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339),
		"days": int(to.Sub(from).Hours()/24 + 0.999), "rows": rows,
	})
}
