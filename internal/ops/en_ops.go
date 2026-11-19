package ops

import "marquee/internal/i18n"

func init() {
	i18n.Register(map[string]string{
		"Резервното копие не успя": "The backup failed",
		"Резервно копие":           "Backup",

		"Проверката за офлайн устройства не успя":   "The check for offline devices failed",
		"имейлът не е настроен":                     "e-mail is not set up",
		"Имейлът с известие не се изпрати":          "The notification e-mail was not sent",
		"Устройството не провери навреме (офлайн)":  "The device did not check in on time (offline)",
		"Устройството отново е на линия":            "The device is back online",
		"устройство #%d":                            "device #%d",
		"последно видяно":                           "last seen",
		"трябваше да провери в":                     "was due to check at",
		"marquee: %d офлайн, %d отново на линия":    "marquee: %d offline, %d back online",
		"marquee: устройство е офлайн":              "marquee: a device is offline",
		"marquee: %d устройства са офлайн":          "marquee: %d devices are offline",
		"marquee: устройство отново е на линия":     "marquee: a device is back online",
		"marquee: %d устройства отново са на линия": "marquee: %d devices are back online",
		"Не се свързаха навреме:":                   "Did not connect on time:",
		"Екранът продължава да показва последното получено съдържание, но няма да получи промени, докато не се свърже. Проверете захранването и интернета на обекта.": "The screen keeps showing the last content it received but gets no changes until it connects. Check the power and the internet at the site.",
		"Отново се свързаха:": "Connected again:",
		"никога":              "never",
	})
}
