// Package poll decides when a player should next ask for its index.
//
// SMIL players re-read the index after the number of seconds in the SMIL's
// <meta http-equiv="Refresh">, so every policy comes down to that number. A
// campaign switch never depends on it (the SMIL carries the exact switch
// times); polling only decides how soon a newly published change arrives.
package poll

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Players ignore refresh values below a minute.
const MinRefreshSec = 60

type Policy struct {
	Mode      string   `json:"mode"`       // interval | times
	Minutes   int      `json:"minutes"`    // interval mode
	Times     []string `json:"times"`      // times mode, "HH:MM" local
	SpreadMin int      `json:"spread_min"` // times mode: devices spread over this many minutes after each time
}

func Interval(d time.Duration) Policy {
	return Policy{Mode: "interval", Minutes: max(1, int(d.Minutes()))}
}

// Normalize validates p and puts its times in order.
func (p *Policy) Normalize() error {
	switch p.Mode {
	case "interval":
		if p.Minutes < 1 || p.Minutes > 24*60 {
			return errors.New("interval must be 1..1440 minutes")
		}
		p.Times, p.SpreadMin = nil, 0
	case "times":
		if len(p.Times) == 0 || len(p.Times) > 48 {
			return errors.New("give 1..48 check times")
		}
		for i, t := range p.Times {
			m, err := clock(t)
			if err != nil {
				return err
			}
			p.Times[i] = fmt.Sprintf("%02d:%02d", m/60, m%60)
		}
		slices.Sort(p.Times)
		p.Times = slices.Compact(p.Times)
		if p.SpreadMin < 0 || p.SpreadMin > 120 {
			return errors.New("spread must be 0..120 minutes")
		}
		p.Minutes = 0
	default:
		return fmt.Errorf("invalid mode %q", p.Mode)
	}
	return nil
}

// Next returns when device should check next, after now. In times mode each
// device keeps a fixed offset within the spread window, so a thousand screens
// do not all arrive at 00:00:00, and the same screen always comes at the same
// minute (easy to reason about when publishing).
func (p Policy) Next(now time.Time, loc *time.Location, deviceID int64) time.Time {
	if p.Mode != "times" || len(p.Times) == 0 {
		return now.Add(time.Duration(max(p.Minutes, 1)) * time.Minute)
	}
	offset := time.Duration(0)
	if p.SpreadMin > 0 {
		h := fnv.New32a()
		h.Write([]byte(strconv.FormatInt(deviceID, 10)))
		offset = time.Duration(h.Sum32()%uint32(p.SpreadMin*60)) * time.Second
	}
	earliest := now.Add(MinRefreshSec * time.Second)
	local := now.In(loc)
	for day := 0; day <= 2; day++ {
		y, mo, d := local.AddDate(0, 0, day).Date()
		for _, t := range p.Times {
			m, _ := clock(t)
			at := time.Date(y, mo, d, m/60, m%60, 0, 0, loc).Add(offset)
			if !at.Before(earliest) {
				return at
			}
		}
	}
	return now.Add(24 * time.Hour) // unreachable with at least one time
}

// RefreshSec is the value for the index's Refresh meta.
func (p Policy) RefreshSec(now time.Time, loc *time.Location, deviceID int64) int {
	s := int(p.Next(now, loc, deviceID).Sub(now).Seconds() + 0.5)
	return max(s, MinRefreshSec)
}

// Resolve picks the policy for a node: the one set on the nearest node of
// chain (root first, the node itself last), else def. from is the node it
// came from, 0 for def.
func Resolve(chain []int64, set map[int64]string, def Policy) (p Policy, from int64) {
	for i := len(chain) - 1; i >= 0; i-- {
		raw, ok := set[chain[i]]
		if !ok {
			continue
		}
		var q Policy
		if json.Unmarshal([]byte(raw), &q) == nil && q.Normalize() == nil {
			return q, chain[i]
		}
	}
	return def, 0
}

func (p Policy) JSON() string {
	b, _ := json.Marshal(p)
	return string(b)
}

// Describe is a short Bulgarian summary for the UI and logs.
func (p Policy) Describe() string {
	if p.Mode == "times" {
		s := "в " + strings.Join(p.Times, ", ")
		if p.SpreadMin > 0 {
			s += fmt.Sprintf(" (разпределени в %d мин)", p.SpreadMin)
		}
		return s
	}
	return fmt.Sprintf("на всеки %d мин", p.Minutes)
}

func clock(s string) (int, error) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if !ok || err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, fmt.Errorf("invalid time %q", s)
	}
	return hh*60 + mm, nil
}
