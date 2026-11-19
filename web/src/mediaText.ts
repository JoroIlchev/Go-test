import type { MediaWarning } from "./api";
import { lang } from "./i18n";

// The hub writes media warnings in its own language. Bulgarian ones are
// matched by their wording (one code covers several messages) and shown in
// English when the admin is English; anything unknown stays as written.
const EN: [RegExp, (m: RegExpMatchArray) => string][] = [
  [/^Формат (\S+): за плейърите се препоръчва MP4\.$/, (m) => `${m[1]} format: MP4 is recommended for the players.`],
  [/^H\.264 профил (.+) не се декодира хардуерно на повечето Android устройства\.$/,
    (m) => `H.264 profile ${m[1]} is not hardware-decoded on most Android devices.`],
  [/^Видеото е H\.265 \(HEVC\): много Android 7 устройства не могат да го пуснат\.$/,
    () => "The video is H.265 (HEVC): many Android 7 devices cannot play it."],
  [/^Във файла няма видео поток\.$/, () => "The file has no video stream."],
  [/^Във файла няма аудио поток\.$/, () => "The file has no audio stream."],
  [/^Видео кодек (\S+): за Android 7 се препоръчва H\.264\.$/, (m) => `Video codec ${m[1]}: H.264 is recommended for Android 7.`],
  [/^Цветови формат yuvj420p \(пълен диапазон\): някои устройства показват черен или замръзнал кадър\. Конвертирайте го\.$/,
    () => "Colour format yuvj420p (full range): some devices show a black or frozen frame. Convert it."],
  [/^Цветови формат (\S+): по-сигурен е yuv420p\.$/, (m) => `Colour format ${m[1]}: yuv420p is safer.`],
  [/^Резолюция (\d+×\d+): над Full HD е твърде тежко за Android 7 устройствата\.$/,
    (m) => `Resolution ${m[1]}: above Full HD is too heavy for Android 7 devices.`],
  [/^(\d+) кадъра в секунда: по-слабите устройства може да насичат над 30\.$/,
    (m) => `${m[1]} frames per second: weaker devices may stutter above 30.`],
  [/^Битрейт ([\d.]+) Mbps: висок за евтините устройства \(до 12 Mbps е сигурно\)\.$/,
    (m) => `Bitrate ${m[1]} Mbps: high for cheap devices (up to 12 Mbps is safe).`],
  [/^Аудио кодек (\S+): за Android се препоръчва AAC\.$/, (m) => `Audio codec ${m[1]}: AAC is recommended for Android.`],
  [/^Аудио (\S+): най-сигурни са MP3 и AAC\.$/, (m) => `${m[1]} audio: MP3 and AAC are the safest.`],
  [/^WebP не се поддържа сигурно от всички плейъри; по-сигурни са JPG и PNG\.$/,
    () => "WebP is not reliably supported by all players; JPG and PNG are safer."],
  [/^Изображение (\d+×\d+): много голямо, може да се зарежда бавно\. Достатъчно е 1920×1080\.$/,
    (m) => `Image ${m[1]}: very large, it may load slowly. 1920×1080 is enough.`],
  [/^Смалена от (\d+×\d+) \((.+)\), за да се побере в екран (\d+×\d+)\.$/,
    (m) => `Shrunk from ${m[1]} (${m[2]}) to fit a ${m[3]} screen.`],
  [/^Конвертирано автоматично за плейърите \(оригинал: (\S+), (.+)\)\.$/,
    (m) => `Converted automatically for the players (original: ${m[1]}, ${m[2]}).`],
  [/^ffmpeg не е наличен на сървъра: файлът не е проверен за съвместимост\.$/,
    () => "ffmpeg is not available on the server: the file was not checked for compatibility."],
];

export function warningText(w: MediaWarning): string {
  if (lang() !== "en") return w.message;
  for (const [re, fn] of EN) {
    const m = w.message.match(re);
    if (m) return fn(m);
  }
  return w.message;
}
