import { useState, type FormEvent } from "react";
import { api, errorText, type Device } from "./api";
import { ago, type TreeIndex } from "./model";
import type { Selection } from "./App";

interface Props {
  idx: TreeIndex;
  pending: Device[];
  hubUrl?: string;
  serverTime: string;
  onChange: () => Promise<void>;
  onSelect: (s: Selection) => void;
}

export function PendingPanel({ idx, pending, hubUrl, serverTime, onChange, onSelect }: Props) {
  const sites = idx.sites();
  const [code, setCode] = useState("");
  const [siteId, setSiteId] = useState(sites[0]?.id ?? 0);
  const [name, setName] = useState("");
  const [error, setError] = useState("");

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    try {
      const d = await api<Device>("POST", "/api/devices/pair", { code, site_id: siteId, name });
      setCode("");
      setName("");
      await onChange();
      onSelect({ type: "device", id: d.id });
    } catch (err) {
      setError(errorText(err));
    }
  }

  return (
    <section>
      <h2>Чакащи устройства</h2>
      <form className="card form" onSubmit={submit}>
        <h3>Свързване</h3>
        <p className="muted small">Въведете кода, който устройството показва на екрана.</p>
        {sites.length === 0 ? <p className="error">Първо създайте обект в дървото.</p> : (
          <>
            <div className="grid3">
              <label>Код<input className="mono code-input" value={code} onChange={(e) => setCode(e.target.value.toUpperCase())}
                maxLength={6} placeholder="K7F2QX" required /></label>
              <label>Обект
                <select value={siteId} onChange={(e) => setSiteId(Number(e.target.value))}>
                  {sites.map((s) => <option key={s.id} value={s.id}>{idx.label(s)}</option>)}
                </select>
              </label>
              <label>Име на екрана<input value={name} onChange={(e) => setName(e.target.value)} placeholder="Бар" required /></label>
            </div>
            {error && <div className="error">{error}</div>}
            <div className="buttons"><button className="primary">Свържи</button></div>
          </>
        )}
      </form>

      <div className="card">
        <h3>Обадили се устройства</h3>
        {pending.length === 0 ? <p className="muted">Няма чакащи устройства.</p> : (
          <table className="table">
            <thead><tr><th>Код</th><th>Име от плейъра</th><th>IP</th><th>Първо обаждане</th><th>Последно</th><th /></tr></thead>
            <tbody>
              {pending.map((d) => (
                <tr key={d.id}>
                  <td className="mono strong">{d.pairing_code}</td>
                  <td>{d.name || "—"}</td>
                  <td>{d.last_ip}</td>
                  <td className="muted">{ago(d.created_at, serverTime)}</td>
                  <td className="muted">{ago(d.last_seen_at, serverTime)}</td>
                  <td><button className="link" onClick={() => { setCode(d.pairing_code ?? ""); if (!name) setName(d.name); }}>Избери</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <p className="muted small">
          Нови устройства се появяват тук, когато Marquee Player се свърже с хъба (инсталира се от{" "}
          <a href={(hubUrl ?? "") + "/app"} target="_blank" rel="noreferrer"><code>{(hubUrl ?? "") + "/app"}</code></a>{" "}
          в браузъра на устройството) или друг плейър — с общия адрес <code>{(hubUrl ?? "") + "/p/index.smil"}</code>.
          Сравнете кода в таблицата с кода на екрана, преди да свържете.
        </p>
      </div>
    </section>
  );
}
