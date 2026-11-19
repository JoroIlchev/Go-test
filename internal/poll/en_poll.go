package poll

import "marquee/internal/i18n"

func init() {
	i18n.Register(map[string]string{
		"в": "at",
		" (разпределени в %d мин)": " (spread over %d min)",
		"на всеки %d мин":          "every %d min",
	})
}
