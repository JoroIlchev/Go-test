import type { Approval } from "./api";
import type { Selection } from "./App";
import { locale, t } from "./i18n";

export function ApprovalsPanel({ approvals, onSelect }: { approvals: Approval[]; onSelect: (s: Selection) => void }) {
  return (
    <section>
      <h2>{t("За одобрение")}</h2>
      <div className="card">
        {approvals.length === 0 ? <p className="muted">{t("Няма публикации, които чакат одобрение.")}</p> : (
          <table className="table">
            <thead><tr><th>{t("Кампания")}</th><th>{t("Версия")}</th><th>{t("Звено")}</th><th>{t("Изпратена от")}</th><th>{t("Кога")}</th></tr></thead>
            <tbody>
              {approvals.map((a) => (
                <tr key={a.version_id}>
                  <td><button className="link" onClick={() => onSelect({ type: "campaign", id: a.campaign_id })}>{a.campaign_name}</button></td>
                  <td>v{a.number}</td>
                  <td>{a.owner_name}</td>
                  <td>{a.created_by_name}</td>
                  <td className="muted">{new Date(a.created_at).toLocaleString(locale())}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </section>
  );
}
