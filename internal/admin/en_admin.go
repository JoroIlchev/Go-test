package admin

import "marquee/internal/i18n"

func init() {
	i18n.Register(map[string]string{
		// Audit labels (actionLabels).
		"Създадена кампания":                 "Campaign created",
		"Публикувана кампания":               "Campaign published",
		"Одобрена кампания":                  "Campaign approved",
		"Отхвърлена кампания":                "Campaign rejected",
		"Сменена версия на живо":             "Live version changed",
		"Изтрита кампания":                   "Campaign deleted",
		"Качен файл":                         "File uploaded",
		"Изтрит файл":                        "File deleted",
		"Конвертиране на видео":              "Video conversion",
		"Видео от снимки":                    "Video from images",
		"Създадено звено":                    "Node created",
		"Преименувано звено":                 "Node renamed",
		"Изтрито звено":                      "Node deleted",
		"Сменен график на проверките":        "Check schedule changed",
		"Свързано устройство":                "Device paired",
		"Добавено устройство":                "Device added",
		"Променено устройство":               "Device changed",
		"Създаден потребител":                "User created",
		"Променен потребител":                "User changed",
		"Добавен в екипа":                    "Added to the team",
		"Променен член на екипа":             "Team member changed",
		"Променени настройки":                "Settings changed",
		"Променена проверка по подразбиране": "Default check changed",
		"Ръчно резервно копие":               "Manual backup",

		"по подразбиране (-refresh)":                            "default (-refresh)",
		"Списъкът с резервни копия не се чете":                  "The list of backups cannot be read",
		"marquee: пробно писмо":                                 "marquee: test e-mail",
		"Ако четете това, известията от marquee стигат до вас.": "If you are reading this, marquee notifications reach you.",
		"Клиент":                           "Client",
		"Устройство-дни":                   "Device-days",
		"Устройства през месеца":           "Devices during the month",
		"Активни в края":                   "Active at month end",
		"Слайдшоу":                         "Slideshow",
		"Неуспешен вход":                   "Failed sign-in",
		"Вход в админ панела":              "Signed in to the admin panel",
		"Грешка в админ заявка":            "Error in an admin request",
		"Експортът на кампания не завърши": "Campaign export did not finish",
		"файла": "files",
	})
}
