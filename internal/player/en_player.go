package player

import "marquee/internal/i18n"

func init() {
	i18n.Register(map[string]string{
		"Ново устройство чака свързване":             "A new device is waiting to be paired",
		"Посещението на устройството не се записа":   "The device visit was not recorded",
		"Данните на устройството не се записаха":     "The device details were not recorded",
		"Не се записа какво е получило устройството": "What the device received was not recorded",
		"няма активни кампании":                      "no active campaigns",
		"Устройството получи ново съдържание":        "The device received new content",
		"Не се записа следващата проверка":           "The next check was not recorded",
		"Повредена кампания, пропуска се":            "Damaged campaign, skipped",
		"Невалиден график на кампания, пропуска се":  "Invalid campaign schedule, skipped",
		"Грешка при заявка от устройство":            "Error in a device request",

		// pair.html
		"Устройството е свързано": "The device is paired",
		"Очаква съдържание":       "Waiting for content",
		"Код за свързване":        "Pairing code",
		"Въведете кода в marquee": "Enter the code in marquee",
	})
}
