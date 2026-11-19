package logx

import "marquee/internal/i18n"

// Messages and keys are translated when a line is written (see Handler), so
// every package registers the English of its own log texts.
func init() {
	i18n.Register(map[string]string{
		"ГРЕШКА":   "ERROR",
		"ВНИМАНИЕ": "WARN",
		"ИНФО":     "INFO",
		"ДЕБЪГ":    "DEBUG",
		"да":       "yes",
		"не":       "no",

		// Keys used across the hub.
		"грешка":               "error",
		"решение":              "fix",
		"път":                  "path",
		"домейн":               "domain",
		"админ отвън":          "public admin",
		"адрес":                "address",
		"данни":                "data",
		"потребител":           "user",
		"звено":                "node",
		"какво":                "detail",
		"имейл":                "email",
		"кампания":             "campaign",
		"кампании":             "campaigns",
		"файл":                 "file",
		"снимка":               "image",
		"снимки":               "images",
		"беше":                 "was",
		"видео":                "video",
		"резултат":             "result",
		"секунди":              "seconds",
		"нови медийни файлове": "new media files",
		"до":                   "to",
		"устройство":           "device",
		"последно видяно":      "last seen",
		"известени":            "notified",
		"код":                  "code",
		"име":                  "name",
		"версия":               "version",
		"следваща проверка":    "next check",
	})
}
