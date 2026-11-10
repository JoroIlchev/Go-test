import { useCallback, useEffect, useRef, useState, type DragEvent } from "react";
import { api, errorText, uploadFile, type Media, type TreeNode, type User } from "./api";
import { can, formatBytes, formatDuration, type TreeIndex } from "./model";
import { SlideshowDialog } from "./SlideshowDialog";

interface Props {
  me: User;
  idx: TreeIndex;
  node: TreeNode;
}

interface Upload {
  key: number;
  name: string;
  size: number;
  progress: number;
  error?: string;
  done?: boolean;
}

const ACCEPT = ".jpg,.jpeg,.png,.gif,.webp,.mp4,.m4v,.mov,.webm,.mkv,.avi,.mp3,.m4a,.aac,.ogg,.wav";
type KindFilter = "all" | "image" | "video" | "audio";
const kindLabel = { image: "Снимка", video: "Видео", audio: "Аудио" };

export function MediaLibrary({ me, idx, node }: Props) {
  const canUpload = can(me, node, idx, "create");
  const [items, setItems] = useState<Media[]>([]);
  const [ffmpeg, setFfmpeg] = useState(true);
  const [uploads, setUploads] = useState<Upload[]>([]);
  const [drag, setDrag] = useState(false);
  const [filter, setFilter] = useState<KindFilter>("all");
  const [onlyHere, setOnlyHere] = useState(false);
  const [query, setQuery] = useState("");
  const [preview, setPreview] = useState<Media | null>(null);
  const [slideshow, setSlideshow] = useState(false);
  const [error, setError] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const keyRef = useRef(0);

  const load = useCallback(async () => {
    try {
      const res = await api<{ items: Media[]; ffmpeg: boolean }>("GET", `/api/media?node_id=${node.id}`);
      setItems(res.items);
      setFfmpeg(res.ffmpeg);
    } catch (e) {
      setError(errorText(e));
    }
  }, [node.id]);

  useEffect(() => { load(); }, [load]);

  // Poll while the server is still probing or converting something.
  const busy = items.some((m) => m.status === "processing");
  useEffect(() => {
    if (!busy) return;
    const t = setInterval(load, 2000);
    return () => clearInterval(t);
  }, [busy, load]);

  async function uploadAll(files: File[]) {
    const queued = files.map((f) => ({ key: ++keyRef.current, name: f.name, size: f.size, progress: 0 }));
    setUploads((u) => [...queued, ...u]);
    const patch = (key: number, p: Partial<Upload>) => setUploads((u) => u.map((x) => (x.key === key ? { ...x, ...p } : x)));
    // One at a time: steady progress and no saturating a shop's upload link.
    for (let i = 0; i < files.length; i++) {
      const { key } = queued[i];
      try {
        const res = await uploadFile(node.id, files[i], (progress) => patch(key, { progress }));
        if (res.error) patch(key, { error: errorText(new Error(res.error)), done: true });
        else patch(key, { progress: 1, done: true });
      } catch (e) {
        patch(key, { error: errorText(e), done: true });
      }
      await load();
    }
    // Drop finished rows after a moment; keep failures visible.
    setTimeout(() => setUploads((u) => u.filter((x) => !x.done || x.error)), 4000);
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    setDrag(false);
    if (canUpload && e.dataTransfer.files.length) uploadAll([...e.dataTransfer.files]);
  }

  async function act(fn: () => Promise<unknown>) {
    setError("");
    try {
      await fn();
      await load();
    } catch (e) {
      setError(errorText(e));
    }
  }

  const q = query.trim().toLowerCase();
  const shown = items.filter((m) =>
    (filter === "all" || m.kind === filter) &&
    (!onlyHere || m.node_id === node.id) &&
    (!q || m.name.toLowerCase().includes(q)));
  const totalSize = items.filter((m) => m.node_id === node.id).reduce((s, m) => s + m.size, 0);

  return (
    <div className="media-lib" onDragOver={(e) => { e.preventDefault(); if (canUpload) setDrag(true); }}
      onDragLeave={(e) => { if (e.currentTarget === e.target) setDrag(false); }} onDrop={onDrop}>
      {canUpload && (
        <div className={"dropzone" + (drag ? " over" : "")} onClick={() => inputRef.current?.click()}
          role="button" tabIndex={0} onKeyDown={(e) => e.key === "Enter" && inputRef.current?.click()}>
          <strong>Пуснете файлове тук</strong> или натиснете за избор
          <span className="muted small">Снимки (JPG, PNG, GIF), видео (MP4, MOV…) и аудио (MP3, AAC). Качват се в „{node.name}“ и се виждат от всичко под него.</span>
          <input ref={inputRef} type="file" multiple accept={ACCEPT} hidden
            onChange={(e) => { if (e.target.files?.length) uploadAll([...e.target.files]); e.target.value = ""; }} />
        </div>
      )}
      {!ffmpeg && <div className="notice">На сървъра няма ffmpeg: видеата се качват, но не се проверяват и нямат миниатюри.</div>}

      {uploads.length > 0 && (
        <ul className="uploads">
          {uploads.map((u) => (
            <li key={u.key}>
              <span className="name">{u.name}</span>
              <span className="muted small">{formatBytes(u.size)}</span>
              {u.error ? <span className="error small">{u.error}</span> : (
                <span className="bar"><span style={{ width: `${Math.round(u.progress * 100)}%` }} /></span>
              )}
              {u.done && !u.error && <span className="ok small">✓</span>}
            </li>
          ))}
        </ul>
      )}

      <div className="toolbar">
        <div className="seg">
          {(["all", "image", "video", "audio"] as KindFilter[]).map((k) => (
            <button key={k} className={filter === k ? "on" : ""} onClick={() => setFilter(k)}>
              {k === "all" ? "Всички" : kindLabel[k]}
            </button>
          ))}
        </div>
        <input className="search" placeholder="Търсене…" value={query} onChange={(e) => setQuery(e.target.value)} />
        <label className="check small"><input type="checkbox" checked={onlyHere} onChange={(e) => setOnlyHere(e.target.checked)} /> само от това звено</label>
        <span className="spacer" />
        {canUpload && ffmpeg && <button className="ghost" onClick={() => setSlideshow(true)}>🎬 Видео от снимки</button>}
        <span className="muted small">{items.length} файла · {formatBytes(totalSize)} тук</span>
      </div>
      {error && <div className="error">{error}</div>}

      {shown.length === 0 ? <p className="muted">{items.length === 0 ? "Още няма качени файлове." : "Нищо не отговаря на филтъра."}</p> : (
        <div className="media-grid">
          {shown.map((m) => (
            <MediaCard key={m.id} m={m} inherited={m.node_id !== node.id} onPreview={() => setPreview(m)}
              onRename={() => {
                const name = prompt("Ново име", m.name);
                if (name && name !== m.name) act(() => api("PATCH", `/api/media/${m.id}`, { name }));
              }}
              onDelete={() => confirm(`Да изтрия ли „${m.name}“?`) && act(() => api("DELETE", `/api/media/${m.id}`))}
              onConvert={() => act(() => api("POST", `/api/media/${m.id}/convert`))} />
          ))}
        </div>
      )}

      {preview && <Preview m={preview} onClose={() => setPreview(null)} />}
      {slideshow && (
        // Only what is owned here or above can go into a video that lives here.
        <SlideshowDialog nodeId={node.id} library={items.filter((m) => idx.chain(node).some((n) => n.id === m.node_id))}
          onClose={() => setSlideshow(false)}
          onCreated={() => { setSlideshow(false); load(); }} />
      )}
    </div>
  );
}

