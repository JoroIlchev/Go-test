// Package ops keeps the hub running unattended: it notices screens that went
// silent, e-mails about it, backs up the data every night and reports device
// usage per client for billing.
package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"marquee/internal/store"
)

// Settings are the hub-wide options the super admin edits in the UI.
type Settings struct {
	// PublicURL is the hub's address as devices see it, put into device
	// links; "" = this machine's detected IP.
	PublicURL string `json:"public_url"`

	SMTPHost string `json:"smtp_host"`
	SMTPPort int    `json:"smtp_port"` // 587 STARTTLS, 465 TLS, 25 plain
	SMTPUser string `json:"smtp_user"`
	SMTPPass string `json:"smtp_pass,omitempty"`
	SMTPFrom string `json:"smtp_from"`

	AlertEmails   []string `json:"alert_emails"`    // always notified
	AlertAdmins   bool     `json:"alert_admins"`    // also the admins above the device
	AlertGraceMin int      `json:"alert_grace_min"` // how late a check may be before "offline"

	BackupTime string `json:"backup_time"` // "HH:MM" local, "" = off
	BackupKeep int    `json:"backup_keep"` // database copies kept
	BackupDir  string `json:"backup_dir"`  // "" = data\backups
}

const settingsKey = "hub"

func Defaults() Settings {
	return Settings{SMTPPort: 587, AlertGraceMin: 10, BackupTime: "03:00", BackupKeep: 14, AlertEmails: []string{}}
}

func Load(ctx context.Context, st *store.Store) (Settings, error) {
	s := Defaults()
	raw, err := st.Setting(ctx, settingsKey)
	if err != nil || raw == "" {
		return s, err
	}
	err = json.Unmarshal([]byte(raw), &s)
	if s.AlertEmails == nil {
		s.AlertEmails = []string{}
	}
	return s, err
}

func Save(ctx context.Context, st *store.Store, s Settings) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return st.SetSetting(ctx, settingsKey, string(b))
}

// Normalize validates s; an empty SMTPPass keeps prev's, so the UI never
// needs to see the stored password.
func (s *Settings) Normalize(prev Settings) error {
	s.SMTPHost = strings.TrimSpace(s.SMTPHost)
	s.SMTPFrom = strings.TrimSpace(s.SMTPFrom)
	if s.SMTPPass == "" {
		s.SMTPPass = prev.SMTPPass
	}
	if s.SMTPPort == 0 {
		s.SMTPPort = 587
	}
	if s.SMTPPort < 1 || s.SMTPPort > 65535 {
		return errors.New("invalid SMTP port")
	}
	if s.SMTPFrom != "" {
		if _, err := mail.ParseAddress(s.SMTPFrom); err != nil {
			return errors.New("invalid sender address")
		}
	}
	emails := []string{}
	for _, e := range s.AlertEmails {
		if e = strings.TrimSpace(e); e == "" {
			continue
		}
		if _, err := mail.ParseAddress(e); err != nil {
			return fmt.Errorf("invalid e-mail %q", e)
		}
		emails = append(emails, e)
	}
	s.AlertEmails = emails
	if s.AlertGraceMin < 1 || s.AlertGraceMin > 24*60 {
		return errors.New("grace must be 1..1440 minutes")
	}
	if s.BackupTime != "" {
		var h, m int
		if n, _ := fmt.Sscanf(s.BackupTime, "%d:%d", &h, &m); n != 2 || h < 0 || h > 23 || m < 0 || m > 59 {
			return errors.New("invalid backup time")
		}
		s.BackupTime = fmt.Sprintf("%02d:%02d", h, m)
	}
	if s.BackupKeep < 1 || s.BackupKeep > 365 {
		return errors.New("keep 1..365 backups")
	}
	s.BackupDir = strings.TrimSpace(s.BackupDir)
	return nil
}

func (s Settings) MailConfigured() bool { return s.SMTPHost != "" && s.SMTPFrom != "" }
