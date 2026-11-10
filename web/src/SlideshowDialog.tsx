import { useEffect, useState } from "react";
import { api, errorText, type Media, type Slide, type Slideshow, type TextStyle } from "./api";
import { formatDuration } from "./model";

interface Props {
  nodeId: number;
  /** Media usable at the node: owned by it or by something above it. */
  library: Media[];
  initial?: Partial<Slideshow>;
  initialName?: string;
  /** When set, offers to put the finished video in place of the images. */
  replaceLabel?: string;
  onClose: () => void;
  onCreated: (m: Media, sh: Slideshow, replace: boolean) => void;
}

const TRANSITIONS: [string, string][] = [
  ["fade", "Плавно преливане"],
  ["fadeblack", "През черно"],
  ["dissolve", "Разпадане"],
  ["smoothleft", "Меко плъзгане"],
  ["slideleft", "Плъзгане"],
  ["wipeleft", "Завеса"],
  ["circleopen", "Кръг"],
  ["zoomin", "Приближаване"],
  ["none", "Без преход (рязко)"],
];
const DEFAULT_S = 5;

const FONTS: [TextStyle["font"], string, string][] = [
  ["arial", "Arial", "Arial, sans-serif"],
  ["segoe", "Segoe UI", "'Segoe UI', sans-serif"],
  ["impact", "Impact (реклами)", "Impact, sans-serif"],
  ["verdana", "Verdana", "Verdana, sans-serif"],
  ["georgia", "Georgia", "Georgia, serif"],
  ["comic", "Comic Sans", "'Comic Sans MS', cursive"],
];
const SIZES: [TextStyle["size"], string, number][] = [["s", "Малък", 0.045], ["m", "Среден", 0.065], ["l", "Голям", 0.09], ["xl", "Огромен", 0.12]];
const COLORS = ["#ffffff", "#ffcc00", "#ff3b30", "#34c759", "#000000"];
const POSITIONS: [NonNullable<Slide["text_pos"]>, string][] = [["top", "Горе"], ["center", "В средата"], ["bottom", "Долу"]];

