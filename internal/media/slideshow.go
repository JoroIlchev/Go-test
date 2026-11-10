package media

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Slideshow is the recipe for rendering still images into one MP4: the
// player then gets a single H.264 file with smooth transitions instead of a
// playlist of images it switches between abruptly.
type Slideshow struct {
	Items       []Slide   `json:"items"`
	AudioID     int64     `json:"audio_id,omitempty"`
	Transition  string    `json:"transition"`   // none or one of Transitions
	TransitionS float64   `json:"transition_s"` // overlap between neighbouring slides
	Motion      bool      `json:"motion"`       // slow zoom (Ken Burns)
	Background  string    `json:"background"`   // blur | black | crop
	Portrait    bool      `json:"portrait"`     // 1080x1920 instead of 1920x1080
	Text        TextStyle `json:"text_style"`
}

type Slide struct {
	MediaID   int64  `json:"media_id"`
	DurationS int    `json:"duration_s"`
	Text      string `json:"text,omitempty"`     // caption burned into the slide
	TextPos   string `json:"text_pos,omitempty"` // top | center | bottom
}

// TextStyle applies to the captions of all slides.
type TextStyle struct {
	Font  string `json:"font"`  // one of Fonts
	Size  string `json:"size"`  // s | m | l | xl
	Color string `json:"color"` // #rrggbb
	Box   string `json:"box"`   // none | dark | light
}

// Fonts maps the editor's font choices to Windows font files, all of which
// cover Cyrillic. A copy in tools\fonts next to the exe takes precedence.
var Fonts = map[string]string{
	"arial":   "arialbd.ttf",
	"segoe":   "segoeuib.ttf",
	"georgia": "georgiab.ttf",
	"impact":  "impact.ttf",
	"verdana": "verdanab.ttf",
	"comic":   "comicbd.ttf",
}

// Font sizes as a fraction of the screen's short side.
var textSizes = map[string]float64{"s": 0.045, "m": 0.065, "l": 0.09, "xl": 0.12}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

const maxTextRunes = 300

// HasText reports whether any slide carries a caption.
func (sh Slideshow) HasText() bool {
	for _, it := range sh.Items {
		if strings.TrimSpace(it.Text) != "" {
			return true
		}
	}
	return false
}

// Transitions are the xfade effects offered in the editor.
var Transitions = map[string]bool{
	"fade": true, "fadeblack": true, "dissolve": true, "slideleft": true, "smoothleft": true,
	"wipeleft": true, "circleopen": true, "zoomin": true,
}

const (
	maxSlides       = 200
	maxSlideS       = 120
	maxSlideshowS   = 30 * 60
	maxTransitionS  = 3.0
	slideshowFPS    = 30
	edgeFadeS       = 0.5
	audioFadeOutS   = 2.0
	motionZoom      = 0.12
	motionOversizeX = 2
)

func ParseSlideshow(raw string) (Slideshow, error) {
	var sh Slideshow
	err := json.Unmarshal([]byte(raw), &sh)
	return sh, err
}

func (sh Slideshow) JSON() string {
	b, _ := json.Marshal(sh)
	return string(b)
}

// Normalize fills defaults and rejects recipes the renderer cannot honour.
func (sh *Slideshow) Normalize() error {
	if len(sh.Items) == 0 {
		return errors.New("add at least one image")
	}
	if len(sh.Items) > maxSlides {
		return fmt.Errorf("at most %d images", maxSlides)
	}
	if sh.Background == "" {
		sh.Background = "blur"
	}
	if sh.Background != "blur" && sh.Background != "black" && sh.Background != "crop" {
		return fmt.Errorf("invalid background %q", sh.Background)
	}
	if sh.Transition == "" || len(sh.Items) == 1 {
		sh.Transition = "none"
	}
	if sh.Transition == "none" {
		sh.TransitionS = 0
	} else if !Transitions[sh.Transition] {
		return fmt.Errorf("invalid transition %q", sh.Transition)
	} else if sh.TransitionS <= 0 {
		sh.TransitionS = 1
	}
	if sh.TransitionS > maxTransitionS {
		return fmt.Errorf("transition longer than %gs", maxTransitionS)
	}
	st := &sh.Text
	if st.Font == "" {
		st.Font = "arial"
	}
	if st.Size == "" {
		st.Size = "m"
	}
	if st.Color == "" {
		st.Color = "#ffffff"
	}
	if st.Box == "" {
		st.Box = "dark"
	}
	if _, ok := Fonts[st.Font]; !ok {
		return fmt.Errorf("invalid font %q", st.Font)
	}
	if _, ok := textSizes[st.Size]; !ok {
		return fmt.Errorf("invalid text size %q", st.Size)
	}
	if !hexColor.MatchString(st.Color) {
		return fmt.Errorf("invalid text color %q", st.Color)
	}
	if st.Box != "none" && st.Box != "dark" && st.Box != "light" {
		return fmt.Errorf("invalid text background %q", st.Box)
	}
	total := 0
	for i := range sh.Items {
		it := &sh.Items[i]
		it.Text = strings.TrimSpace(strings.ReplaceAll(it.Text, "\r\n", "\n"))
		if utf8.RuneCountInString(it.Text) > maxTextRunes {
			return fmt.Errorf("text may be at most %d characters", maxTextRunes)
		}
		if it.Text == "" {
			it.TextPos = ""
		} else if it.TextPos == "" {
			it.TextPos = "bottom"
		} else if it.TextPos != "top" && it.TextPos != "center" && it.TextPos != "bottom" {
			return fmt.Errorf("invalid text position %q", it.TextPos)
		}
		if it.DurationS < 1 || it.DurationS > maxSlideS {
			return fmt.Errorf("image duration must be 1..%d s", maxSlideS)
		}
		if float64(it.DurationS) <= sh.TransitionS {
			return errors.New("every image must last longer than the transition")
		}
		total += it.DurationS
	}
	if total > maxSlideshowS {
		return fmt.Errorf("the video may be at most %d minutes", maxSlideshowS/60)
	}
	return nil
}

