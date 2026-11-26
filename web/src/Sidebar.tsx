import { useState } from "react";
import type { TreeNode, User } from "./api";
import { deviceState, isStaff, type TreeIndex } from "./model";
import type { Selection } from "./App";
import { t } from "./i18n";

interface Props {
  me: User;
  idx: TreeIndex;
  pendingCount: number;
  approvalCount: number;
  sel: Selection;
  serverTime: string;
  onSelect: (s: Selection) => void;
  onHouse: () => void;
}

export function Sidebar({ me, idx, pendingCount, approvalCount, sel, serverTime, onSelect, onHouse }: Props) {
  const all = idx.children.get(null) ?? [];
  const roots = all.filter((n) => !n.house);
  const house = all.find((n) => n.house);
  const staff = isStaff(me);
  const isAdminSomewhere = staff || me.roles.some((r) => r.role === "admin");
  const offline = idx.devices.filter((d) => deviceState(d, serverTime) === "offline").length;
  return (
    <nav className="tree">
      {isAdminSomewhere && !staff && (
        <div className="tree-actions">
          <button className={"nav-item" + (sel?.type === "approvals" ? " active" : "")} onClick={() => onSelect({ type: "approvals" })}>
            {t("За одобрение")} {approvalCount > 0 && <span className="count">{approvalCount}</span>}
          </button>
        </div>
      )}
      {staff && (
        <div className="tree-actions">
          <button className={"nav-item" + (sel?.type === "pending" ? " active" : "")} onClick={() => onSelect({ type: "pending" })}>
            {t("Чакащи устройства")} {pendingCount > 0 && <span className="count">{pendingCount}</span>}
          </button>
          <button className={"nav-item" + (sel?.type === "approvals" ? " active" : "")} onClick={() => onSelect({ type: "approvals" })}>
            {t("За одобрение")} {approvalCount > 0 && <span className="count">{approvalCount}</span>}
          </button>
          <button className={"nav-item" + (sel?.type === "new-client" ? " active" : "")} onClick={() => onSelect({ type: "new-client" })}>
            + {t("Нов клиент")}
          </button>
          {(me.is_super || me.can_house) && (
            <button className={"nav-item" + (house && sel?.type === "node" && sel.id === house.id ? " active" : "")} onClick={onHouse}>
              ★ {t("Моята реклама")}
            </button>
          )}
          {(me.is_super || me.can_billing) && (
            <button className={"nav-item" + (sel?.type === "billing" ? " active" : "")} onClick={() => onSelect({ type: "billing" })}>
              {t("Таксуване")}
            </button>
          )}
          {(me.is_super || me.can_logs) && (
            <button className={"nav-item" + (sel?.type === "logs" ? " active" : "")} onClick={() => onSelect({ type: "logs" })}>
              {t("Логове")}
            </button>
          )}
          {me.is_super && (
            <button className={"nav-item" + (sel?.type === "team" ? " active" : "")} onClick={() => onSelect({ type: "team" })}>
              {t("Екип")}
            </button>
          )}
          {me.is_super && (
            <button className={"nav-item" + (sel?.type === "settings" ? " active" : "")} onClick={() => onSelect({ type: "settings" })}>
              {t("Настройки")} {offline > 0 && <span className="count warn" title={t("Офлайн устройства")}>{t("{n} офлайн", { n: offline })}</span>}
            </button>
          )}
        </div>
      )}
      {roots.length === 0 && <div className="muted pad">{t("Няма звена.")}</div>}
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
        <button className="twisty" onClick={() => setOpen(!open)} aria-label={open ? t("Свий") : t("Разгъни")}
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
                  {d.name || t("Без име")}
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </li>
  );
}
