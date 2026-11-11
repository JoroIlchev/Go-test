import type { Prep, Quality } from "./api";

/** The processing choice for uploads: in the hub settings (the default) and
 *  in the media library (for the next uploads). */
export function PrepFields({ value: p, onChange }: { value: Prep; onChange: (p: Partial<Prep>) => void }) {
  return (
    <>
      <h4>Снимки</h4>
      <div className="grid2">
        <label>Смаляване на големите снимки
          <select value={p.image_screen} onChange={(e) => onChange({ image_screen: e.target.value })}>
            <option value="1920x1080">До хоризонтален Full HD екран (1920×1080)</option>
            <option value="1080x1920">До вертикален Full HD екран (1080×1920)</option>
            <option value="3840x2160">До 4K екран (3840×2160)</option>
            <option value="off">Без смаляване (оригиналът)</option>
          </select>
        </label>
        <label>Качество (компресия) на смалените снимки
          <select value={p.image_quality} onChange={(e) => onChange({ image_quality: e.target.value as Quality })}>
            <option value="high">Високо (по-голям файл)</option>
            <option value="normal">Нормално</option>
            <option value="small">По-малък файл</option>
          </select>
        </label>
      </div>
      <h4>Видео</h4>
      <div className="grid2">
        <label>Автоматично конвертиране при качване
          <select value={p.video_auto} onChange={(e) => onChange({ video_auto: e.target.value as Prep["video_auto"] })}>
            <option value="problems">Само когато видеото може да не тръгне</option>
            <option value="always">Винаги (уеднаквява и смалява файловете)</option>
            <option value="off">Никога (само с бутона „Конвертирай“)</option>
          </select>
        </label>
        <label>Максимална резолюция
          <select value={p.video_max} onChange={(e) => onChange({ video_max: Number(e.target.value) })}>
            <option value={1080}>Full HD (1080p)</option>
            <option value={720}>HD (720p) — за много слаби устройства</option>
          </select>
        </label>
        <label>Качество (компресия)
          <select value={p.video_quality} onChange={(e) => onChange({ video_quality: e.target.value as Quality })}>
            <option value="high">Високо (по-голям файл)</option>
            <option value="normal">Нормално</option>
            <option value="small">По-малък файл</option>
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
  const video = videoText[p.video_auto] + (p.video_auto === "off" ? "" : `, ${p.video_max}p`);
  return `${screenText[p.image_screen] ?? p.image_screen} · ${video}`;
}

export const samePrep = (a: Prep, b: Prep) =>
  a.image_screen === b.image_screen && a.image_quality === b.image_quality && a.video_auto === b.video_auto &&
  a.video_max === b.video_max && a.video_quality === b.video_quality;
