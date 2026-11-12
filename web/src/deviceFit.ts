import type { CampaignSpec, Device, Media } from "./api";
import { formatBytes, type TreeIndex } from "./model";

/** Problems the campaign will have on the targeted screens, from what the
 *  Marquee Player reported about them (other players report nothing). */
export function fitWarnings(spec: CampaignSpec, idx: TreeIndex, byId: Map<number, Media>): string[] {
  const devices = idx.devices.filter((d) => {
    if (!d.info || d.status !== "active" || d.node_id === null) return false;
    const site = idx.byId.get(d.node_id);
    return !!site && idx.chain(site).some((n) => spec.targets.includes(n.id));
  });
  if (devices.length === 0) return [];

  const files = new Map<number, Media>();
  for (const id of [...spec.items.map((it) => it.media_id), spec.audio_id ?? 0]) {
    const m = byId.get(id);
    if (m) files.set(id, m);
  }
  const out: string[] = [];
  // One line per file and problem, naming the devices it affects.
  const add = (what: string, ds: Device[], tail = "") => {
    if (ds.length) out.push(`${what}: ${ds.map((d) => d.name || `#${d.id}`).join(", ")}${tail}`);
  };
  for (const m of files.values()) {
    if (m.kind !== "image" && m.kind !== "video") continue;
    const portrait = m.height > m.width;
    const turned = devices.filter((d) => {
      const i = d.info!;
      return i.screen_w > 0 && i.screen_h > 0 && portrait !== i.screen_h > i.screen_w;
    });
    add(`„${m.name}“ е ${portrait ? "вертикален" : "хоризонтален"}, а екранът не е — ще ${spec.fit === "slice" ? "се изреже" : spec.fit === "fill" ? "се разтегне" : "има черни ленти"}`, turned);
    if (m.kind !== "video" || !m.width) continue;
    const big = devices.filter((d) => {
      const i = d.info!;
      if (!i.video_max_w || !i.video_max_h) return false;
      const fits = (m.width <= i.video_max_w && m.height <= i.video_max_h) || (m.width <= i.video_max_h && m.height <= i.video_max_w);
      return !fits;
    });
    add(`„${m.name}“ (${m.width}×${m.height}) е по-голямо, отколкото устройството може да пусне`, big, " — конвертирайте го до 1080p");
    if (m.video_codec === "hevc") add(`„${m.name}“ е H.265, а устройството не го поддържа`, devices.filter((d) => !d.info!.hevc));
  }
  const total = [...files.values()].reduce((s, m) => s + m.size, 0);
  add(`Кампанията е ${formatBytes(total)}, а няма толкова свободно място`,
    devices.filter((d) => d.info!.free_bytes > 0 && d.info!.free_bytes < total));
  return out;
}
