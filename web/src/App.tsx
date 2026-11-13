import { useCallback, useEffect, useMemo, useState } from "react";
import { Logo } from "./Logo";
import { api, ApiError, type Approval, type Device, type Tree, type User } from "./api";
import { CampaignEditor } from "./CampaignEditor";
import { ApprovalsPanel } from "./ApprovalsPanel";
import { isStaff, TreeIndex } from "./model";
import { TeamPanel } from "./TeamPanel";
import { Login } from "./Login";
import { Sidebar } from "./Sidebar";
import { NodePanel } from "./NodePanel";
import { DevicePanel } from "./DevicePanel";
import { PendingPanel } from "./PendingPanel";
import { NewClientPanel } from "./NewClientPanel";
import { SettingsPanel } from "./SettingsPanel";
import { BillingPanel } from "./BillingPanel";
import { LogsPanel } from "./LogsPanel";

export type Selection =
  | { type: "node"; id: number }
  | { type: "device"; id: number }
  | { type: "campaign"; id: number }
  | { type: "pending" }
  | { type: "approvals" }
  | { type: "new-client" }
  | { type: "settings" }
  | { type: "billing" }
  | { type: "logs" }
  | { type: "team" }
  | null;

export function App() {
  const [me, setMe] = useState<User | null | undefined>(undefined);

  useEffect(() => {
    api<User>("GET", "/api/me")
      .then(setMe)
      .catch(() => setMe(null));
  }, []);

  if (me === undefined) return null;
  if (me === null) return <Login onLogin={setMe} />;
  return <Shell me={me} onMe={setMe} onLogout={() => setMe(null)} />;
}

function Shell({ me, onMe, onLogout }: { me: User; onMe: (u: User) => void; onLogout: () => void }) {
  const [tree, setTree] = useState<Tree | null>(null);
  const [pending, setPending] = useState<Device[]>([]);
  const [approvals, setApprovals] = useState<Approval[]>([]);
  const [sel, setSelection] = useState<Selection>(null);
  // On a phone the tree is a drawer: open until something is picked.
  const [menu, setMenu] = useState(true);
  const setSel = useCallback((s: Selection) => { setSelection(s); setMenu(false); }, []);

  const staff = isStaff(me);
  const reload = useCallback(async () => {
    try {
      // Rights change under us: a new client of a manager, the team page.
      onMe(await api<User>("GET", "/api/me"));
      setTree(await api<Tree>("GET", "/api/tree"));
      setApprovals(await api<Approval[]>("GET", "/api/approvals"));
      if (staff) setPending(await api<Device[]>("GET", "/api/devices/pending"));
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) onLogout();
    }
  }, [staff, onMe, onLogout]);

  useEffect(() => {
    reload();
    // Keep "last seen" and the pending list fresh while the page is open.
    const t = setInterval(reload, 30_000);
    return () => clearInterval(t);
  }, [reload]);

  const idx = useMemo(() => (tree ? new TreeIndex(tree.nodes, tree.devices) : null), [tree]);

  // The operator's own ads live in a node made on first use.
  async function openHouse() {
    const h = await api<{ id: number }>("POST", "/api/house");
    await reload();
    setSel({ type: "node", id: h.id });
  }

  async function logout() {
    await api("POST", "/api/logout").catch(() => {});
    onLogout();
  }

  let main = (
    <div className="empty">
      Изберете звено от дървото вляво.
      <p className="phone-only"><button className="primary" onClick={() => setMenu(true)}>Отвори менюто</button></p>
    </div>
  );
  if (idx && tree && sel) {
    if (sel.type === "node" && idx.byId.has(sel.id)) {
      main = <NodePanel key={sel.id} me={me} idx={idx} node={idx.byId.get(sel.id)!} serverTime={tree.server_time}
        onChange={reload} onSelect={setSel} />;
    } else if (sel.type === "device") {
      const d = tree.devices.find((x) => x.id === sel.id);
      if (d) main = <DevicePanel key={d.id} me={me} idx={idx} device={d} serverTime={tree.server_time} onChange={reload} onSelect={setSel} />;
    } else if (sel.type === "campaign") {
      main = <CampaignEditor key={sel.id} me={me} idx={idx} id={sel.id} onSelect={(s) => { setSel(s); reload(); }} />;
    } else if (sel.type === "approvals") {
      main = <ApprovalsPanel approvals={approvals} onSelect={setSel} />;
    } else if (sel.type === "pending" && staff) {
      main = <PendingPanel idx={idx} pending={pending} hubUrl={tree.hub_url} serverTime={tree.server_time} onChange={reload} onSelect={setSel} />;
    } else if (sel.type === "new-client" && staff) {
      main = <NewClientPanel onChange={reload} onSelect={setSel} />;
    } else if (sel.type === "settings" && me.is_super) {
      main = <SettingsPanel />;
    } else if (sel.type === "billing" && (me.is_super || me.can_billing)) {
      main = <BillingPanel />;
    } else if (sel.type === "logs" && (me.is_super || me.can_logs)) {
      main = <LogsPanel />;
    } else if (sel.type === "team" && me.is_super) {
      main = <TeamPanel me={me} clients={(idx.children.get(null) ?? []).filter((n) => !n.house)} onMeChange={reload} />;
    }
  }

  return (
    <div className={"shell" + (menu ? " menu-open" : "")}>
      <header className="topbar">
        <button className="ghost icon menu-btn" onClick={() => setMenu(!menu)} aria-label="Меню" aria-expanded={menu}>☰</button>
        <span className="brand"><Logo size={28} /></span>
        <span className="spacer" />
        <span className="who"><span className="who-name">{me.name || me.email}</span>{me.is_super && <span className="badge">Супер Админ</span>}{me.is_manager && <span className="badge">Мениджър</span>}</span>
        <button className="ghost" onClick={logout}>Изход</button>
      </header>
      <div className="menu-back" onClick={() => setMenu(false)} />
      <aside className="sidebar">
        {idx && <Sidebar me={me} idx={idx} pendingCount={pending.length} approvalCount={approvals.length} sel={sel} onSelect={setSel}
          serverTime={tree!.server_time} onHouse={openHouse} />}
      </aside>
      <main className="main">{main}</main>
    </div>
  );
}
