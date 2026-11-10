import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api, errorText, type CampaignState, type CampaignSummary, type TreeNode, type User } from "./api";
import { can, type TreeIndex } from "./model";
import type { Selection } from "./App";

export const stateLabel: Record<CampaignState, string> = {
  draft: "Чернова",
  pending: "Чака одобрение",
  active: "Излъчва се",
  scheduled: "Насрочена",
  expired: "Изтекла",
  stopped: "Спряна",
};

export function StateBadge({ state }: { state: CampaignState }) {
  return <span className={"tag st-" + state}>{stateLabel[state]}</span>;
}

export function CampaignList({ me, idx, node, onSelect }: {
  me: User; idx: TreeIndex; node: TreeNode; onSelect: (s: Selection) => void;
}) {
  const [items, setItems] = useState<CampaignSummary[]>([]);
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const canCreate = can(me, node, idx, "create");

  const load = useCallback(async () => {
    try {
      setItems(await api<CampaignSummary[]>("GET", `/api/campaigns?node_id=${node.id}`));
    } catch (e) {
      setError(errorText(e));
    }
  }, [node.id]);

  useEffect(() => { load(); }, [load]);

  async function create(e: FormEvent) {
    e.preventDefault();
    try {
      const c = await api<{ id: number }>("POST", "/api/campaigns", { node_id: node.id, name });
      onSelect({ type: "campaign", id: c.id });
    } catch (err) {
      setError(errorText(err));
    }
  }

  return (
    <div className="card">
      {canCreate && (
        <form className="inline-form" onSubmit={create} style={{ marginTop: 0, marginBottom: 12 }}>
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Име на нова кампания, напр. Черен петък" required />
          <button className="primary">+ Нова кампания в „{node.name}“</button>
        </form>
      )}
      {error && <div className="error">{error}</div>}
      {items.length === 0 ? <p className="muted">Няма кампании.</p> : (
        <table className="table">
          <thead><tr><th>Кампания</th><th>Състояние</th><th>Версия на живо</th><th>Звено</th></tr></thead>
          <tbody>
            {items.map((c) => (
              <tr key={c.id}>
                <td><button className="link" onClick={() => onSelect({ type: "campaign", id: c.id })}>{c.name}</button></td>
                <td>
                  <StateBadge state={c.state} />
                  {c.has_pending && c.state !== "pending" && <span className="tag st-pending" style={{ marginLeft: 6 }}>има чакаща версия</span>}
                </td>
                <td className="muted">{c.live ? `v${c.live.number}` : "—"}{c.draft_dirty && c.live && " · има промени"}</td>
                <td className="muted">{c.owner_name}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
