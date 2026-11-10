package ops

import (
	"time"

	"marquee/internal/store"
)

// Usage is one client's device usage in a period.
type Usage struct {
	ClientID   int64 `json:"client_id"`
	DeviceDays int   `json:"device_days"` // device × calendar day, active for the client at any time that day
	Devices    int   `json:"devices"`     // distinct devices active for the client at some point
	ActiveEnd  int   `json:"active_end"`  // devices active at the end of the period (or now)
}

// ComputeUsage replays the device history over [from, to). A device counts
// for a day if it was active for the client at any moment of that day, so a
// screen moved between clients mid-day counts once for each.
func ComputeUsage(events []store.HistoryEvent, from, to time.Time, loc *time.Location) map[int64]*Usage {
	type state struct {
		client int64 // 0 = not active for anyone
		since  time.Time
	}
	out := map[int64]*Usage{}
	usage := func(c int64) *Usage {
		if out[c] == nil {
			out[c] = &Usage{ClientID: c}
		}
		return out[c]
	}
	days := map[[3]int64]bool{} // client, device, day number
	devices := map[[2]int64]bool{}
	credit := func(dev int64, s state, end time.Time) {
		if s.client == 0 {
			return
		}
		start := s.since
		if start.Before(from) {
			start = from
		}
		if end.After(to) {
			end = to
		}
		if !end.After(start) {
			return
		}
		devices[[2]int64{s.client, dev}] = true
		for d := dayStart(start, loc); d.Before(end); d = d.AddDate(0, 0, 1) {
			days[[3]int64{s.client, dev, d.Unix()}] = true
		}
	}

	cur := map[int64]state{}
	for _, e := range events {
		if !e.At.Before(to) {
			break
		}
		prev := cur[e.DeviceID]
		credit(e.DeviceID, prev, e.At)
		next := state{since: e.At}
		if e.Status == "active" && e.ClientID.Valid {
			next.client = e.ClientID.Int64
		}
		cur[e.DeviceID] = next
	}
	for dev, s := range cur {
		credit(dev, s, to)
		if s.client != 0 {
			usage(s.client).ActiveEnd++
		}
	}
	for k := range days {
		usage(k[0]).DeviceDays++
	}
	for k := range devices {
		usage(k[0]).Devices++
	}
	return out
}

func dayStart(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}