func (sh Slideshow) Size() (w, h int) {
	if sh.Portrait {
		return maxShortSide, maxLongSide
	}
	return maxLongSide, maxShortSide
}

// TotalS is the length of the rendered video: transitions overlap the
// neighbouring slides, they do not add time.
func (sh Slideshow) TotalS() float64 {
	t := 0
	for _, it := range sh.Items {
		t += it.DurationS
	}
	return float64(t)
}

// SlideshowFont is the name the caption font must have in the job directory.
const SlideshowFont = "font.ttf"

// Job is one render. ffmpeg runs with the job directory as its working
// directory, holding Files (the filter graph and one caption file per slide)
// and the caption font as SlideshowFont. Relative names keep Windows paths,
// whose drive colons clash with filter syntax, out of the graph; captions in
// files cannot break it whatever characters they contain. The graph itself is
// a file because with many slides it would exceed the command line limit.
type Job struct {
	Args  []string
	Files map[string]string
}

// Job builds the render. images are the slide files in order; audio may be empty.
func (sh Slideshow) Job(images []string, audio, out string) Job {
	w, h := sh.Size()
	n := len(sh.Items)
	t := sh.TransitionS
	total := sh.TotalS()
	files := map[string]string{}
	var g strings.Builder

	args := []string{"-v", "error", "-y"}
	for _, img := range images {
		args = append(args, "-i", img)
	}
	for i, it := range sh.Items {
		// Every slide but the last runs on under the next one's transition.
		length := float64(it.DurationS)
		if i < n-1 {
			length += t
		}
		frames := int(length*slideshowFPS + 0.5)
		// One still frame per slide, even from an animated GIF.
		in := fmt.Sprintf("[%d:v]trim=end_frame=1,", i)
		if sh.Motion {
			compose(&g, in, i, w*motionOversizeX, h*motionOversizeX, sh.Background)
			// Alternate zooming in and out so consecutive slides do not feel repetitive.
			zoom := fmt.Sprintf("1+%g*on/%d", motionZoom, frames)
			if i%2 == 1 {
				zoom = fmt.Sprintf("%g-%g*on/%d", 1+motionZoom, motionZoom, frames)
			}
			fmt.Fprintf(&g, ",zoompan=z='%s':x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':d=%d:s=%dx%d:fps=%d",
				zoom, frames, w, h, slideshowFPS)
		} else {
			compose(&g, in, i, w, h, sh.Background)
			fmt.Fprintf(&g, ",loop=loop=%d:size=1:start=0,setpts=N/%d/TB", frames-1, slideshowFPS)
		}
		// The caption goes on after the zoom so it stays still and sharp.
		if it.Text != "" {
			name := fmt.Sprintf("t%d.txt", i)
			files[name] = sh.caption(&g, name, it, w, h)
		}
		fmt.Fprintf(&g, ",fps=%d,settb=1/%d,setsar=1,format=yuv420p[s%d];\n", slideshowFPS, slideshowFPS, i)
	}

	last := "[s0]"
	if n > 1 && sh.Transition == "none" {
		for i := range n {
			fmt.Fprintf(&g, "[s%d]", i)
		}
		fmt.Fprintf(&g, "concat=n=%d:v=1:a=0[x];\n", n)
		last = "[x]"
	} else if n > 1 {
		offset := 0.0
		for i := 1; i < n; i++ {
			offset += float64(sh.Items[i-1].DurationS)
			next := fmt.Sprintf("[x%d]", i)
			fmt.Fprintf(&g, "%s[s%d]xfade=transition=%s:duration=%s:offset=%s%s;\n",
				last, i, sh.Transition, num(t), num(offset), next)
			last = next
		}
	}
	// Fade in from and out to black so the loop on the screen restarts cleanly.
	fmt.Fprintf(&g, "%sfade=t=in:st=0:d=%s,fade=t=out:st=%s:d=%s[v]",
		last, num(edgeFadeS), num(total-edgeFadeS), num(edgeFadeS))

	if audio != "" {
		args = append(args, "-stream_loop", "-1", "-i", audio)
		fmt.Fprintf(&g, ";\n[%d:a]atrim=0:%s,asetpts=N/SR/TB,afade=t=out:st=%s:d=%s,aresample=48000[a]",
			n, num(total), num(max(0, total-audioFadeOutS)), num(min(audioFadeOutS, total)))
	}
	files["graph.txt"] = g.String()

	args = append(args, "-/filter_complex", "graph.txt", "-map", "[v]")
	if audio != "" {
		args = append(args, "-map", "[a]", "-c:a", "aac", "-b:a", "128k", "-ac", "2")
	}
	args = append(args,
		"-c:v", "libx264", "-preset", "medium", "-crf", "20",
		"-profile:v", "high", "-level:v", "4.1", "-pix_fmt", "yuv420p",
		"-r", strconv.Itoa(slideshowFPS), "-g", strconv.Itoa(2*slideshowFPS),
		"-t", num(total), "-movflags", "+faststart", out)
	return Job{Args: args, Files: files}
}

