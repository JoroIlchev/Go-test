import { useCallback, useEffect, useState } from "react";
import { api, errorText, type TrustedDevice } from "./api";
import { locale, t } from "./i18n";

/** Browsers the user chose to remember at login, with a way to forget them. */
export function TrustedPanel() {
  const [list, setList] = useState<TrustedDevice[] | null>(null);
  const [error, setError] = useState("");

  const load = useCallback(() => {
    api<TrustedDevice[]>("GET", "/api/me/trusted").then(setList).catch((e) => setError(errorText(e)));
  }, []);
  useEffect(() => { load(); }, [load]);

  async function forget(path: string) {
    setError("");
    try {
      await api("DELETE", path);
      load();
    } catch (e) {
      setError(errorText(e));
    }
  }

  return (
    <section>
      <div className="title-row">
        <h2>{t("Устройства за вход")}</h2>
        <span className="spacer" />
        {!!list?.length && <button className="ghost" onClick={() => forget("/api/me/trusted")}>{t("Забрави всички")}</button>}
      </div>
      <p className="muted">
        {t("Браузъри, в които сте влезли с отметка „Запомни това устройство“. Те влизат дори докато някой друг опитва да познае паролата ви. Паролата се иска винаги. При смяна на паролата всички се забравят, а неизползван 90 дни се забравя сам.")}
      </p>
      {error && <div className="error">{error}</div>}
      {list && list.length === 0 && <p className="muted">{t("Няма запомнени устройства.")}</p>}
      {list && list.length > 0 && (
        <table className="table">
          <thead>
            <tr><th>{t("Браузър")}</th><th>{t("Последен вход")}</th><th>{t("IP")}</th><th>{t("Запомнено")}</th><th /></tr>
          </thead>
          <tbody>
            {list.map((d) => (
              <tr key={d.id}>
                <td>{browserName(d.user_agent)}{d.current && <span className="badge">{t("това устройство")}</span>}</td>
                <td>{new Date(d.last_used_at).toLocaleString(locale())}</td>
                <td className="mono">{d.ip}</td>
                <td className="muted">{new Date(d.created_at).toLocaleDateString(locale())}</td>
                <td><button className="ghost" onClick={() => forget(`/api/me/trusted/${d.id}`)}>{t("Забрави")}</button></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

// browserName turns a User-Agent into "Chrome · Windows", enough to tell
// one's own browsers apart.
function browserName(ua: string): string {
  const browser =
    /Edg\//.test(ua) ? "Edge" :
    /OPR\/|Opera/.test(ua) ? "Opera" :
    /Firefox\//.test(ua) ? "Firefox" :
    /SamsungBrowser\//.test(ua) ? "Samsung Internet" :
    /Chrome\//.test(ua) ? "Chrome" :
    /Safari\//.test(ua) ? "Safari" : "";
  const os =
    /Android/.test(ua) ? "Android" :
    /iPhone|iPad/.test(ua) ? "iOS" :
    /Windows/.test(ua) ? "Windows" :
    /Mac OS X/.test(ua) ? "macOS" :
    /Linux/.test(ua) ? "Linux" : "";
  return [browser, os].filter(Boolean).join(" · ") || ua || "?";
}
