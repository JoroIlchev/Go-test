import { useState, type FormEvent } from "react";
import { api, errorText, type Device, type User } from "./api";
import { ago, deviceState, type TreeIndex, isStaff } from "./model";
import type { Selection } from "./App";
import { Breadcrumb } from "./NodePanel";

interface Props {
  me: User;
  idx: TreeIndex;
  device: Device;
  serverTime: string;
  onChange: () => Promise<void>;
  onSelect: (s: Selection) => void;
}

export function DevicePanel({ me, idx, device: d, serverTime, onChange, onSelect }: Props) {
  const site = d.node_id !== null ? idx.byId.get(d.node_id) : undefined;
  const st = deviceState(d, serverTime);

  return (
    <section>
      {site && <Breadcrumb idx={idx} node={site} onSelect={onSelect} />}
      {site && (
        <div className="crumbs">
          <button className="link" onClick={() => onSelect({ type: "node", id: site.id })}>{site.name}</button> ›
        </div>
      )}
      <div className="title-row">
        <h2>{d.name || "Без име"}</h2>
        <span className={"pill state-" + st}><span className={"dot " + st} /> {ago(d.last_seen_at, serverTime)}</span>
      </div>

      <div className="card">
        <h3>Информация</h3>
        <dl className="facts">
          <dt>Последна връзка</dt><dd>{d.last_seen_at ? new Date(d.last_seen_at).toLocaleString("bg-BG") : "никога"}</dd>
          {d.offline_since && <><dt>Офлайн</dt><dd className="error-text">не провери в {new Date(d.offline_since).toLocaleString("bg-BG")}</dd></>}
          {d.next_check_at && !d.offline_since && <><dt>Следваща проверка</dt><dd>{new Date(d.next_check_at).toLocaleString("bg-BG")}</dd></>}
          <dt>IP</dt><dd>{d.last_ip || "—"}</dd>
          <dt>Плейър</dt><dd className="mono small">{d.user_agent || "—"}</dd>
          <dt>Добавено</dt><dd>{new Date(d.created_at).toLocaleString("bg-BG")}</dd>
        </dl>
      </div>

      {isStaff(me) ? <DeviceSettings d={d} idx={idx} onChange={onChange} /> : (
        <p className="muted">Настройките на устройството се управляват от оператора на системата.</p>
      )}
    </section>
  );
}

function DeviceSettings({ d, idx, onChange }: { d: Device; idx: TreeIndex; onChange: () => Promise<void> }) {
  const [name, setName] = useState(d.name);
  const [siteId, setSiteId] = useState(d.node_id ?? 0);
  const [authMode, setAuthMode] = useState(d.auth_mode);
  const [authUser, setAuthUser] = useState(d.auth_user);
  const [authPassword, setAuthPassword] = useState("");
  const [msg, setMsg] = useState("");
  const [error, setError] = useState("");

  async function patch(body: Record<string, unknown>, ok: string) {
    setError("");
    setMsg("");
    try {
      await api("PATCH", `/api/devices/${d.id}`, body);
      await onChange();
      setMsg(ok);
    } catch (e) {
      setError(errorText(e));
    }
  }

  function save(e: FormEvent) {
    e.preventDefault();
    if (authMode === "basic" && !d.has_password && !authPassword) {
      setError("За режим с парола задайте парола.");
      return;
    }
    const body: Record<string, unknown> = { name, site_id: siteId, auth_mode: authMode, auth_user: authUser };
    if (authPassword) body.auth_password = authPassword;
    patch(body, "Запазено.").then(() => setAuthPassword(""));
  }

  return (
    <>
      <form className="card form" onSubmit={save}>
        <h3>Настройки</h3>
        <div className="grid2">
          <label>Име<input value={name} onChange={(e) => setName(e.target.value)} /></label>
          <label>Обект
            <select value={siteId} onChange={(e) => setSiteId(Number(e.target.value))}>
              {idx.sites().map((s) => <option key={s.id} value={s.id}>{idx.label(s)}</option>)}
            </select>
          </label>
        </div>
        <fieldset>
          <legend>Достъп на плейъра</legend>
          <label className="radio"><input type="radio" checked={authMode === "link"} onChange={() => setAuthMode("link")} />
            Само таен линк</label>
          <label className="radio"><input type="radio" checked={authMode === "basic"} onChange={() => setAuthMode("basic")} />
            Таен линк + потребител/парола</label>
          {authMode === "basic" && (
            <div className="grid2">
              <label>Потребител<input value={authUser} onChange={(e) => setAuthUser(e.target.value)} required /></label>
              <label>{d.has_password ? "Нова парола (по желание)" : "Парола"}
                <input type="password" autoComplete="new-password" value={authPassword} onChange={(e) => setAuthPassword(e.target.value)} minLength={10} />
              </label>
            </div>
          )}
          <p className="muted small">Режимът с парола важи за личния линк. Не всички плейъри го поддържат.</p>
        </fieldset>
        {error && <div className="error">{error}</div>}
        {msg && <div className="ok">{msg}</div>}
        <div className="buttons"><button className="primary">Запази</button></div>
      </form>

      <div className="card">
        <h3>Личен линк</h3>
        <div className="copy-row">
          <code className="mono">{d.url}</code>
          <button className="ghost" onClick={() => navigator.clipboard.writeText(d.url ?? "")}>Копирай</button>
        </div>
        <p className="muted small">
          Плейърите могат да ползват и общия адрес <code>{(d.url ?? "").replace(/\/d\/[^/]+\/index\.smil$/, "/p/index.smil")}</code>:
          тогава се разпознават по UUID. Адресът на хъба се сменя от Настройки.
        </p>
        <div className="buttons">
          <button className="ghost" onClick={() => confirm("Старият линк спира да работи веднага. Продължаваме?") &&
            patch({ rotate_token: true }, "Нов линк е генериран.")}>Генерирай нов линк</button>
          {d.status === "blocked" ? (
            <button className="ghost" onClick={() => patch({ status: "active" }, "Устройството е активирано.")}>Активирай</button>
          ) : (
            <button className="ghost danger" onClick={() => confirm("Блокираното устройство спира да получава съдържание. Продължаваме?") &&
              patch({ status: "blocked" }, "Устройството е блокирано.")}>Блокирай</button>
          )}
        </div>
      </div>
    </>
  );
}