function MediaCard({ m, inherited, onPreview, onRename, onDelete, onConvert }: {
  m: Media; inherited: boolean; onPreview: () => void; onRename: () => void; onDelete: () => void; onConvert: () => void;
}) {
  const [open, setOpen] = useState(false);
  const errors = m.warnings.filter((w) => w.level === "error");
  const meta = [
    m.width > 0 && `${m.width}×${m.height}`,
    m.duration_ms > 0 && formatDuration(m.duration_ms),
    m.size > 0 && formatBytes(m.size),
  ].filter(Boolean).join(" · ");

  return (
    <div className={"media-card" + (m.status === "failed" ? " failed" : "")}>
      <button className="thumb" onClick={onPreview} disabled={m.status !== "ready"} title="Преглед">
        {m.has_thumb ? <img src={`/api/media/${m.id}/thumb`} alt="" loading="lazy" /> : (
          <span className="thumb-icon">{m.kind === "audio" ? "♪" : m.kind === "video" ? "▶" : "▣"}</span>
        )}
        <span className="kind-tag">{kindLabel[m.kind]}</span>
        {m.kind === "video" && m.duration_ms > 0 && <span className="dur-tag">{formatDuration(m.duration_ms)}</span>}
      </button>
      <div className="media-body">
        <div className="media-name" title={m.name}>{m.name}</div>
        <div className="muted small">{meta || " "}</div>
        {inherited && <div className="muted small">от „{m.owner_name}“</div>}
        <div className="status-row">
          {m.status === "processing" && <span className="tag busy">{m.source_id ? "Конвертиране…" : "Проверка…"}</span>}
          {m.status === "failed" && <span className="tag bad" title={m.error}>Грешка</span>}
          {m.status === "ready" && m.warnings.length === 0 && <span className="tag good">✓ Съвместимо</span>}
          {m.status === "ready" && m.warnings.length > 0 && (
            <button className={"tag " + (errors.length ? "bad" : "warn")} onClick={() => setOpen(!open)}>
              {errors.length ? "✕ Няма да тръгне" : "⚠ Внимание"} ({m.warnings.length})
            </button>
          )}
        </div>
        {m.status === "failed" && <div className="error small">{m.error}</div>}
        {open && (
          <ul className="warnings">
            {m.warnings.map((w) => <li key={w.code} className={w.level}>{w.message}</li>)}
          </ul>
        )}
        <div className="card-actions">
          {m.convertible && <button className="primary small-btn" onClick={onConvert}>Конвертирай за Android</button>}
          {m.can_edit && <button className="link small" onClick={onRename}>Преименувай</button>}
          {m.can_edit && <button className="link small danger" onClick={onDelete}>Изтрий</button>}
        </div>
      </div>
    </div>
  );
}

