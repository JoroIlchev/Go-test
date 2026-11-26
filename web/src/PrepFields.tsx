import type { Prep, Quality } from "./api";
import { t } from "./i18n";

/** The processing choice for uploads: in the hub settings (the default) and
 *  in the media library (per client or site, inherited below). */
export function PrepFields({ value: p, onChange }: { value: Prep; onChange: (p: Partial<Prep>) => void }) {
  return (
    <>
      <h4>{t("Снимки")}</h4>
      <div className="grid2">
        <label>{t("Смаляване на големите снимки")}
          <select value={p.image_screen} onChange={(e) => onChange({ image_screen: e.target.value })}>
            <option value="1920x1080">{t("До хоризонтален Full HD екран (1920×1080)")}</option>
            <option value="1080x1920">{t("До вертикален Full HD екран (1080×1920)")}</option>
            <option value="3840x2160">{t("До 4K екран (3840×2160)")}</option>
            <option value="off">{t("Без смаляване (оригиналът)")}</option>
          </select>
        </label>
        <label>{t("Качество (компресия) на смалените снимки")}
          <select value={p.image_quality} onChange={(e) => onChange({ image_quality: e.target.value as Quality })}>
            <option value="high">{t("Високо (по-голям файл)")}</option>
            <option value="normal">{t("Нормално")}</option>
            <option value="small">{t("По-малък файл")}</option>
          </select>
        </label>
      </div>
      <h4>{t("Видео")}</h4>
      <div className="grid2">
        <label>{t("Автоматично конвертиране при качване")}
          <select value={p.video_auto} onChange={(e) => onChange({ video_auto: e.target.value as Prep["video_auto"] })}>
            <option value="problems">{t("Само когато видеото може да не тръгне")}</option>
            <option value="always">{t("Винаги (уеднаквява и смалява файловете)")}</option>
            <option value="off">{t("Никога (само с бутона „Конвертирай“)")}</option>
          </select>
        </label>
        <label>{t("Максимална резолюция")}
          <select value={p.video_max} onChange={(e) => onChange({ video_max: Number(e.target.value) })}>
            <option value={1080}>Full HD (1080p)</option>
            <option value={720}>{t("HD (720p) — за много слаби устройства")}</option>
          </select>
        </label>
        <label>{t("Качество (компресия)")}
          <select value={p.video_quality} onChange={(e) => onChange({ video_quality: e.target.value as Quality })}>
            <option value="high">{t("Високо (по-голям файл)")}</option>
            <option value="normal">{t("Нормално")}</option>
            <option value="small">{t("По-малък файл")}</option>
          </select>
        </label>
      </div>
    </>
  );
}

const screenText: Record<string, string> = {
  "1920x1080": "снимки до 1920×1080", "1080x1920": "снимки до 1080×1920", "3840x2160": "снимки до 4K", off: "снимки без смаляване",
};
const videoText = { problems: "видео при проблем", always: "видео винаги", off: "видео без конвертиране" };

/** One line, e.g. "снимки до 1920×1080 · видео при проблем, 1080p". */
export function prepSummary(p: Prep): string {
  const video = t(videoText[p.video_auto]) + (p.video_auto === "off" ? "" : `, ${p.video_max}p`);
  const screen = screenText[p.image_screen];
  return `${screen ? t(screen) : p.image_screen} · ${video}`;
}