// caption appends the drawtext step for a slide and returns the text to put
// in the caption file, wrapped to fit the screen width.
func (sh Slideshow) caption(g *strings.Builder, file string, it Slide, w, h int) string {
	st := sh.Text
	fs := int(textSizes[st.Size] * float64(min(w, h)))
	pad := fs / 2
	margin := int(0.06*float64(h)) + pad
	y := map[string]string{
		"top":    strconv.Itoa(margin),
		"center": "(h-text_h)/2",
		"bottom": fmt.Sprintf("h-text_h-%d", margin),
	}[it.TextPos]
	fmt.Fprintf(g, ",drawtext=fontfile=%s:textfile=%s:expansion=none:fontsize=%d:fontcolor=0x%s"+
		":text_align=C:line_spacing=%d:x=(w-text_w)/2:y=%s",
		SlideshowFont, file, fs, strings.TrimPrefix(st.Color, "#"), fs/4, y)
	switch st.Box {
	case "dark":
		fmt.Fprintf(g, ":box=1:boxcolor=black@0.55:boxborderw=%d", pad)
	case "light":
		fmt.Fprintf(g, ":box=1:boxcolor=white@0.75:boxborderw=%d", pad)
	default:
		fmt.Fprintf(g, ":borderw=%d:bordercolor=black@0.6:shadowx=%d:shadowy=%d:shadowcolor=black@0.5",
			max(2, fs/24), max(2, fs/16), max(2, fs/16))
	}
	// Average glyph width of a bold font is a bit over half its size.
	return wrap(it.Text, int(float64(w)*0.86/(float64(fs)*0.58)))
}

// wrap breaks each line of s at word boundaries so no line exceeds width runes
// (a single longer word stays whole).
func wrap(s string, width int) string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			switch {
			case line == "":
				line = word
			case utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) <= width:
				line += " " + word
			default:
				out = append(out, line)
				line = word
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// compose writes the chain that places one image on a w x h canvas.
func compose(g *strings.Builder, in string, i, w, h int, background string) {
	switch background {
	case "crop":
		fmt.Fprintf(g, "%sscale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d", in, w, h, w, h)
	case "black":
		fmt.Fprintf(g, "%sscale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black",
			in, w, h, w, h)
	default:
		// The picture whole, over a blurred and darkened copy filling the screen.
		// Blurring a small copy is much cheaper than blurring at full size.
		fmt.Fprintf(g, "%sformat=yuv420p,split=2[bg%d][fg%d];\n", in, i, i)
		fmt.Fprintf(g, "[bg%d]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,boxblur=6:2,eq=brightness=-0.12,scale=%d:%d[bb%d];\n",
			i, w/8, h/8, w/8, h/8, w, h, i)
		fmt.Fprintf(g, "[fg%d]scale=%d:%d:force_original_aspect_ratio=decrease[ff%d];\n", i, w, h, i)
		fmt.Fprintf(g, "[bb%d][ff%d]overlay=(W-w)/2:(H-h)/2", i, i)
	}
}

func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
