package store

import (
	"context"
	"encoding/json"
)

// DeviceInfo is what Marquee Player reports about the device it runs on.
// Zero values mean "unknown".
type DeviceInfo struct {
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Android      string `json:"android"` // e.g. "7.1.1"
	SDK          int    `json:"sdk"`
	App          string `json:"app"` // player version
	ScreenW      int    `json:"screen_w"`
	ScreenH      int    `json:"screen_h"`
	Density      int    `json:"density"`
	// The largest video the hardware decoder plays smoothly.
	VideoMaxW int  `json:"video_max_w"`
	VideoMaxH int  `json:"video_max_h"`
	HEVC      bool `json:"hevc"`
	// Storage the player may use for its cache.
	FreeBytes  int64  `json:"free_bytes"`
	TotalBytes int64  `json:"total_bytes"`
	LocalIP    string `json:"local_ip"`
	UptimeS    int64  `json:"uptime_s"`
}

// ParsedInfo decodes what Marquee Player reported; nil for other players and unknown data.
func (d *Device) ParsedInfo() *DeviceInfo {
	if d.Info == "" {
		return nil
	}
	var i DeviceInfo
	if json.Unmarshal([]byte(d.Info), &i) != nil {
		return nil
	}
	return &i
}

func (s *Store) SetDeviceInfo(ctx context.Context, id int64, info DeviceInfo) error {
	b, err := json.Marshal(info)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE devices SET info = ? WHERE id = ?`, string(b), id)
	return err
}
