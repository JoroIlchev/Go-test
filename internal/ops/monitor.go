package ops

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"marquee/internal/store"
)

// Monitor marks devices offline when they miss their expected check and
// e-mails about it, and again when they come back.
//
// A device's next check is known exactly (the hub told it when to return),
// so this works the same for a 15-minute interval and for twice a day; the
// price of twice a day is that a dead screen is noticed only at its next check.
type Monitor struct {
	st      *store.Store
	log     *slog.Logger
	loc     *time.Location
	started time.Time
}

func NewMonitor(st *store.Store, log *slog.Logger, loc *time.Location) *Monitor {
	return &Monitor{st: st, log: log, loc: loc, started: time.Now()}
}

func (m *Monitor) Run(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if err := m.Check(ctx, now); err != nil && ctx.Err() == nil {
				m.log.Error("Проверката за офлайн устройства не успя", "грешка", err)
			}
		}
	}
}

type event struct {
	dev     *store.Device
	label   string
	offline bool
}

// Check runs one round. Checks that fell due while the hub itself was down
// are not the devices' fault, so only those after the hub started count.
func (m *Monitor) Check(ctx context.Context, now time.Time) error {
	set, err := Load(ctx, m.st)
	if err != nil {
		return err
	}
	grace := time.Duration(set.AlertGraceMin) * time.Minute
	overdue, err := m.st.OverdueDevices(ctx, m.started, now.Add(-grace))
	if err != nil {
		return err
	}
	returned, err := m.st.ReturnedDevices(ctx)
	if err != nil {
		return err
	}
	var events []event
	for _, d := range overdue {
		// Offline since the check it missed: any visit after that means it is back.
		due, err := time.Parse(time.RFC3339, d.NextCheckAt.String)
		if err != nil {
			due = now
		}
		if err := m.st.SetOffline(ctx, d.ID, due); err != nil {
			return err
		}
		events = append(events, event{dev: d, offline: true})
	}
	for _, d := range returned {
		if err := m.st.SetOffline(ctx, d.ID, time.Time{}); err != nil {
			return err
		}
		if d.Status == "active" {
			events = append(events, event{dev: d})
		}
	}
	if len(events) == 0 {
		return nil
	}
	if !set.MailConfigured() {
		for _, e := range events {
			m.logEvent(ctx, e, "имейлът не е настроен")
		}
		return nil
	}

	// Each recipient gets one message with everything relevant to them.
	byRcpt := map[string][]event{}
	for _, e := range events {
		rcpts, label, err := m.recipients(ctx, set, e.dev)
		if err != nil {
			return err
		}
		e.label = label
		for _, r := range rcpts {
			byRcpt[r] = append(byRcpt[r], e)
		}
		m.logEvent(ctx, e, strings.Join(rcpts, ", "))
	}
	for rcpt, evs := range byRcpt {
		subject, body := m.compose(evs)
		if err := set.SendMail(ctx, []string{rcpt}, subject, body); err != nil {
			m.log.Error("Имейлът с известие не се изпрати", "до", rcpt, "грешка", err)
		}
	}
	return nil
}

// recipients are the always-notified addresses plus, when enabled, the
// admins of every node above the device. label is the device's full path.
func (m *Monitor) recipients(ctx context.Context, set Settings, d *store.Device) ([]string, string, error) {
	rcpts := slices.Clone(set.AlertEmails)
	label := d.Name
	if d.NodeID.Valid {
		site, err := m.st.Node(ctx, d.NodeID.Int64)
		if err != nil {
			return nil, "", err
		}
		var names []string
		for _, id := range site.AncestorIDs() {
			if n, err := m.st.Node(ctx, id); err == nil {
				names = append(names, n.Name)
			}
		}
		label = strings.Join(append(names, d.Name), " › ")
		if set.AlertAdmins {
			admins, err := m.st.AdminEmails(ctx, site.AncestorIDs())
			if err != nil {
				return nil, "", err
			}
			rcpts = append(rcpts, admins...)
		}
	}
	slices.Sort(rcpts)
	return slices.Compact(rcpts), label, nil
}

func (m *Monitor) logEvent(ctx context.Context, e event, notified string) {
	label := e.label
	if label == "" {
		if _, l, err := m.recipients(ctx, Settings{}, e.dev); err == nil {
			label = l
		}
	}
	if e.offline {
		m.log.Warn("Устройството не провери навреме (офлайн)", "устройство", label,
			"последно видяно", m.when(e.dev.LastSeenAt.String), "известени", notified)
	} else {
		m.log.Info("Устройството отново е на линия", "устройство", label, "известени", notified)
	}
}

func (m *Monitor) compose(evs []event) (subject, body string) {
	var off, on []string
	for _, e := range evs {
		name := e.label
		if name == "" {
			name = fmt.Sprintf("устройство #%d", e.dev.ID)
		}
		if e.offline {
			line := "• " + name + " — последно видяно " + m.when(e.dev.LastSeenAt.String)
			if e.dev.NextCheckAt.Valid {
				line += ", трябваше да провери в " + m.when(e.dev.NextCheckAt.String)
			}
			off = append(off, line)
		} else {
			on = append(on, "• "+name)
		}
	}
	var b strings.Builder
	switch {
	case len(off) > 0 && len(on) > 0:
		subject = fmt.Sprintf("marquee: %d офлайн, %d отново на линия", len(off), len(on))
	case len(off) == 1:
		subject = "marquee: устройство е офлайн"
	case len(off) > 1:
		subject = fmt.Sprintf("marquee: %d устройства са офлайн", len(off))
	case len(on) == 1:
		subject = "marquee: устройство отново е на линия"
	default:
		subject = fmt.Sprintf("marquee: %d устройства отново са на линия", len(on))
	}
	if len(off) > 0 {
		b.WriteString("Не се свързаха навреме:\n" + strings.Join(off, "\n") + "\n\n")
		b.WriteString("Екранът продължава да показва последното получено съдържание, но няма да получи промени, докато не се свърже. " +
			"Проверете захранването и интернета на обекта.\n\n")
	}
	if len(on) > 0 {
		b.WriteString("Отново се свързаха:\n" + strings.Join(on, "\n") + "\n\n")
	}
	b.WriteString("-- \nmarquee")
	return subject, b.String()
}

func (m *Monitor) when(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return "никога"
	}
	return t.In(m.loc).Format("02.01 15:04")
}
