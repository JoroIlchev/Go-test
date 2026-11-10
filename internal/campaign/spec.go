// Package campaign defines what a campaign contains (its spec), validates it
// and renders the SMIL index a player receives.
package campaign

import (
	"encoding/json"
	"fmt"
	"time"

	"marquee/internal/schedule"
)

// Spec is a campaign's content and schedule. Drafts and versions store it as JSON.
type Spec struct {
	Items    []Item  `json:"items"`
	AudioID  int64   `json:"audio_id,omitempty"`
	Fit      string  `json:"fit"`     // meet (whole picture, black bars) | fill (stretch) | slice (crop)
	Targets  []int64 `json:"targets"` // nodes the campaign plays on (and everything below them)
	Start    string  `json:"start"`   // "2006-01-02T15:04" local time; empty = immediately
	End      string  `json:"end"`     // empty = no end
	Days     []int   `json:"days"`    // ISO weekdays; empty = every day
	TimeFrom string  `json:"time_from"`
	TimeTo   string  `json:"time_to"`
	Priority int     `json:"priority"` // 0 normal, 1 high (e.g. a sale that replaces normal content)
}

type Item struct {
	MediaID   int64   `json:"media_id"`
	DurationS float64 `json:"duration_s"` // images; videos play to their end when 0
}

const localLayout = "2006-01-02T15:04"

func ParseSpec(raw string) (Spec, error) {
	var s Spec
	if raw == "" || raw == "{}" {
		return Spec{Fit: "meet", Items: []Item{}, Targets: []int64{}}, nil
	}
	err := json.Unmarshal([]byte(raw), &s)
	if s.Items == nil {
		s.Items = []Item{}
	}
	if s.Targets == nil {
		s.Targets = []int64{}
	}
	return s, err
}

func (s Spec) JSON() string {
	b, _ := json.Marshal(s)
	return string(b)
}

// MediaIDs lists every media item the spec references.
func (s Spec) MediaIDs() []int64 {
	seen := map[int64]bool{}
	var out []int64
	add := func(id int64) {
		if id != 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, it := range s.Items {
		add(it.MediaID)
	}
	add(s.AudioID)
	return out
}

// Rule converts the schedule fields, interpreting dates in loc.
func (s Spec) Rule(loc *time.Location) (schedule.Rule, error) {
	r := schedule.Rule{Days: s.Days, TimeFrom: s.TimeFrom, TimeTo: s.TimeTo, Priority: s.Priority}
	var err error
	if s.Start != "" {
		if r.Start, err = time.ParseInLocation(localLayout, s.Start, loc); err != nil {
			return r, fmt.Errorf("invalid start %q", s.Start)
		}
	}
	if s.End != "" {
		if r.End, err = time.ParseInLocation(localLayout, s.End, loc); err != nil {
			return r, fmt.Errorf("invalid end %q", s.End)
		}
	}
	return r, r.Validate()
}

// ValidateDraft checks what must hold even for a work in progress.
func (s Spec) ValidateDraft(loc *time.Location) error {
	switch s.Fit {
	case "", "meet", "fill", "slice":
	default:
		return fmt.Errorf("invalid fit %q", s.Fit)
	}
	if s.Priority < 0 || s.Priority > 1 {
		return fmt.Errorf("priority must be 0 or 1")
	}
	for _, it := range s.Items {
		if it.DurationS < 0 || it.DurationS > 3600 {
			return fmt.Errorf("duration must be between 0 and 3600 seconds")
		}
	}
	_, err := s.Rule(loc)
	return err
}

// ValidatePublish additionally requires a playable, targeted campaign.
func (s Spec) ValidatePublish(loc *time.Location) error {
	if err := s.ValidateDraft(loc); err != nil {
		return err
	}
	if len(s.Items) == 0 {
		return fmt.Errorf("the playlist is empty")
	}
	if len(s.Targets) == 0 {
		return fmt.Errorf("choose where the campaign plays")
	}
	return nil
}
