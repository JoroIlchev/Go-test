import { useEffect, useState } from "react";
import { api, errorText, type BillingRow } from "./api";
import { t } from "./i18n";

interface Report {
  month: string;
  days: number;
  rows: BillingRow[];
}

/** Device usage per client for a month, for invoicing. */
export function BillingPanel() {
  const [month, setMonth] = useState(() => new Date().toISOString().slice(0, 7));
  const [r, setR] = useState<Report | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    setError("");
    api<Report>("GET", `/api/billing?month=${month}`).then(setR).catch((e) => setError(errorText(e)));
  }, [month]);

  const total = (k: "device_days" | "devices" | "active_end") => r?.rows.reduce((s, x) => s + x[k], 0) ?? 0;
  return (
    <section>
      <div className="title-row">
        <h2>{t("Таксуване")}</h2>
        <span className="spacer" />
        <input type="month" value={month} onChange={(e) => e.target.value && setMonth(e.target.value)} />
        <a className="button ghost" href={`/api/billing?month=${month}&format=csv`}>{t("Свали за Excel")}</a>
      </div>
      {error && <div className="error">{error}</div>}
      <p className="muted small">
        <strong>{t("Устройство-дни")}</strong>: {t("за всеки ден, в който устройството е било активно за клиента поне за малко, се брои 1.")}{" "}
        {t("Чакащо или блокирано устройство не се брои.")}{" "}
        {r ? t("За текущия месец данните са до днес ({days} дни).", { days: r.days }) : t("За текущия месец данните са до днес.")}
      </p>
      {r && (
        <table className="table">
          <thead><tr><th>{t("Клиент")}</th><th className="num">{t("Устройство-дни")}</th><th className="num">{t("Устройства през месеца")}</th><th className="num">{t("Активни в края")}</th></tr></thead>
          <tbody>
            {r.rows.map((x) => (
              <tr key={x.client_id}><td>{x.name}</td><td className="num">{x.device_days}</td><td className="num">{x.devices}</td><td className="num">{x.active_end}</td></tr>
            ))}
            <tr className="total"><td>{t("Общо")}</td><td className="num">{total("device_days")}</td><td className="num">{total("devices")}</td><td className="num">{total("active_end")}</td></tr>
          </tbody>
        </table>
      )}
    </section>
  );
}
