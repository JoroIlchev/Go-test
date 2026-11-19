package main

import "marquee/internal/i18n"

func init() {
	i18n.Register(map[string]string{
		"Спиране заради грешка": "Stopping because of an error",
		"Още няма потребители: създайте първия с marquee.exe create-super -email vie@firma.bg": "No users yet: create the first one with marquee.exe create-super -email you@company.com",
		"Няма ffmpeg: видеата се качват, но не се проверяват и конвертират":                    "No ffmpeg: videos are uploaded but not checked or converted",
		"сложете ffmpeg.exe и ffprobe.exe в папка tools до marquee.exe":                        "put ffmpeg.exe and ffprobe.exe into a tools folder next to marquee.exe",
		"ffmpeg е наличен":                    "ffmpeg is available",
		"устройства":                          "devices",
		"админ панел":                         "admin panel",
		"устройства по HTTPS":                 "devices over HTTPS",
		"устройства и админ панел по HTTPS":   "devices and admin panel over HTTPS",
		"сертификат и пренасочване към HTTPS": "certificate and redirect to HTTPS",
		"HTTPS е включен":                     "HTTPS is on",
		"Слуша за":                            "Listening for",
		"marquee е стартиран":                 "marquee has started",
		"Адрес за устройствата е вътрешен":    "The device address is internal",
		"публичният IP се задава в Настройки": "set the public IP in Settings",
		"Адрес за устройствата":               "Device address",
		"Админ панел":                         "Admin panel",
		"Услугата стартира":                   "The service has started",
		"Услугата е спряна":                   "The service has stopped",
		"Услугата спря с грешка":              "The service stopped with an error",
	})
}
