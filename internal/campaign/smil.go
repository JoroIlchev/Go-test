package campaign

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"time"

	"marquee/internal/schedule"
	"marquee/internal/store"
)

// Playable is one live campaign version resolved for a device.
type Playable struct {
	VersionID int64
	Spec      Spec
	Rule      schedule.Rule
}

// Horizon is how far ahead the index schedules. It is anchored to local
// midnight so the index body (and its ETag) changes at most once a day plus
// whenever content changes, not on every poll.
const Horizon = 7 * 24 * time.Hour

// Window returns the scheduling window for now: today's local midnight + Horizon.
func Window(now time.Time, loc *time.Location) (from, to time.Time) {
	l := now.In(loc)
	from = time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, loc)
	return from, from.Add(Horizon)
}

type Builder struct {
	Loc        *time.Location
	MediaURL   func(m *store.Media) string
	RefreshSec int
}

// Build renders the SMIL index for a device. Segments become <par> blocks in
// an <excl>: each starts on its wallclock time and stops the previous one,
// so the switch happens on the player's own clock. The segment already
// running starts at once ("0") instead of a past wallclock.
// It returns the body and the version ids included.
func (b Builder) Build(now time.Time, playables []Playable, media map[int64]*store.Media) ([]byte, []int64) {
	from, to := Window(now, b.Loc)
	entries := make([]schedule.Entry[*Playable], len(playables))
	for i := range playables {
		entries[i] = schedule.Entry[*Playable]{Rule: playables[i].Rule, Value: &playables[i]}
	}
	segments := schedule.Timeline(entries, from, to, b.Loc)

	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n<smil>\n<head>\n")
	fmt.Fprintf(&buf, "  <meta http-equiv=\"Refresh\" content=\"%d\"/>\n", b.RefreshSec)
	buf.WriteString("  <layout>\n" +
		"    <root-layout width=\"1920\" height=\"1080\" backgroundColor=\"black\"/>\n" +
		"    <region regionName=\"screen\" left=\"0\" top=\"0\" width=\"100%\" height=\"100%\" z-index=\"1\" backgroundColor=\"black\"/>\n" +
		"  </layout>\n</head>\n<body>\n  <excl>\n")

	included := map[int64]bool{}
	var versions []int64
	for _, seg := range segments {
		if !seg.End.IsZero() && !seg.End.After(now) {
			continue // already over
		}
		begin := "0"
		if seg.Start.After(now) {
			begin = wallclock(seg.Start, b.Loc)
		}
		end := ""
		if !seg.End.IsZero() {
			end = ` end="` + wallclock(seg.End, b.Loc) + `"`
		}
		fmt.Fprintf(&buf, "    <par begin=\"%s\"%s>\n", begin, end)
		buf.WriteString("      <seq repeatCount=\"indefinite\">\n")
		var audio *store.Media
		for _, p := range seg.Values {
			if !included[p.VersionID] {
				included[p.VersionID] = true
				versions = append(versions, p.VersionID)
			}
			fit := p.Spec.Fit
			if fit == "" {
				fit = "meet"
			}
			for _, it := range p.Spec.Items {
				m := media[it.MediaID]
				if m == nil || m.Status != "ready" {
					continue
				}
				b.writeItem(&buf, m, it, fit)
			}
			if audio == nil && p.Spec.AudioID != 0 {
				if a := media[p.Spec.AudioID]; a != nil && a.Status == "ready" {
					audio = a
				}
			}
		}
		buf.WriteString("      </seq>\n")
		if audio != nil {
			fmt.Fprintf(&buf, "      <audio src=\"%s\" repeatCount=\"indefinite\"/>\n", attr(b.MediaURL(audio)))
		}
		buf.WriteString("    </par>\n")
	}
	buf.WriteString("  </excl>\n</body>\n</smil>\n")
	return buf.Bytes(), versions
}

func (b Builder) writeItem(buf *bytes.Buffer, m *store.Media, it Item, fit string) {
	src := attr(b.MediaURL(m))
	switch m.Kind {
	case "image":
		d := it.DurationS
		if d <= 0 {
			d = 10
		}
		fmt.Fprintf(buf, "        <img src=\"%s\" region=\"screen\" dur=\"%ss\" fit=\"%s\"/>\n", src, num(d), fit)
	case "video":
		if it.DurationS > 0 {
			fmt.Fprintf(buf, "        <video src=\"%s\" region=\"screen\" dur=\"%ss\" fit=\"%s\"/>\n", src, num(it.DurationS), fit)
		} else {
			fmt.Fprintf(buf, "        <video src=\"%s\" region=\"screen\" fit=\"%s\"/>\n", src, fit)
		}
	}
}

// wallclock uses an explicit UTC offset so the player cannot misread the zone.
func wallclock(t time.Time, loc *time.Location) string {
	return "wallclock(" + t.In(loc).Format("2006-01-02T15:04:05-07:00") + ")"
}

func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func attr(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}
