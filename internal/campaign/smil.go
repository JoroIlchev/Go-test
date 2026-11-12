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
	// Seed varies shuffled playlists between devices; with the date it
	// gives each device a new order every day.
	Seed uint64
}

// Segment is a stretch of time with one playlist: the items loop from Start
// until End (zero: open-ended), when the next segment takes over.
type Segment struct {
	Start, End time.Time
	Items      []PlanItem
	Audio      *store.Media // background music, looped
}

type PlanItem struct {
	Media     *store.Media
	DurationS float64 // images always; videos only when cut short (0: to the end)
	Fit       string  // meet | fill | slice
}

// Plan resolves the playables into the segments of the scheduling window
// that are not over yet, with their items in play order. Every player
// format renders from it. It also returns the version ids included.
func (b Builder) Plan(now time.Time, playables []Playable, media map[int64]*store.Media) ([]Segment, []int64) {
	from, to := Window(now, b.Loc)
	entries := make([]schedule.Entry[*Playable], len(playables))
	for i := range playables {
		entries[i] = schedule.Entry[*Playable]{Rule: playables[i].Rule, Value: &playables[i]}
	}
	included := map[int64]bool{}
	var versions []int64
	var out []Segment
	for _, seg := range schedule.Timeline(entries, from, to, b.Loc) {
		if !seg.End.IsZero() && !seg.End.After(now) {
			continue // already over
		}
		s := Segment{Start: seg.Start, End: seg.End}
		for _, p := range seg.Values {
			if !included[p.VersionID] {
				included[p.VersionID] = true
				versions = append(versions, p.VersionID)
			}
			fit := p.Spec.Fit
			if fit == "" {
				fit = "meet"
			}
			for _, i := range p.Spec.Sequence(b.seed(from, p.VersionID)) {
				it := p.Spec.Items[i]
				m := media[it.MediaID]
				if m == nil || m.Status != "ready" || (m.Kind != "image" && m.Kind != "video") {
					continue
				}
				d := it.DurationS
				if m.Kind == "image" && d <= 0 {
					d = 10
				}
				s.Items = append(s.Items, PlanItem{Media: m, DurationS: d, Fit: fit})
			}
			if s.Audio == nil && p.Spec.AudioID != 0 {
				if a := media[p.Spec.AudioID]; a != nil && a.Status == "ready" {
					s.Audio = a
				}
			}
		}
		out = append(out, s)
	}
	return out, versions
}

// Build renders the SMIL index for a device. Segments become <par> blocks in
// an <excl>: each starts on its wallclock time and stops the previous one,
// so the switch happens on the player's own clock. The segment already
// running starts at once ("0") instead of a past wallclock.
// It returns the body and the version ids included.
func (b Builder) Build(now time.Time, playables []Playable, media map[int64]*store.Media) ([]byte, []int64) {
	segments, versions := b.Plan(now, playables, media)

	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n<smil>\n<head>\n")
	fmt.Fprintf(&buf, "  <meta http-equiv=\"Refresh\" content=\"%d\"/>\n", b.RefreshSec)
	buf.WriteString("  <layout>\n" +
		"    <root-layout width=\"1920\" height=\"1080\" backgroundColor=\"black\"/>\n" +
		"    <region regionName=\"screen\" left=\"0\" top=\"0\" width=\"100%\" height=\"100%\" z-index=\"1\" backgroundColor=\"black\"/>\n" +
		"  </layout>\n</head>\n<body>\n  <excl>\n")

	for _, seg := range segments {
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
		for _, it := range seg.Items {
			b.writeItem(&buf, it)
		}
		buf.WriteString("      </seq>\n")
		if seg.Audio != nil {
			fmt.Fprintf(&buf, "      <audio src=\"%s\" repeatCount=\"indefinite\"/>\n", attr(b.MediaURL(seg.Audio)))
		}
		buf.WriteString("    </par>\n")
	}
	buf.WriteString("  </excl>\n</body>\n</smil>\n")
	return buf.Bytes(), versions
}

func (b Builder) writeItem(buf *bytes.Buffer, it PlanItem) {
	src := attr(b.MediaURL(it.Media))
	switch {
	case it.Media.Kind == "image":
		fmt.Fprintf(buf, "        <img src=\"%s\" region=\"screen\" dur=\"%ss\" fit=\"%s\"/>\n", src, num(it.DurationS), it.Fit)
	case it.DurationS > 0:
		fmt.Fprintf(buf, "        <video src=\"%s\" region=\"screen\" dur=\"%ss\" fit=\"%s\"/>\n", src, num(it.DurationS), it.Fit)
	default:
		fmt.Fprintf(buf, "        <video src=\"%s\" region=\"screen\" fit=\"%s\"/>\n", src, it.Fit)
	}
}

// seed mixes the device, the day and the version, so the order (and with it
// the index) stays the same all day and changes at midnight with the window.
func (b Builder) seed(day time.Time, version int64) uint64 {
	return b.Seed*1_000_003 + uint64(day.Year()*10000+int(day.Month())*100+day.Day())*7919 + uint64(version)
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
