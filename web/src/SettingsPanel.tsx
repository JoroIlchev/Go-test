import { useEffect, useState } from "react";
import { api, errorText, type BackupResult, type HubSettings } from "./api";
import { locale, t } from "./i18n";
import { formatBytes } from "./model";
import { PollForm } from "./PollCard";
import { PrepFields } from "./PrepFields";

// lang is the hub language: logs, e-mails, exports and player screens.
type Settings = HubSettings & { lang?: "bg" | "en" };

/** Hub-wide settings, for the super admin: alert e-mails and backups. */
export function SettingsPanel() {
  const [s, setS] = useState<Settings | null>(null);
  const [emails, setEmails] = useState("");
  const [msg, setMsg] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [testTo, setTestTo] = useState("");
  const [pollEdit, setPollEdit] = useState(false);

  const apply = (v: Settings) => {
    setS({ ...v, smtp_pass: "" });
    setEmails(v.alert_emails.join(", "));
  };
  useEffect(() => {
    api<Settings>("GET", "/api/settings").then(apply).catch((e) => setError(errorText(e)));
  }, []);
  if (!s) return error ? <div className="error">{error}</div> : null;

  const set = (p: Partial<Settings>) => setS({ ...s, ...p });
  async function run(fn: () => Promise<void>, ok: string) {
    setBusy(true);
    setError("");
    setMsg("");
    try {
      await fn();
      setMsg(ok);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  const save = () => run(async () => {
    apply(await api<Settings>("PUT", "/api/settings", {
      public_url: s.public_url,
      smtp_host: s.smtp_host, smtp_port: s.smtp_port, smtp_user: s.smtp_user, smtp_pass: s.smtp_pass, smtp_from: s.smtp_from,
      alert_emails: emails.split(/[,;\s]+/).filter(Boolean), alert_admins: s.alert_admins, alert_grace_min: s.alert_grace_min,
      backup_time: s.backup_time, backup_keep: s.backup_keep, backup_dir: s.backup_dir,
      image_screen: s.image_screen, image_quality: s.image_quality,
      video_auto: s.video_auto, video_max: s.video_max, video_quality: s.video_quality,
      lang: s.lang ?? "bg",
    }));
  }, t("Настройките са запазени."));

  return (
    <section>
      <div className="title-row">
        <h2>{t("Настройки на системата")}</h2>
        <span className="spacer" />
        <button className="primary" disabled={busy} onClick={save}>{t("Запази")}</button>
      </div>
      {error && <div className="error">{error}</div>}
      {msg && <div className="ok">{msg}</div>}

      <div className="card">
        <h3>{t("Език на системата")}</h3>
        <label>
          <select value={s.lang ?? "bg"} onChange={(e) => set({ lang: e.target.value === "en" ? "en" : "bg" })}>
            <option value="bg">Български</option>
            <option value="en">English</option>
          </select>
        </label>
        <p className="muted small">{t("Логове, имейли, експорти и екраните на плейърите. Админът се превключва с БГ/EN горе.")}</p>
      </div>

      <div className="card">
        <h3>{t("Адрес на хъба за устройствата")}</h3>
        <p>
          {t("Сега в линковете на устройствата:")} <strong className="mono">{s.hub_url}</strong>
          {s.hub_private && <span className="tag warn-tag">{t("вътрешен адрес")}</span>}
        </p>
        {s.hub_url_fixed ? (
          <p className="muted small">{t("Адресът е зададен при стартиране с {flag} и не се сменя оттук.", { flag: "-public-url" })}</p>
        ) : (
          <>
            <label>{t("Публичен IP или адрес")}
              <input value={s.public_url} onChange={(e) => set({ public_url: e.target.value })}
                placeholder={t("празно = автоматично ({addr})", { addr: s.detected_urls[0] ?? t("няма мрежа") })} />
            </label>
            {s.detected_urls.length > 0 && (
              <p className="small">
                {t("Адреси на тази машина:")}{" "}
                {s.detected_urls.map((u) => (
                  <button key={u} type="button" className="link mono" onClick={() => set({ public_url: u })}>{u}</button>
                )).reduce<React.ReactNode[]>((acc, el, i) => (i ? [...acc, ", ", el] : [el]), [])}
              </p>
            )}
            <p className="muted small">
              {t("Въведете статичния IP, на който устройствата ще намират машината (напр. {ip}; портът 8080 се добавя сам). Празно = IP-то на тази машина, открито автоматично.", { ip: "85.10.20.30" })}
            </p>
          </>
        )}
        {s.hub_private && (
          <div className="notice small">
            {t("{url} е адрес от локална мрежа: работи само за устройства в същата мрежа като тази машина. За устройства по обектите въведете публичния статичен IP и пренасочете на рутера порт 8080 (TCP) към тази машина.", { url: s.hub_url })}
          </div>
        )}
      </div>

      <div className="card">
        <div className="title-row">
          <h3>{t("Проверка за ново съдържание — по подразбиране")}</h3>
          <span className="spacer" />
          {!pollEdit && <button className="ghost" onClick={() => setPollEdit(true)}>{t("Промени")}</button>}
        </div>
        {!pollEdit && (
          <p>{t("Устройствата проверяват")} <strong>{s.default_poll_text}</strong>{" "}
            <span className="muted small">({s.default_poll_own ? t("зададено тук") : t("от стартирането на marquee, -refresh")})</span></p>
        )}
        {pollEdit && (
          <PollForm initial={s.default_poll} canInherit={s.default_poll_own} inheritLabel={t("Върни стойността от стартирането")}
            onCancel={() => setPollEdit(false)}
            onSave={(policy) => run(async () => {
              apply(await api<Settings>("PUT", "/api/settings/poll", { policy }));
              setPollEdit(false);
            }, t("Проверката по подразбиране е сменена. Устройствата я получават при следващата си проверка."))} />
        )}
        <p className="muted small">
          {t("Важи за всички клиенти, които нямат собствена настройка. За отделен клиент, група или обект: отворете го в дървото вляво → картата „Проверка за ново съдържание“ → Промени. Най-близката настройка нагоре по дървото печели.")}
        </p>
      </div>

      <div className="card">
        <h3>{t("Обработка на качените файлове — по подразбиране")}</h3>
        <p className="muted small">
          {t("Това е изборът по подразбиране в „Медия“. Който качва, може да го смени за своите качвания (например без смаляване за една кампания или вертикален екран за друга). Вече качените файлове не се променят.")}
        </p>
        <PrepFields value={s} onChange={set} />
        <p className="muted small">
          {t("Снимка от телефон е 4000×3000 и повече: устройството я зарежда бавно, а слабите Android кутии могат да останат без памет. Смалената снимка изглежда същата на екрана. Снимки, които вече се побират, не се пипат.")}
        </p>
        <p className="muted small">
          {t("Конвертираното видео е H.264 MP4 до 30 кадъра/сек — форматът, който всяко Android 7 устройство пуска. При автоматично конвертиране файлът се заменя (оригиналът не се пази); с бутона се прави отделно копие.")}
        </p>
      </div>

      <div className="card">
        <h3>{t("Известия за офлайн устройства")}</h3>
        <p className="muted small">
          {t("Всяко устройство знае кога да провери отново. Ако закъснее повече от толеранса, получавате имейл; когато се върне, още един.")}
        </p>
        <div className="grid2">
          <label>{t("Имейли за известия")}<input value={emails} onChange={(e) => setEmails(e.target.value)} placeholder="ops@firma.bg, az@firma.bg" /></label>
          <label>{t("Толеранс (минути закъснение)")}<input type="number" min={1} max={1440} value={s.alert_grace_min}
            onChange={(e) => set({ alert_grace_min: Number(e.target.value) })} /></label>
        </div>
        <label className="check"><input type="checkbox" checked={s.alert_admins} onChange={(e) => set({ alert_admins: e.target.checked })} />
          {t("Извести и админите на клиента (тези с админ права над устройството)")}</label>

        <h4>{t("Пощенски сървър (SMTP)")}</h4>
        <div className="grid2">
          <label>{t("Сървър")}<input value={s.smtp_host} onChange={(e) => set({ smtp_host: e.target.value })} placeholder="smtp.firma.bg" /></label>
          <label>{t("Порт")}<input type="number" value={s.smtp_port} onChange={(e) => set({ smtp_port: Number(e.target.value) })} /></label>
          <label>{t("Потребител")}<input value={s.smtp_user} onChange={(e) => set({ smtp_user: e.target.value })} autoComplete="off" /></label>
          <label>{t("Парола")}<input type="password" value={s.smtp_pass ?? ""} autoComplete="new-password"
            placeholder={s.has_password ? t("•••••• (непроменена)") : ""} onChange={(e) => set({ smtp_pass: e.target.value })} /></label>
          <label>{t("Подател")}<input value={s.smtp_from} onChange={(e) => set({ smtp_from: e.target.value })} placeholder="marquee <hub@firma.bg>" /></label>
        </div>
        <p className="muted small">{t("Порт 587 (STARTTLS) или 465 (TLS). Паролата не се изпраща по некриптирана връзка.")}</p>
        <div className="inline-form">
          <input value={testTo} onChange={(e) => setTestTo(e.target.value)} placeholder={t("до (празно = вашият имейл)")} />
          <button className="ghost" disabled={busy} onClick={() => run(async () => {
            await api("POST", "/api/settings/test-mail", { to: testTo });
          }, t("Пробното писмо е изпратено."))}>{t("Изпрати пробно писмо")}</button>
          <span className="muted small">{t("Първо запазете промените.")}</span>
        </div>
      </div>

      <div className="card">
        <h3>{t("Резервни копия")}</h3>
        <div className="grid2">
          <label>{t("Всеки ден в")}<input type="time" value={s.backup_time} onChange={(e) => set({ backup_time: e.target.value })} /></label>
          <label>{t("Пази последните")}<input type="number" min={1} max={365} value={s.backup_keep}
            onChange={(e) => set({ backup_keep: Number(e.target.value) })} /></label>
        </div>
        <label>{t("Папка")}<input value={s.backup_dir} onChange={(e) => set({ backup_dir: e.target.value })} placeholder={s.backup_path} /></label>
        <p className="muted small">
          {t("Празно =")} <span className="mono">{s.backup_path}</span>.{" "}
          {t("За истинска защита изберете друг диск или мрежова папка (напр. {dir}): ако дискът с данните откаже, копието на същия диск няма да помогне. Копира се базата и всички качени файлове (само новите).", { dir: "D:\\marquee-backup" })}
        </p>
        <LastBackup r={s.last_backup} />
        <div className="inline-form">
          <button className="ghost" disabled={busy} onClick={() => run(async () => {
            await api<BackupResult>("POST", "/api/backups");
            apply(await api<Settings>("GET", "/api/settings"));
          }, t("Резервното копие е готово."))}>{t("Направи копие сега")}</button>
        </div>
        {s.backups.length > 0 && (
          <table className="table">
            <thead><tr><th>{t("Копие на базата")}</th><th>{t("Размер")}</th></tr></thead>
            <tbody>
              {s.backups.map((b) => <tr key={b.name}><td className="mono small">{b.name}</td><td>{formatBytes(b.size)}</td></tr>)}
            </tbody>
          </table>
        )}
        <details className="small">
          <summary>{t("Как се възстановява")}</summary>
          <ol>
            <li>{t("Спрете marquee.")}</li>
            <li>{t("Копирайте избраното")} <span className="mono">marquee-….db</span> {t("като")} <span className="mono">data\marquee.db</span>{" "}
              ({t("изтрийте")} <span className="mono">marquee.db-wal</span> {t("и")} <span className="mono">-shm</span>, {t("ако ги има")}).</li>
            <li>{t("Копирайте папката")} <span className="mono">media</span> {t("от копието в")} <span className="mono">data\media</span>.</li>
            <li>{t("Пуснете marquee.")}</li>
          </ol>
        </details>
      </div>
    </section>
  );
}

function LastBackup({ r }: { r: BackupResult }) {
  if (!r.at) return <p className="muted small">{t("Още няма направено копие.")}</p>;
  const when = new Date(r.at).toLocaleString(locale());
  if (r.error) return <div className="error small">{t("Последното копие ({when}) не успя: {error}", { when, error: r.error })}</div>;
  return <p className="small">{t("Последно копие:")} <strong>{when}</strong> · {r.file} · {t("нови файлове: {n}", { n: r.media_copied })}</p>;
}
