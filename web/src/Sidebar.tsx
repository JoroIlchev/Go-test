import { useState } from "react";
import type { TreeNode, User } from "./api";
import { deviceState, isStaff, type TreeIndex } from "./model";
import type { Selection } from "./App";

interface Props {
  me: User;
  idx: TreeIndex;
  pendingCount: number;
  approvalCount: number;
  sel: Selection;
  serverTime: string;
  onSelect: (s: Selection) => void;
}

export function Sidebar({ me, idx, pendingCount, approvalCount, sel, serverTime, onSelect }: Props) {
  const roots = idx.children.get(null) ?? [];
  const staff = isStaff(me);
  const isAdminSomewhere = staff || me.roles.some((r) => r.role === "admin");
  const offline = idx.devices.filter((d) => deviceState(d, serverTime) === "offline").length;
  return (
    <nav className="tree">
      {isAdminSomewhere && !staff && (
        <div className="tree-actions">
          <button className={"nav-item" + (sel?.type === "approvals" ? " active" : "")} onClick={() => onSelect({ type: "approvals" })}>
            За одобрение {approvalCount > 0 && <span className="count">{approvalCount}</span>}
          </button>
        </div>
      )}
      {staff && (
        <div className="tree-actions">
          <button className={"nav-item" + (sel?.type === "pending" ? " active" : "")} onClick={() => onSelect({ type: "pending" })}>
            Чакащи устройства {pendingCount > 0 && <span className="count">{pendingCount}</span>}
          </button>
          <button className={"nav-item" + (sel?.type === "approvals" ? " active" : "")} onClick={() => onSelect({ type: "approvals" })}>
            За одобрение {approvalCount > 0 && <span className="count">{approvalCount}</span>}
          </button>
          <button className={"nav-item" + (sel?.type === "new-client" ? " active" : "")} onClick={() => onSelect({ type: "new-client" })}>
            + Нов клиент
          </button>
          {(me.is_super || me.can_billing) && (
            <button className={"nav-item" + (sel?.type === "billing" ? " active" : "")} onClick={() => onSelect({ type: "billing" })}>
              Таксуване
            </button>
          )}
          {(me.is_super || me.can_logs) && (
            <button className={"nav-item" + (sel?.type === "logs" ? " active" : "")} onClick={() => onSelect({ type: "logs" })}>
              Логове
            </button>
          )}
          {me.is_super && (
            <button className={"nav-item" + (sel?.type === "team" ? " active" : "")} onClick={() => onSelect({ type: "team" })}>
              Екип
            </button>
          )}
          {me.is_super && (
            <button className={"nav-item" + (sel?.type === "settings" ? " active" : "")} onClick={() => onSelect({ type: "settings" })}>
              Настройки {offline > 0 && <span className="count warn" title="Офлайн устройства">{offline} офлайн</span>}
            </button>
          )}
        </div>
      )}
      {roots.length === 0 && <div className="muted pad">Няма звена.</div>}
      <ul>
        {roots.map((n) => (
          <Branch key={n.id} node={n} idx={idx} sel={sel} serverTime={serverTime} onSelect={onSelect} />
        ))}
      </ul>
    </nav>
  );
}

function Branch({ node, idx, sel, serverTime, onSelect }: {
  node: TreeNode; idx: TreeIndex; sel: Selection; serverTime: string; onSelect: (s: Selection) => void;
}) {
  const [open, setOpen] = useState(node.kind !== "site");
  const children = idx.children.get(node.id) ?? [];
  const devices = idx.devicesBySite.get(node.id) ?? [];
  const hasKids = children.length + devices.length > 0;
  const active = sel?.type === "node" && sel.id === node.id;

  return (
    <li>
      <div className={"row" + (active ? " active" : "")}>
        <button className="twisty" onClick={() => setOpen(!open)} aria-label={open ? "Свий" : "Разгъни"}
          style={{ visibility: hasKids ? "visible" : "hidden" }}>
          {open ? "▾" : "▸"}
        </button>
        <button className={"label kind-" + node.kind} onClick={() => onSelect({ type: "node", id: node.id })}>
          {node.name}
        </button>
      </div>
      {open && hasKids && (
        <ul>
          {children.map((c) => (
            <Branch key={c.id} node={c} idx={idx} sel={sel} serverTime={serverTime} onSelect={onSelect} />
          ))}
          {devices.map((d) => (
            <li key={"d" + d.id}>
              <div className={"row" + (sel?.type === "device" && sel.id === d.id ? " active" : "")}>
                <span className="twisty" />
                <button className="label device" onClick={() => onSelect({ type: "device", id: d.id })}>
                  <span className={"dot " + deviceState(d, serverTime)} />
                  {d.name || "Без име"}
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </li>
  );
}
