import { useState, type FormEvent } from "react";
import { api, errorText, type Device, type DeviceInfo, type User } from "./api";
import { ago, deviceState, formatBytes, type TreeIndex, isStaff } from "./model";
import type { Selection } from "./App";
import { Breadcrumb } from "./NodePanel";
import { locale, t } from "./i18n";

interface Props {
  me: User;
  idx: TreeIndex;
  device: Device;
  serverTime: string;
  playerApp?: string;
  onChange: () => Promise<void>;
  onSelect: (s: Selection) => void;
}

export function DevicePanel({ me, idx, device: d, serverTime, playerApp, onChange, onSelect }: Props) {
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
        <h2>{d.name || t("Без име")}</h2>
        <span className={"pill state-" + st}><span className={"dot " + st} /> {ago(d.last_seen_at, serverTime)}</span>
      </div>

      <div className="card">
        <h3>{t("Информация")}</h3>
        <dl className="facts">
          <dt>{t("Последна връзка")}</dt><dd>{d.last_seen_at ? new Date(d.last_seen_at).toLocaleString(locale()) : t("никога")}</dd>
          {d.offline_since && <><dt>{t("Офлайн")}</dt><dd className="error-text">{t("не провери в {time}", { time: new Date(d.offline_since).toLocaleString(locale()) })}</dd></>}
          {d.next_check_at && !d.offline_since && <><dt>{t("Следваща проверка")}</dt><dd>{new Date(d.next_check_at).toLocaleString(locale())}</dd></>}
          <dt>IP</dt><dd>{d.last_ip || "—"}</dd>
          <dt>{t("Плейър")}</dt><dd className="mono small">{d.user_agent || "—"}</dd>
          <dt>{t("Добавено")}</dt><dd>{new Date(d.created_at).toLocaleString(locale())}</dd>
        </dl>
      </div>

      {d.info && <Hardware info={d.info} playerApp={playerApp} />}

      {isStaff(me) ? <DeviceSettings d={d} idx={idx} onChange={onChange} /> : (
        <p className="muted">{t("Настройките на устройството се управляват от оператора на системата.")}</p>
      )}
    </section>
  );
}

