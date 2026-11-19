package media

import "marquee/internal/i18n"

// Warnings and errors are stored with the file in the hub language of the
// moment it was processed; their codes stay the same in every language.
func init() {
	i18n.Register(map[string]string{
		"Формат %s: за плейърите се препоръчва MP4.":                                                                       "Format %s: MP4 is recommended for players.",
		"H.264 профил %q не се декодира хардуерно на повечето Android устройства.":                                         "H.264 profile %q is not hardware-decoded on most Android devices.",
		"Видеото е H.265 (HEVC): много Android 7 устройства не могат да го пуснат.":                                        "The video is H.265 (HEVC): many Android 7 devices cannot play it.",
		"Във файла няма видео поток.":                                                                                      "The file has no video stream.",
		"Видео кодек %s: за Android 7 се препоръчва H.264.":                                                                "Video codec %s: H.264 is recommended for Android 7.",
		"Цветови формат yuvj420p (пълен диапазон): някои устройства показват черен или замръзнал кадър. Конвертирайте го.": "Pixel format yuvj420p (full range): some devices show a black or frozen frame. Convert it.",
		"Цветови формат %s: по-сигурен е yuv420p.":                                                                         "Pixel format %s: yuv420p is safer.",
		"Резолюция %d×%d: над Full HD е твърде тежко за Android 7 устройствата.":                                           "Resolution %d×%d: above Full HD is too heavy for Android 7 devices.",
		"%.0f кадъра в секунда: по-слабите устройства може да насичат над 30.":                                             "%.0f frames per second: weaker devices may stutter above 30.",
		"Битрейт %.1f Mbps: висок за евтините устройства (до 12 Mbps е сигурно).":                                          "Bitrate %.1f Mbps: high for cheap devices (up to 12 Mbps is safe).",
		"Аудио кодек %s: за Android се препоръчва AAC.":                                                                    "Audio codec %s: AAC is recommended for Android.",
		"Във файла няма аудио поток.":                                                                                      "The file has no audio stream.",
		"Аудио %s: най-сигурни са MP3 и AAC.":                                                                              "Audio %s: MP3 and AAC are the safest.",
		"WebP не се поддържа сигурно от всички плейъри; по-сигурни са JPG и PNG.":                                          "WebP is not reliably supported by all players; JPG and PNG are safer.",
		"Изображение %d×%d: много голямо, може да се зарежда бавно. Достатъчно е 1920×1080.":                               "Image %d×%d: very large, it may load slowly. 1920×1080 is enough.",

		"Опашката за обработка е пълна; файлът ще се обработи след рестарт":     "The processing queue is full; the file will be processed after a restart",
		"Файлът не можа да се обработи":                                         "The file could not be processed",
		"Файлът не е валидно изображение: %v":                                   "The file is not a valid image: %v",
		"Снимката не можа да се смали; остава оригиналът":                       "The image could not be scaled down; the original stays",
		"Смалена от %d×%d (%s), за да се побере в екран %d×%d.":                 "Scaled down from %d×%d (%s) to fit a %d×%d screen.",
		"Снимката е смалена":                                                    "Image scaled down",
		"Автоматично конвертиране на видео":                                     "Automatic video conversion",
		"автоматичното конвертиране не успя: %w":                                "automatic conversion failed: %w",
		"Конвертирано автоматично за плейърите (оригинал: %s, %s).":             "Converted automatically for the players (original: %s, %s).",
		"Невалиден избор за обработка; ползват се настройките на хъба":          "Invalid processing choice; the hub settings are used",
		"ffmpeg не е наличен на сървъра: файлът не е проверен за съвместимост.": "ffmpeg is not available on the server: the file was not checked for compatibility.",
		"Файлът не може да се прочете: %v":                                      "The file cannot be read: %v",
		"Миниатюрата не се създаде":                                             "The thumbnail was not created",
		"ffmpeg не е наличен на сървъра":                                        "ffmpeg is not available on the server",
		"изходният файл липсва: %w":                                             "the source file is missing: %w",
		"Конвертиране на видео":                                                 "Video conversion",
		"конвертирането не успя: %w":                                            "the conversion failed: %w",
		"невалидна рецепта: %v":                                                 "invalid recipe: %v",
		"файл #%d липсва или още не е готов":                                    "file #%d is missing or not ready yet",
		"Правене на видео от снимки":                                            "Making a video from images",
		"видеото не можа да се направи: %w":                                     "the video could not be made: %w",
		"повторението на видеото не успя: %w":                                   "repeating the video failed: %w",
		"шрифтът %s не е намерен (сложете го в tools\\fonts)":                   "font %s not found (put it into tools\\fonts)",
	})
}
