package media

import (
	"fmt"
	"strings"
)

// Warning is one compatibility finding. Level "error" means the file will most
// likely not play on an Android 7 player; "warn" means it may stutter or fail
// on weaker devices.
type Warning struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Limits for SMIL players on Android 7.1 boxes. Conservative on purpose: the
// cheapest devices decode 1080p30 H.264 reliably and little beyond that.
const (
	maxLongSide    = 1920
	maxShortSide   = 1080
	maxFPS         = 30.5
	maxBitrate     = 12_000_000
	maxImagePixels = 3840 * 2160
)

func CheckVideo(ext string, p *ProbeResult) []Warning {
	var w []Warning
	add := func(level, code, format string, args ...any) {
		w = append(w, Warning{Level: level, Code: code, Message: fmt.Sprintf(format, args...)})
	}
	if ext != "mp4" && ext != "m4v" {
		add("warn", "container", "Формат %s: за плейърите се препоръчва MP4.", strings.ToUpper(ext))
	}
	switch p.VideoCodec {
	case "h264":
		if strings.Contains(p.Profile, "10") || strings.Contains(p.Profile, "4:2:2") || strings.Contains(p.Profile, "4:4:4") {
			add("error", "profile", "H.264 профил %q не се декодира хардуерно на повечето Android устройства.", p.Profile)
		}
	case "hevc":
		add("error", "codec", "Видеото е H.265 (HEVC): много Android 7 устройства не могат да го пуснат.")
	case "":
		add("error", "codec", "Във файла няма видео поток.")
	default:
		add("error", "codec", "Видео кодек %s: за Android 7 се препоръчва H.264.", strings.ToUpper(p.VideoCodec))
	}
	if p.PixFmt != "" && p.PixFmt != "yuv420p" && p.PixFmt != "yuvj420p" {
		add("warn", "pixfmt", "Цветови формат %s: по-сигурен е yuv420p.", p.PixFmt)
	}
	long, short := max(p.Width, p.Height), min(p.Width, p.Height)
	if long > maxLongSide || short > maxShortSide {
		add("error", "resolution", "Резолюция %d×%d: над Full HD е твърде тежко за Android 7 устройствата.", p.Width, p.Height)
	}
	if p.FPS > maxFPS {
		add("warn", "fps", "%.0f кадъра в секунда: по-слабите устройства може да насичат над 30.", p.FPS)
	}
	if p.Bitrate > maxBitrate {
		add("warn", "bitrate", "Битрейт %.1f Mbps: висок за евтините устройства (до 12 Mbps е сигурно).", float64(p.Bitrate)/1e6)
	}
	if p.AudioCodec != "" && p.AudioCodec != "aac" && p.AudioCodec != "mp3" {
		add("warn", "audio", "Аудио кодек %s: за Android се препоръчва AAC.", strings.ToUpper(p.AudioCodec))
	}
	return w
}

func CheckAudio(ext string, p *ProbeResult) []Warning {
	if p.AudioCodec == "" {
		return []Warning{{Level: "error", Code: "codec", Message: "Във файла няма аудио поток."}}
	}
	if p.AudioCodec != "mp3" && p.AudioCodec != "aac" {
		return []Warning{{Level: "warn", Code: "audio",
			Message: fmt.Sprintf("Аудио %s: най-сигурни са MP3 и AAC.", strings.ToUpper(p.AudioCodec))}}
	}
	return nil
}

func CheckImage(ext string, width, height int) []Warning {
	var w []Warning
	if ext == "webp" {
		w = append(w, Warning{Level: "warn", Code: "format",
			Message: "WebP не се поддържа сигурно от всички плейъри; по-сигурни са JPG и PNG."})
	}
	if width*height > maxImagePixels {
		w = append(w, Warning{Level: "warn", Code: "resolution",
			Message: fmt.Sprintf("Изображение %d×%d: много голямо, може да се зарежда бавно. Достатъчно е 1920×1080.", width, height)})
	}
	return w
}

// Convertible reports whether converting the video would fix its warnings.
func Convertible(kind string, w []Warning) bool {
	return kind == "video" && len(w) > 0
}
