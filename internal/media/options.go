package media

import (
	"fmt"
	"strconv"
)

// Options say how new uploads are prepared for the players. The super admin
// picks them in the hub settings; they apply to files processed afterwards.
type Options struct {
	// ImageW x ImageH is the screen new images are shrunk to fit; 0 = keep
	// the original size.
	ImageW, ImageH int
	ImageQ         int // ffmpeg JPEG quality, 2 (best) .. 31

	// VideoAuto converts uploads in place: "off" (only the button), "problems"
	// (when a check failed) or "always" (also to make files smaller).
	VideoAuto  string
	VideoShort int // the short side of converted videos: 1080 or 720
	VideoCRF   int // x264 quality: lower is better and bigger
}

func DefaultOptions() Options {
	return Options{ImageW: 1920, ImageH: 1080, ImageQ: imageQ["normal"], VideoAuto: "off",
		VideoShort: 1080, VideoCRF: videoCRF["normal"]}
}

// Prep is the processing choice as the admin forms send it: the hub
// settings hold the default, an upload may carry its own.
type Prep struct {
	ImageScreen  string `json:"image_screen"`  // "1920x1080", "1080x1920", "3840x2160" or "off"
	ImageQuality string `json:"image_quality"` // high | normal | small
	VideoAuto    string `json:"video_auto"`    // off | problems | always
	VideoMax     int    `json:"video_max"`     // 1080 | 720
	VideoQuality string `json:"video_quality"` // high | normal | small
}

func (p Prep) Options() (Options, error) {
	return OptionsFrom(p.ImageScreen, p.ImageQuality, p.VideoAuto, p.VideoMax, p.VideoQuality)
}

var (
	imageQ   = map[string]int{"high": 2, "normal": 4, "small": 7}
	videoCRF = map[string]int{"high": 19, "normal": 21, "small": 25}
)

// OptionsFrom builds Options from the settings form: screen is "WxH" or
// "off", the qualities are high|normal|small.
func OptionsFrom(screen, imageQuality, videoAuto string, videoShort int, videoQuality string) (Options, error) {
	o := DefaultOptions()
	if screen == "off" {
		o.ImageW, o.ImageH = 0, 0
	} else if n, _ := fmt.Sscanf(screen, "%dx%d", &o.ImageW, &o.ImageH); n != 2 || o.ImageW < 320 || o.ImageH < 320 || o.ImageW > 7680 || o.ImageH > 7680 {
		return o, fmt.Errorf("invalid screen size %q", screen)
	}
	var ok bool
	if o.ImageQ, ok = imageQ[imageQuality]; !ok {
		return o, fmt.Errorf("invalid image quality %q", imageQuality)
	}
	switch videoAuto {
	case "off", "problems", "always":
		o.VideoAuto = videoAuto
	default:
		return o, fmt.Errorf("invalid video mode %q", videoAuto)
	}
	if videoShort != 1080 && videoShort != 720 {
		return o, fmt.Errorf("invalid video size %d", videoShort)
	}
	o.VideoShort = videoShort
	if o.VideoCRF, ok = videoCRF[videoQuality]; !ok {
		return o, fmt.Errorf("invalid video quality %q", videoQuality)
	}
	return o, nil
}

// fits reports whether a w x h image already fits the screen.
func (o Options) fits(w, h int) bool {
	return o.ImageW == 0 || (w <= o.ImageW && h <= o.ImageH)
}

// imageArgs shrinks src into the screen box, keeping the aspect ratio.
func (o Options) imageArgs(src, dst string) []string {
	scale := "scale=" + strconv.Itoa(o.ImageW) + ":" + strconv.Itoa(o.ImageH) + ":force_original_aspect_ratio=decrease"
	return []string{"-v", "error", "-y", "-i", src, "-vf", scale, "-frames:v", "1", "-q:v", strconv.Itoa(o.ImageQ), dst}
}

// videoArgs re-encodes src to H.264/AAC MP4 within the size limit and 30 fps,
// in limited-range BT.709 colour: full-range ("yuvj") video, which ffmpeg
// makes from JPEG photos, shows black or frozen on some hardware decoders.
func (o Options) videoArgs(src, dst string, portrait bool) []string {
	short, long := o.VideoShort, o.VideoShort*16/9
	boxW, boxH := long, short
	if portrait {
		boxW, boxH = short, long
	}
	scale := "scale='min(" + strconv.Itoa(boxW) + ",iw)':'min(" + strconv.Itoa(boxH) + ",ih)'" +
		":force_original_aspect_ratio=decrease:force_divisible_by=2:out_range=tv:out_color_matrix=bt709,format=yuv420p"
	return append([]string{"-v", "error", "-y", "-i", src,
		"-map", "0:v:0", "-map", "0:a:0?",
		"-c:v", "libx264", "-preset", "medium", "-crf", strconv.Itoa(o.VideoCRF),
		"-profile:v", "high", "-level:v", "4.1", "-pix_fmt", "yuv420p",
		"-maxrate", "8M", "-bufsize", "16M", "-fpsmax", "30", "-vf", scale},
		append(colorArgs(), "-c:a", "aac", "-b:a", "128k", "-ac", "2",
			"-movflags", "+faststart", dst)...)
}

// colorArgs tag the output as limited-range BT.709, what every decoder expects.
func colorArgs() []string {
	return []string{"-color_range", "tv", "-colorspace", "bt709", "-color_primaries", "bt709", "-color_trc", "bt709"}
}