function Hardware({ info: i, playerApp }: { info: DeviceInfo; playerApp?: string }) {
  const behind = !!playerApp && !!i.app && i.app !== playerApp;
  const days = Math.floor(i.uptime_s / 86400);
  const hours = Math.floor((i.uptime_s % 86400) / 3600);
  return (
    <div className="card">
      <h3>{t("Устройство")}</h3>
      <dl className="facts">
        <dt>{t("Модел")}</dt><dd>{[i.manufacturer, i.model].filter(Boolean).join(" ") || "—"}{i.tv && <span className="muted"> · {t("телевизор")}</span>}</dd>
        <dt>Android</dt><dd>{i.android || "—"}</dd>
        <dt>{t("Екран")}</dt><dd>{i.screen_w && i.screen_h ? `${i.screen_w}×${i.screen_h}, ${i.screen_w >= i.screen_h ? t("хоризонтален") : t("вертикален")}` : "—"}</dd>
        <dt>{t("Видео до")}</dt><dd>{i.video_max_w ? `${i.video_max_w}×${i.video_max_h}${i.hevc ? t(", и H.265") : t(", само H.264")}` : "—"}</dd>
        <dt>{t("Свободно място")}</dt><dd>{i.total_bytes ? t("{free} от {total}", { free: formatBytes(i.free_bytes), total: formatBytes(i.total_bytes) }) : "—"}</dd>
        <dt>{t("Локален IP")}</dt><dd>{i.local_ip || "—"}</dd>
        <dt>{t("Работи от")}</dt><dd>{i.uptime_s ? (days ? t("{days} дни {hours} ч", { days, hours }) : t("{hours} ч", { hours })) : "—"}</dd>
        <dt>{t("Плейър")}</dt><dd>Marquee Player {i.app}
          {playerApp && (behind
            ? <> <span className="tag warn">{t("на хъба е {version}, ще се обнови", { version: playerApp })}</span></>
            : <span className="muted"> · {t("актуална")}</span>)}</dd>
        <dt>{t("Обновяване")}</dt><dd>{i.installs === false
          ? <span className="tag warn">{t("Няма разрешение „Инсталиране на неизвестни приложения“: обновленията няма да минат")}</span>
          : i.owner ? t("Автоматично, без потвърждение") : t("Автоматично, с потвърждение на екрана")}</dd>
        {i.autostart !== undefined && <>
          <dt>{t("Автоматичен старт")}</dt><dd>{i.autostart
            ? t("Да")
            : <span className="tag warn">{t("Няма разрешение „Показване върху други приложения“: няма да тръгва сам при включване и след обновяване")}</span>}</dd>
        </>}
      </dl>
    </div>
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
      setError(t("За режим с парола задайте парола."));
      return;
    }
    const body: Record<string, unknown> = { name, site_id: siteId, auth_mode: authMode, auth_user: authUser };
    if (authPassword) body.auth_password = authPassword;
    patch(body, t("Запазено.")).then(() => setAuthPassword(""));
  }

  return (
    <>
      <form className="card form" onSubmit={save}>
        <h3>{t("Настройки")}</h3>
        <div className="grid2">
          <label>{t("Име")}<input value={name} onChange={(e) => setName(e.target.value)} /></label>
          <label>{t("Обект")}
            <select value={siteId} onChange={(e) => setSiteId(Number(e.target.value))}>
              {idx.sites().map((s) => <option key={s.id} value={s.id}>{idx.label(s)}</option>)}
            </select>
          </label>
        </div>
        {!d.info && <fieldset>
          <legend>{t("Достъп на плейъра")}</legend>
          <label className="radio"><input type="radio" checked={authMode === "link"} onChange={() => setAuthMode("link")} />
            {t("Само таен линк")}</label>
          <label className="radio"><input type="radio" checked={authMode === "basic"} onChange={() => setAuthMode("basic")} />
            {t("Таен линк + потребител/парола")}</label>
          {authMode === "basic" && (
            <div className="grid2">
              <label>{t("Потребител")}<input value={authUser} onChange={(e) => setAuthUser(e.target.value)} required /></label>
              <label>{d.has_password ? t("Нова парола (по желание)") : t("Парола")}
                <input type="password" autoComplete="new-password" value={authPassword} onChange={(e) => setAuthPassword(e.target.value)} minLength={10} />
              </label>
            </div>
          )}
          <p className="muted small">{t("Режимът с парола важи за личния линк. Не всички плейъри го поддържат.")}</p>
        </fieldset>}
        {error && <div className="error">{error}</div>}
        {msg && <div className="ok">{msg}</div>}
        <div className="buttons"><button className="primary">{t("Запази")}</button></div>
      </form>

      <div className="card">
        {d.info ? <h3>{t("Достъп")}</h3> : <>
        <h3>{t("Личен линк")}</h3>
        <div className="copy-row">
          <code className="mono">{d.url}</code>
          <button className="ghost" onClick={() => navigator.clipboard.writeText(d.url ?? "")}>{t("Копирай")}</button>
        </div>
        <p className="muted small">
          {t("Плейърите могат да ползват и общия адрес")} <code>{(d.url ?? "").replace(/\/d\/[^/]+\/index\.smil$/, "/p/index.smil")}</code>:{" "}
          {t("тогава се разпознават по UUID. Адресът на хъба се сменя от Настройки.")}
        </p>
        </>}
        <div className="buttons">
          {!d.info && <button className="ghost" onClick={() => confirm(t("Старият линк спира да работи веднага. Продължаваме?")) &&
            patch({ rotate_token: true }, t("Нов линк е генериран."))}>{t("Генерирай нов линк")}</button>}
          {d.status === "blocked" ? (
            <button className="ghost" onClick={() => patch({ status: "active" }, t("Устройството е активирано."))}>{t("Активирай")}</button>
          ) : (
            <button className="ghost danger" onClick={() => confirm(t("Блокираното устройство спира да получава съдържание. Продължаваме?")) &&
              patch({ status: "blocked" }, t("Устройството е блокирано."))}>{t("Блокирай")}</button>
          )}
        </div>
      </div>
    </>
  );
}
