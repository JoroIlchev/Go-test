import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  api, errorText, type CampaignDetail, type CampaignSpec, type CampaignVersion, type Media, type TreeNode, type User,
} from "./api";
import { formatDuration, type TreeIndex } from "./model";
import type { Selection } from "./App";
import { StateBadge } from "./CampaignList";
import { SlideshowDialog } from "./SlideshowDialog";

interface Props {
  me: User;
  idx: TreeIndex;
  id: number;
  onSelect: (s: Selection) => void;
}

const DAYS = ["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Нд"];
const kindLabel = { image: "Снимка", video: "Видео", audio: "Аудио" };
const DEFAULT_IMAGE_S = 8;
const EVERY = [2, 3, 4, 5, 6, 8, 10];

export function CampaignEditor({ idx, id, onSelect }: Props) {
  const [c, setC] = useState<CampaignDetail | null>(null);
  const [name, setName] = useState("");
  const [spec, setSpec] = useState<CampaignSpec | null>(null);
  const [dirty, setDirty] = useState(false);
  const [library, setLibrary] = useState<Media[]>([]);
  const [error, setError] = useState("");
  const [msg, setMsg] = useState("");
  const [preview, setPreview] = useState(false);
  const [slideshow, setSlideshow] = useState(false);
  const [busy, setBusy] = useState(false);
  const [sequence, setSequence] = useState<number[]>([]);

  const apply = useCallback((d: CampaignDetail) => {
    setC(d);
    setName(d.name);
    setSpec({ ...d.draft, days: d.draft.days ?? [] });
    setDirty(false);
  }, []);

  useEffect(() => {
    api<CampaignDetail>("GET", `/api/campaigns/${id}`).then(apply).catch((e) => setError(errorText(e)));
  }, [id, apply]);

  const loadLibrary = useCallback((nodeId: number) =>
    api<{ items: Media[] }>("GET", `/api/media?node_id=${nodeId}`).then((r) => setLibrary(r.items)).catch(() => {}), []);

  useEffect(() => {
    if (c) loadLibrary(c.node_id);
  }, [c?.node_id, loadLibrary]); // eslint-disable-line react-hooks/exhaustive-deps

  // Poll while a video made from the playlist is still rendering.
  const processing = library.some((m) => m.status === "processing");
  useEffect(() => {
    if (!processing || !c) return;
    const t = setInterval(() => loadLibrary(c.node_id), 3000);
    return () => clearInterval(t);
  }, [processing, c?.node_id, loadLibrary]); // eslint-disable-line react-hooks/exhaustive-deps

  // The order the player will use (one cycle), from the hub's own rules.
  const seqKey = spec ? JSON.stringify([spec.items, spec.order]) : "";
  useEffect(() => {
    if (!spec || spec.items.length === 0) {
      setSequence([]);
      return;
    }
    const t = setTimeout(() => {
      api<{ sequence: number[] }>("POST", "/api/campaigns/sequence", { spec })
        .then((r) => setSequence(r.sequence)).catch(() => setSequence(spec.items.map((_, i) => i)));
    }, 250);
    return () => clearTimeout(t);
  }, [seqKey]); // eslint-disable-line react-hooks/exhaustive-deps

  const owner = c ? idx.byId.get(c.node_id) : undefined;
  const byId = useMemo(() => new Map(library.map((m) => [m.id, m])), [library]);

  // Media uploaded below the campaign's node is not available on all its targets.
  const usable = useMemo(() => library.filter((m) => {
    if (!c) return false;
    const n = idx.byId.get(m.node_id);
    if (!n || m.node_id === c.node_id) return true;
    return !idx.chain(n).some((x) => x.id === c.node_id);
  }), [library, c, idx]);

  if (error && !c) return <section><div className="error">{error}</div></section>;
  if (!c || !spec) return null;

  const editable = c.can_edit;
  const set = (patch: Partial<CampaignSpec>) => { setSpec({ ...spec, ...patch }); setDirty(true); setMsg(""); };

  async function run(fn: () => Promise<CampaignDetail | void>, ok?: string) {
    setBusy(true);
    setError("");
    setMsg("");
    try {
      const d = await fn();
      if (d) apply(d);
      if (ok) setMsg(ok);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }

  const save = () => api<CampaignDetail>("PUT", `/api/campaigns/${id}/draft`, { name, spec });
  const publish = () => run(async () => {
    if (dirty) await save();
    return api<CampaignDetail>("POST", `/api/campaigns/${id}/publish`);
  }, c.needs_review ? "Изпратено за одобрение." : "Публикувано. Устройствата ще го получат при следващата си проверка.");

  const itemMs = (it: CampaignSpec["items"][number]) => {
    const m = byId.get(it.media_id);
    if (!m) return 0;
    return m.kind === "image" ? (it.duration_s || DEFAULT_IMAGE_S) * 1000 : it.duration_s ? it.duration_s * 1000 : m.duration_ms;
  };
  const seq = sequence.filter((i) => i < spec.items.length);
  const totalMs = seq.reduce((s, i) => s + itemMs(spec.items[i]), 0);

  const pending = c.versions.find((v) => v.status === "pending");

  return (
    <section>
      {owner && (
        <div className="crumbs">
          <button className="link" onClick={() => onSelect({ type: "node", id: owner.id })}>{idx.label(owner)}</button> › Кампании
        </div>
      )}
      <div className="title-row">
        <input className="title-input" value={name} disabled={!editable}
          onChange={(e) => { setName(e.target.value); setDirty(true); }} />
        <StateBadge state={c.state} />
        <span className="spacer" />
        <button className="ghost" onClick={() => setPreview(true)} disabled={spec.items.length === 0}>▶ Преглед</button>
        {editable && <button className="ghost" disabled={busy || !dirty} onClick={() => run(save, "Черновата е запазена.")}>Запази чернова</button>}
        {c.can_publish && (
          <button className="primary" disabled={busy || (!dirty && !c.draft_dirty)} onClick={publish}>
            {c.needs_review ? "Изпрати за одобрение" : "Публикувай"}
          </button>
        )}
      </div>
      {error && <div className="error">{error}</div>}
      {msg && <div className="ok">{msg}</div>}
      {dirty && <div className="notice small">Има незапазени промени.</div>}
      {!dirty && c.draft_dirty && c.live && <div className="notice small">Черновата се различава от излъчваната v{c.live.number}. Публикувайте, за да влезе в сила.</div>}

      {c.live && (
        <div className="card live-card">
          <div>
            <strong>На живо: v{c.live.number}</strong>
            <span className="muted"> · публикувана {new Date(c.live.created_at).toLocaleString("bg-BG")} от {c.live.created_by_name}</span>
          </div>
          <div className="ready">
            <span className="bar"><span style={{ width: c.devices ? `${(100 * c.ready) / c.devices}%` : "0%" }} /></span>
            <span>Получили я: <strong>{c.ready}/{c.devices}</strong> устройства</span>
          </div>
          {c.late > 0 && c.latest_check && (
            <div className="notice small late">
              ⚠ {c.late === 1 ? "1 устройство ще провери" : `${c.late} устройства ще проверят`} чак след началото на кампанията
              (последното в {new Date(c.latest_check).toLocaleString("bg-BG")}). Дотогава те ще показват старото съдържание.
              Публикувайте по-рано или сменете графика на проверките в звеното.
            </div>
          )}
          {c.can_publish && <button className="ghost danger" disabled={busy}
            onClick={() => confirm("Кампанията спира да се излъчва при следващата проверка на устройствата. Продължаваме?") &&
              run(() => api<CampaignDetail>("POST", `/api/campaigns/${id}/live`, { version_id: 0 }), "Кампанията е спряна.")}>Спри</button>}
        </div>
      )}

      {pending && c.can_admin && (
        <ReviewBox v={pending} busy={busy} onReview={(approve, comment) =>
          run(() => api<CampaignDetail>("POST", `/api/campaigns/${id}/versions/${pending.id}/review`, { approve, comment }),
            approve ? "Одобрено и публикувано." : "Отхвърлено.")} />
      )}

      <div className="editor-grid">
        <div className="card">
          <div className="title-row">
            <h3>Плейлист</h3>
            <span className="spacer" />
            <span className="muted small">{spec.items.length} елемента · цикъл {seq.length > spec.items.length && `${seq.length} показвания, `}{formatDuration(totalMs)}</span>
            {editable && spec.items.some((it) => byId.get(it.media_id)?.kind === "image") && (
              <button className="ghost" onClick={() => setSlideshow(true)} title="Слива снимките в едно видео с плавни преходи">🎬 Направи видео</button>
            )}
          </div>
          <Playlist spec={spec} byId={byId} editable={editable} onChange={(items) => set({ items })} />
          <SequenceStrip spec={spec} seq={seq} byId={byId} />
          <div className="grid2" style={{ marginTop: 12 }}>
            <label>Ред на излъчване
              <select value={spec.order ?? ""} disabled={!editable} onChange={(e) => set({ order: e.target.value as CampaignSpec["order"] })}>
                <option value="">По реда в списъка</option>
                <option value="shuffle">Разбъркан (всеки ден различен, на всяко устройство различен)</option>
              </select>
            </label>
            <label>Фонова музика
              <select value={spec.audio_id ?? 0} disabled={!editable} onChange={(e) => set({ audio_id: Number(e.target.value) || undefined })}>
                <option value={0}>Без музика</option>
                {usable.filter((m) => m.kind === "audio" && m.status === "ready").map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}
              </select>
            </label>
            <label>Изображението на екрана
              <select value={spec.fit} disabled={!editable} onChange={(e) => set({ fit: e.target.value as CampaignSpec["fit"] })}>
                <option value="meet">Цялото, с черни ленти при нужда</option>
                <option value="slice">Запълва екрана, изрязва краищата</option>
                <option value="fill">Разтяга се до размера на екрана</option>
              </select>
            </label>
          </div>
        </div>

        {editable && (
          <div className="card picker">
            <h3>Библиотека</h3>
            <p className="muted small">Натиснете файл, за да го добавите в края. Нови файлове се качват от раздел „Медия“.</p>
            <div className="picker-grid">
              {usable.filter((m) => m.kind !== "audio").map((m) => (
                <button key={m.id} className="pick" disabled={m.status !== "ready"} title={m.name}
                  onClick={() => set({ items: [...spec.items, { media_id: m.id, duration_s: m.kind === "image" ? DEFAULT_IMAGE_S : 0 }] })}>
                  {m.has_thumb ? <img src={`/api/media/${m.id}/thumb`} alt="" loading="lazy" /> : <span className="thumb-icon">▶</span>}
                  <span className="pick-name">{m.name}</span>
                  {m.warnings.some((w) => w.level === "error") && <span className="pick-warn" title="Може да не тръгне на Android">!</span>}
                </button>
              ))}
              {usable.length === 0 && <p className="muted small">Библиотеката е празна.</p>}
            </div>
          </div>
        )}
      </div>

      <Schedule spec={spec} owner={owner} idx={idx} editable={editable} onChange={set} />

      <Versions c={c} busy={busy} onLive={(v) => run(() => api<CampaignDetail>("POST", `/api/campaigns/${id}/live`, { version_id: v.id }), `v${v.number} е на живо.`)}
        onLoad={(v) => { setSpec({ ...v.spec, days: v.spec.days ?? [] }); setDirty(true); setMsg(`Черновата е заредена от v${v.number}. Запазете или публикувайте.`); }} />

      {editable && !c.live && (
        <div><button className="ghost danger" onClick={() => confirm(`Да изтрия ли кампания „${c.name}“ с всичките ѝ версии?`) &&
          run(async () => { await api("DELETE", `/api/campaigns/${id}`); onSelect({ type: "node", id: c.node_id }); })}>Изтрий кампанията</button></div>
      )}

      {slideshow && (
        <SlideshowDialog nodeId={c.node_id} library={usable} initialName={`${name} (видео)`}
          initial={{
            items: spec.items.filter((it) => byId.get(it.media_id)?.kind === "image")
              .map((it) => ({ media_id: it.media_id, duration_s: it.duration_s || DEFAULT_IMAGE_S })),
            audio_id: spec.audio_id,
            background: spec.fit === "slice" ? "crop" : "blur",
          }}
          replaceLabel="замени снимките в плейлиста с видеото"
          onClose={() => setSlideshow(false)}
          onCreated={(m, sh, replace) => {
            setSlideshow(false);
            setLibrary((l) => [m, ...l]);
            if (!replace) {
              setMsg("Видеото се прави и ще се появи в библиотеката до минута.");
              return;
            }
            // The video takes the place of the first image; the others go.
            // Music baked into the video must not also play over it.
            const used = new Set(sh.items.map((it) => it.media_id));
            const at = spec.items.findIndex((it) => used.has(it.media_id));
            const items = spec.items.filter((it, i) => i === at || !used.has(it.media_id))
              .map((it) => (used.has(it.media_id) ? { media_id: m.id, duration_s: 0 } : it));
            set({ items, audio_id: sh.audio_id && sh.audio_id === spec.audio_id ? undefined : spec.audio_id });
            setMsg("Видеото се прави (обикновено под минута) и вече е в плейлиста. Запазете и публикувайте, когато стане готово.");
          }} />
      )}

      {preview && <PreviewPlayer spec={spec} seq={seq} byId={byId} onClose={() => setPreview(false)} />}
    </section>
  );
}

function Playlist({ spec, byId, editable, onChange }: {
  spec: CampaignSpec; byId: Map<number, Media>; editable: boolean; onChange: (items: CampaignSpec["items"]) => void;
}) {
  const dragFrom = useRef<number | null>(null);
  const [over, setOver] = useState<number | null>(null);
  const items = spec.items;

  function move(from: number, to: number) {
    if (from === to) return;
    const next = [...items];
    const [it] = next.splice(from, 1);
    next.splice(to, 0, it);
    onChange(next);
  }

  if (items.length === 0) return <div className="empty-playlist">Добавете снимки или видеа от библиотеката вдясно.</div>;
  return (
    <ol className="playlist">
      {items.map((it, i) => {
        const m = byId.get(it.media_id);
        return (
          <li key={i} className={over === i ? "over" : ""} draggable={editable}
            onDragStart={() => { dragFrom.current = i; }}
            onDragOver={(e) => { e.preventDefault(); setOver(i); }}
            onDragLeave={() => setOver(null)}
            onDrop={(e) => { e.preventDefault(); setOver(null); if (dragFrom.current !== null) move(dragFrom.current, i); dragFrom.current = null; }}>
            {editable && <span className="grip" title="Влачете за пренареждане">⋮⋮</span>}
            <span className="pl-num">{i + 1}</span>
            <span className="pl-thumb">{m?.has_thumb ? <img src={`/api/media/${m.id}/thumb`} alt="" /> : "▶"}</span>
            <span className="pl-name">
              {m?.name ?? `#${it.media_id}`}
              <span className="muted small"> {m ? kindLabel[m.kind] : ""}{m?.kind === "video" && m.duration_ms > 0 && ` · ${formatDuration(m.duration_ms)}`}</span>
            </span>
            {m?.kind === "image" ? (
              <label className="dur">
                <input type="number" min={1} max={3600} step={1} value={it.duration_s || DEFAULT_IMAGE_S} disabled={!editable}
                  onChange={(e) => onChange(items.map((x, j) => (j === i ? { ...x, duration_s: Number(e.target.value) } : x)))} /> сек
              </label>
            ) : <span className="dur muted small">до края</span>}
            <select className="every" value={it.every ?? 0} disabled={!editable} title="Колко често се показва"
              onChange={(e) => onChange(items.map((x, j) => (j === i ? { ...x, every: Number(e.target.value) || undefined } : x)))}>
              <option value={0}>веднъж в цикъла</option>
              {EVERY.map((n) => <option key={n} value={n}>всеки {n}-{n === 2 ? "ри" : n <= 4 ? "ти" : "и"}</option>)}
            </select>
            {editable && (
              <span className="pl-actions">
                <button className="ghost icon" disabled={i === 0} onClick={() => move(i, i - 1)} aria-label="Нагоре">↑</button>
                <button className="ghost icon" disabled={i === items.length - 1} onClick={() => move(i, i + 1)} aria-label="Надолу">↓</button>
                <button className="ghost icon danger" onClick={() => onChange(items.filter((_, j) => j !== i))} aria-label="Махни">✕</button>
              </span>
            )}
          </li>
        );
      })}
    </ol>
  );
}

/** One cycle of the playlist the way the player shows it. */
function SequenceStrip({ spec, seq, byId }: { spec: CampaignSpec; seq: number[]; byId: Map<number, Media> }) {
  const special = spec.order === "shuffle" || spec.items.some((it) => (it.every ?? 0) >= 2);
  if (!special || seq.length === 0) return null;
  const shown = seq.slice(0, 60);
  return (
    <div className="seq-strip">
      <div className="muted small">
        Ред на излъчване (един цикъл{spec.order === "shuffle" ? ", пример — всяко устройство има свой ред всеки ден" : ""}):
      </div>
      <div className="seq-row">
        {shown.map((i, k) => {
          const it = spec.items[i];
          const m = it && byId.get(it.media_id);
          return (
            <span key={k} className={"seq-cell" + ((it?.every ?? 0) >= 2 ? " focus" : "")} title={m?.name}>
              {m?.has_thumb ? <img src={`/api/media/${m.id}/thumb`} alt="" /> : <span>{i + 1}</span>}
              <small>{i + 1}</small>
            </span>
          );
        })}
        {seq.length > shown.length && <span className="muted small">… още {seq.length - shown.length}</span>}
      </div>
    </div>
  );
}

function Schedule({ spec, owner, idx, editable, onChange }: {
  spec: CampaignSpec; owner?: TreeNode; idx: TreeIndex; editable: boolean; onChange: (p: Partial<CampaignSpec>) => void;
}) {
  const nodes = owner ? subtree(idx, owner) : [];
  const days = spec.days ?? [];
  const windowOn = spec.time_from !== "" || spec.time_to !== "";
  const toggleTarget = (id: number) =>
    onChange({ targets: spec.targets.includes(id) ? spec.targets.filter((t) => t !== id) : [...spec.targets, id] });

  return (
    <div className="card">
      <h3>Къде и кога</h3>
      <div className="schedule-grid">
        <fieldset>
          <legend>Излъчва се в</legend>
          {nodes.length === 0 && <p className="muted small">{spec.targets.length} звена</p>}
          {nodes.map(({ node, depth }) => (
            <label key={node.id} className="check" style={{ paddingLeft: depth * 18 }}>
              <input type="checkbox" disabled={!editable} checked={spec.targets.includes(node.id)} onChange={() => toggleTarget(node.id)} />
              {node.name} <span className="muted small">{idx.devicesUnder(node).length} устр.</span>
            </label>
          ))}
          <p className="muted small">Отметнато звено включва и всичко под него, също и бъдещи устройства.</p>
        </fieldset>

        <fieldset>
          <legend>Период</legend>
          <div className="grid2">
            <label>От
              <input type="datetime-local" value={spec.start} disabled={!editable} onChange={(e) => onChange({ start: e.target.value })} />
            </label>
            <label>До
              <input type="datetime-local" value={spec.end} disabled={!editable} onChange={(e) => onChange({ end: e.target.value })} />
            </label>
          </div>
          <p className="muted small">Празно „От“ = веднага. Празно „До“ = без край. Часовете са по българско време, а устройствата превключват сами точно в този момент.</p>

          <div className="days">
            {DAYS.map((d, i) => (
              <button key={d} type="button" disabled={!editable} className={days.includes(i + 1) ? "on" : ""}
                onClick={() => onChange({ days: days.includes(i + 1) ? days.filter((x) => x !== i + 1) : [...days, i + 1].sort() })}>{d}</button>
            ))}
            <span className="muted small">{days.length === 0 ? "всеки ден" : ""}</span>
          </div>

          <label className="check">
            <input type="checkbox" disabled={!editable} checked={windowOn}
              onChange={(e) => onChange(e.target.checked ? { time_from: "11:00", time_to: "14:00" } : { time_from: "", time_to: "" })} />
            Само в определени часове
          </label>
          {windowOn && (
            <div className="grid2">
              <label>От час<input type="time" value={spec.time_from} disabled={!editable} onChange={(e) => onChange({ time_from: e.target.value })} /></label>
              <label>До час<input type="time" value={spec.time_to} disabled={!editable} onChange={(e) => onChange({ time_to: e.target.value })} /></label>
            </div>
          )}
        </fieldset>

        <fieldset>
          <legend>Приоритет</legend>
          <label className="radio"><input type="radio" disabled={!editable} checked={spec.priority === 0} onChange={() => onChange({ priority: 0 })} />
            Нормален: редува се с другите нормални кампании</label>
          <label className="radio"><input type="radio" disabled={!editable} checked={spec.priority === 1} onChange={() => onChange({ priority: 1 })} />
            Висок: докато е активна, заменя нормалните (промоции, Черен петък)</label>
        </fieldset>
      </div>
    </div>
  );
}

function subtree(idx: TreeIndex, root: TreeNode, depth = 0): { node: TreeNode; depth: number }[] {
  const out = [{ node: root, depth }];
  for (const ch of idx.children.get(root.id) ?? []) out.push(...subtree(idx, ch, depth + 1));
  return out;
}

const versionStatus = { pending: "чака одобрение", approved: "одобрена", rejected: "отхвърлена" };

function Versions({ c, busy, onLive, onLoad }: {
  c: CampaignDetail; busy: boolean; onLive: (v: CampaignVersion) => void; onLoad: (v: CampaignVersion) => void;
}) {
  if (c.versions.length === 0) return null;
  return (
    <div className="card">
      <h3>Версии</h3>
      <table className="table">
        <thead><tr><th>Версия</th><th>Статус</th><th>Публикувал</th><th>Прегледал</th><th /></tr></thead>
        <tbody>
          {c.versions.map((v) => {
            const live = c.live?.id === v.id;
            return (
              <tr key={v.id}>
                <td><strong>v{v.number}</strong>{live && <span className="tag st-active" style={{ marginLeft: 6 }}>на живо</span>}</td>
                <td>{versionStatus[v.status]}{v.review_comment && <div className="muted small">„{v.review_comment}“</div>}</td>
                <td className="small">{v.created_by_name}<div className="muted">{new Date(v.created_at).toLocaleString("bg-BG")}</div></td>
                <td className="small">{v.reviewed_by_name || "—"}</td>
                <td className="row-actions">
                  {c.can_publish && v.status === "approved" && !live && <button className="link small" disabled={busy} onClick={() => onLive(v)}>Пусни тази</button>}
                  {c.can_edit && <button className="link small" onClick={() => onLoad(v)}>Зареди в черновата</button>}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function ReviewBox({ v, busy, onReview }: { v: CampaignVersion; busy: boolean; onReview: (approve: boolean, comment: string) => void }) {
  const [comment, setComment] = useState("");
  return (
    <div className="card review">
      <strong>v{v.number} чака вашето одобрение</strong>
      <span className="muted small"> · изпратена от {v.created_by_name}, {new Date(v.created_at).toLocaleString("bg-BG")}</span>
      <p className="muted small">Прегледайте плейлиста и графика по-долу. Те показват черновата, която може да е по-нова от v{v.number}; самата версия можете да заредите от таблицата „Версии“.</p>
      <div className="inline-form">
        <input value={comment} onChange={(e) => setComment(e.target.value)} placeholder="Коментар (по желание)" style={{ flex: 1 }} />
        <button className="primary" disabled={busy} onClick={() => onReview(true, comment)}>Одобри и публикувай</button>
        <button className="ghost danger" disabled={busy} onClick={() => onReview(false, comment)}>Отхвърли</button>
      </div>
    </div>
  );
}

/** Plays the playlist in the browser roughly the way the player will. */
function PreviewPlayer({ spec, seq, byId, onClose }: { spec: CampaignSpec; seq: number[]; byId: Map<number, Media>; onClose: () => void }) {
  const items = (seq.length ? seq : spec.items.map((_, i) => i)).map((i) => spec.items[i])
    .filter((it) => byId.get(it.media_id)?.status === "ready");
  const [i, setI] = useState(0);
  const next = useCallback(() => setI((x) => (items.length ? (x + 1) % items.length : 0)), [items.length]);
  const it = items[i];
  const m = it ? byId.get(it.media_id) : undefined;
  const audio = spec.audio_id ? byId.get(spec.audio_id) : undefined;

  useEffect(() => {
    if (!m || m.kind !== "image") return;
    const t = setTimeout(next, (it.duration_s || DEFAULT_IMAGE_S) * 1000);
    return () => clearTimeout(t);
  }, [i, m, it, next]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const fit = spec.fit === "fill" ? "fill" : spec.fit === "slice" ? "cover" : "contain";
  return (
    <div className="modal-back" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="title-row">
          <h3>Преглед</h3>
          <span className="muted small">{items.length ? `${i + 1} / ${items.length}` : ""}</span>
          <span className="spacer" />
          <button className="ghost" onClick={next} disabled={items.length < 2}>Следващ ›</button>
          <button className="ghost" onClick={onClose}>Затвори</button>
        </div>
        <div className="screen16x9">
          {m?.kind === "image" && <img key={i} src={`/api/media/${m.id}/file`} alt="" style={{ objectFit: fit }} />}
          {m?.kind === "video" && <video key={i} src={`/api/media/${m.id}/file`} autoPlay muted={!!audio} onEnded={next}
            style={{ objectFit: fit }} />}
          {!m && <span className="muted">Няма готови елементи.</span>}
        </div>
        {audio && <audio src={`/api/media/${audio.id}/file`} autoPlay loop controls />}
        <p className="muted small">Прегледът е в браузъра. На устройството преходите и времената може леко да се различават.</p>
      </div>
    </div>
  );
}