function Preview({ m, onClose }: { m: Media; onClose: () => void }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  const src = `/api/media/${m.id}/file`;
  return (
    <div className="modal-back" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="title-row">
          <h3>{m.name}</h3>
          <span className="spacer" />
          <a className="ghost btn-link" href={src + "?download"}>Изтегли</a>
          <button className="ghost" onClick={onClose}>Затвори</button>
        </div>
        <div className="preview-body">
          {m.kind === "image" && <img src={src} alt={m.name} />}
          {m.kind === "video" && <video src={src} controls autoPlay />}
          {m.kind === "audio" && <audio src={src} controls autoPlay />}
        </div>
        <dl className="facts small">
          {m.video_codec && <><dt>Видео</dt><dd>{m.video_codec.toUpperCase()} {m.width}×{m.height}{m.fps > 0 && `, ${m.fps.toFixed(0)} fps`}</dd></>}
          {m.audio_codec && <><dt>Аудио</dt><dd>{m.audio_codec.toUpperCase()}</dd></>}
          {m.bitrate > 0 && <><dt>Битрейт</dt><dd>{(m.bitrate / 1e6).toFixed(1)} Mbps</dd></>}
          <dt>Размер</dt><dd>{formatBytes(m.size)}</dd>
          <dt>Качено</dt><dd>{new Date(m.created_at).toLocaleString("bg-BG")}</dd>
        </dl>
      </div>
    </div>
  );
}