export function SlideshowDialog({ nodeId, library, initial, initialName, replaceLabel, onClose, onCreated }: Props) {
  const images = library.filter((m) => m.kind === "image" && m.status === "ready");
  const audios = library.filter((m) => m.kind === "audio" && m.status === "ready");
  const byId = new Map(library.map((m) => [m.id, m]));
  const [name, setName] = useState(initialName ?? "Слайдшоу");
  const [sh, setSh] = useState<Slideshow>({
    items: [], transition: "fade", transition_s: 1, motion: true, background: "blur", portrait: false,
    text_style: { font: "arial", size: "m", color: "#ffffff", box: "dark" }, ...initial,
  });
  const [sel, setSel] = useState(0);
  const [replace, setReplace] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const set = (p: Partial<Slideshow>) => setSh({ ...sh, ...p });
  const setStyle = (p: Partial<TextStyle>) => set({ text_style: { ...sh.text_style, ...p } });
  const items = sh.items;
  const setItem = (i: number, p: Partial<Slide>) => set({ items: items.map((x, j) => (j === i ? { ...x, ...p } : x)) });

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  function move(from: number, to: number) {
    const next = [...items];
    const [it] = next.splice(from, 1);
    next.splice(to, 0, it);
    set({ items: next });
  }

  async function submit() {
    setBusy(true);
    setError("");
    try {
      const body = { ...sh, transition_s: sh.transition === "none" ? 0 : sh.transition_s };
      const m = await api<Media>("POST", "/api/media/render", { node_id: nodeId, name, slideshow: body });
      onCreated(m, body, replace);
    } catch (e) {
      setError(errorText(e));
      setBusy(false);
    }
  }

  const total = items.reduce((s, it) => s + it.duration_s, 0);
  const tooShort = sh.transition !== "none" && items.length > 1 && items.some((it) => it.duration_s <= sh.transition_s);
  const cur = Math.min(sel, items.length - 1);
  const shown = cur >= 0 ? items[cur] : undefined;
  const first = shown ? byId.get(shown.media_id) : undefined;
  const st = sh.text_style;
  const hasText = items.some((it) => it.text?.trim());

  return (
    <div className="modal-back" onClick={onClose}>
      <div className="modal wide" onClick={(e) => e.stopPropagation()}>
        <div className="title-row">
          <h3>🎬 Направи видео от снимки</h3>
          <span className="spacer" />
          <button className="ghost" onClick={onClose}>Затвори</button>
        </div>

        <div className="ss-grid">
          <div>
            <label>Име на видеото<input value={name} onChange={(e) => setName(e.target.value)} /></label>
            <h4>Снимки по ред</h4>
            {items.length === 0 ? <div className="empty-playlist">Натиснете снимки отдолу, за да ги добавите.</div> : (
              <ol className="playlist">
                {items.map((it, i) => {
                  const m = byId.get(it.media_id);
                  return (
                    <li key={i} className={"slide-row" + (i === cur ? " current" : "")} onClick={() => setSel(i)}>
                      <span className="pl-num">{i + 1}</span>
                      <span className="pl-thumb">{m?.has_thumb && <img src={`/api/media/${m.id}/thumb`} alt="" />}</span>
                      <span className="pl-name">{m?.name ?? `#${it.media_id}`}</span>
                      <label className="dur">
                        <input type="number" min={1} max={120} value={it.duration_s}
                          onChange={(e) => setItem(i, { duration_s: Number(e.target.value) })} /> сек
                      </label>
                      <span className="pl-actions">
                        <button className="ghost icon" disabled={i === 0} onClick={() => move(i, i - 1)} aria-label="Нагоре">↑</button>
                        <button className="ghost icon" disabled={i === items.length - 1} onClick={() => move(i, i + 1)} aria-label="Надолу">↓</button>
                        <button className="ghost icon danger" onClick={(e) => { e.stopPropagation(); set({ items: items.filter((_, j) => j !== i) }); }} aria-label="Махни">✕</button>
                      </span>
                      <div className="slide-text">
                        <textarea rows={it.text?.includes("\n") ? 2 : 1} maxLength={300} value={it.text ?? ""} placeholder="Текст върху снимката (по желание)"
                          onFocus={() => setSel(i)} onChange={(e) => setItem(i, { text: e.target.value })} />
                        {it.text?.trim() && (
                          <select value={it.text_pos ?? "bottom"} onChange={(e) => setItem(i, { text_pos: e.target.value as Slide["text_pos"] })}>
                            {POSITIONS.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
                          </select>
                        )}
                      </div>
                    </li>
                  );
                })}
              </ol>
            )}
            <div className="picker-grid small-picks">
              {images.map((m) => (
                <button key={m.id} className="pick" title={m.name}
                  onClick={() => { set({ items: [...items, { media_id: m.id, duration_s: DEFAULT_S }] }); setSel(items.length); }}>
                  {m.has_thumb ? <img src={`/api/media/${m.id}/thumb`} alt="" loading="lazy" /> : <span className="thumb-icon">🖼</span>}
                  <span className="pick-name">{m.name}</span>
                </button>
              ))}
              {images.length === 0 && <p className="muted small">Няма готови снимки. Качете ги от раздел „Медия“.</p>}
            </div>
          </div>

          <div className="ss-options">
            <div className={"ss-preview" + (sh.portrait ? " portrait" : "")}>
              {first && <>
                {sh.background === "blur" && <img className="bg" src={`/api/media/${first.id}/file`} alt="" />}
                <img className={"fg " + sh.background} src={`/api/media/${first.id}/file`} alt="" />
              </>}
              {shown?.text?.trim() && (
                <div className={`ss-caption pos-${shown.text_pos ?? "bottom"} box-${st.box}`} style={{
                  color: st.color,
                  fontFamily: FONTS.find((f) => f[0] === st.font)?.[2] ?? "sans-serif",
                  fontSize: `calc(${SIZES.find((x) => x[0] === st.size)?.[2] ?? 0.065} * 100cqmin)`,
                }}>{shown.text.trim()}</div>
              )}
            </div>
            {items.length > 1 && <span className="muted small">Показана е снимка {cur + 1}. Натиснете друга в списъка, за да я видите.</span>}

            {hasText && (
              <fieldset>
                <legend>Текст</legend>
                <div className="grid2">
                  <label>Шрифт
                    <select value={st.font} onChange={(e) => setStyle({ font: e.target.value as TextStyle["font"] })}>
                      {FONTS.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
                    </select>
                  </label>
                  <label>Размер
                    <select value={st.size} onChange={(e) => setStyle({ size: e.target.value as TextStyle["size"] })}>
                      {SIZES.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
                    </select>
                  </label>
                </div>
                <div className="swatches">
                  <span className="small">Цвят</span>
                  {COLORS.map((c) => (
                    <button key={c} type="button" className={"swatch" + (st.color.toLowerCase() === c ? " on" : "")}
                      style={{ background: c }} onClick={() => setStyle({ color: c })} aria-label={c} />
                  ))}
                  <input type="color" value={st.color} onChange={(e) => setStyle({ color: e.target.value })} title="Друг цвят" />
                </div>
                <label className="radio"><input type="radio" checked={st.box === "dark"} onChange={() => setStyle({ box: "dark" })} /> Върху тъмна лента</label>
                <label className="radio"><input type="radio" checked={st.box === "light"} onChange={() => setStyle({ box: "light" })} /> Върху светла лента</label>
                <label className="radio"><input type="radio" checked={st.box === "none"} onChange={() => setStyle({ box: "none" })} /> Без лента, с контур</label>
              </fieldset>
            )}

            <label>Преход между снимките
              <select value={sh.transition} onChange={(e) => set({ transition: e.target.value })}>
                {TRANSITIONS.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
              </select>
            </label>
            {sh.transition !== "none" && (
              <label>Продължителност на прехода
                <select value={sh.transition_s} onChange={(e) => set({ transition_s: Number(e.target.value) })}>
                  {[0.5, 1, 1.5, 2].map((s) => <option key={s} value={s}>{s.toLocaleString("bg-BG")} сек</option>)}
                </select>
              </label>
            )}
            <label className="check">
              <input type="checkbox" checked={sh.motion} onChange={(e) => set({ motion: e.target.checked })} />
              Бавно приближаване и отдалечаване (оживява снимките)
            </label>

            <fieldset>
              <legend>Когато снимката не е с формата на екрана</legend>
              <label className="radio"><input type="radio" checked={sh.background === "blur"} onChange={() => set({ background: "blur" })} />
                Цялата снимка, отстрани размазано копие</label>
              <label className="radio"><input type="radio" checked={sh.background === "black"} onChange={() => set({ background: "black" })} />
                Цялата снимка, черни ленти</label>
              <label className="radio"><input type="radio" checked={sh.background === "crop"} onChange={() => set({ background: "crop" })} />
                Запълва екрана, изрязва краищата</label>
            </fieldset>

            <fieldset>
              <legend>Екран</legend>
              <label className="radio"><input type="radio" checked={!sh.portrait} onChange={() => set({ portrait: false })} />
                Хоризонтален (1920×1080)</label>
              <label className="radio"><input type="radio" checked={sh.portrait} onChange={() => set({ portrait: true })} />
                Вертикален (1080×1920)</label>
            </fieldset>

            <label>Музика
              <select value={sh.audio_id ?? 0} onChange={(e) => set({ audio_id: Number(e.target.value) || undefined })}>
                <option value={0}>Без музика</option>
                {audios.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}
              </select>
            </label>
          </div>
        </div>

        {error && <div className="error">{error}</div>}
        {tooShort && <div className="notice small">Всяка снимка трябва да стои по-дълго от прехода ({sh.transition_s.toLocaleString("bg-BG")} сек).</div>}
        <div className="title-row">
          <span className="muted">Дължина: <strong>{formatDuration(total * 1000)}</strong>{items.length > 0 && ` · ${items.length} снимки`}</span>
          <span className="spacer" />
          {replaceLabel && (
            <label className="check small"><input type="checkbox" checked={replace} onChange={(e) => setReplace(e.target.checked)} /> {replaceLabel}</label>
          )}
          <button className="primary" disabled={busy || items.length === 0 || tooShort} onClick={submit}>
            {busy ? "Изпращане…" : "Направи видео"}
          </button>
        </div>
        <p className="muted small">Видеото се прави на сървъра (обикновено под минута) и се появява в библиотеката като ново видео. Снимките остават непроменени.</p>
      </div>
    </div>
  );
}
