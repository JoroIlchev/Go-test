import type { Approval } from "./api";
import type { Selection } from "./App";

export function ApprovalsPanel({ approvals, onSelect }: { approvals: Approval[]; onSelect: (s: Selection) => void }) {
  return (
    <section>
      <h2>За одобрение</h2>
      <div className="card">
        {approvals.length === 0 ? <p className="muted">Няма публикации, които чакат одобрение.</p> : (
          <table className="table">
            <thead><tr><th>Кампания</th><th>Версия</th><th>Звено</th><th>Изпратена от</th><th>Кога</th></tr></thead>
            <tbody>
              {approvals.map((a) => (
                <tr key={a.version_id}>
                  <td><button className="link" onClick={() => onSelect({ type: "campaign", id: a.campaign_id })}>{a.campaign_name}</button></td>
                  <td>v{a.number}</td>
                  <td>{a.owner_name}</td>
                  <td>{a.created_by_name}</td>
                  <td className="muted">{new Date(a.created_at).toLocaleString("bg-BG")}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </section>
  );
}
