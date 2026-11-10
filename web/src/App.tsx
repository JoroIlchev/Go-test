import { useCallback, useEffect, useMemo, useState } from "react";
import { api, ApiError, type Approval, type Device, type Tree, type User } from "./api";
import { CampaignEditor } from "./CampaignEditor";
import { ApprovalsPanel } from "./ApprovalsPanel";
import { TreeIndex } from "./model";
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
  return <Shell me={me} onLogout={() => setMe(null)} />;
}

function Shell({ me, onLogout }: { me: User; onLogout: () => void }) {
  const [tree, setTree] = useState<Tree | null>(null);
  const [pending, setPending] = useState<Device[]>([]);
  const [approvals, setApprovals] = useState<Approval[]>([]);
  const [sel, setSel] = useState<Selection>(null);

  const reload = useCallback(async () => {
    try {
      setTree(await api<Tree>("GET", "/api/tree"));
      setApprovals(await api<Approval[]>("GET", "/api/approvals"));
      if (me.is_super) setPending(await api<Device[]>("GET", "/api/devices/pending"));
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) onLogout();
    }
  }, [me.is_super, onLogout]);

  useEffect(() => {
    reload();
    // Keep "last seen" and the pending list fresh while the page is open.
    const t = setInterval(reload, 30_000);
    return () => clearInterval(t);
  }, [reload]);

  const idx = useMemo(() => (tree ? new TreeIndex(tree.nodes, tree.devices) : null), [tree]);

  async function logout() {
    await api("POST", "/api/logout").catch(() => {});
    onLogout();
  }

  let main = <div className="empty">Изберете звено от дървото вляво.</div>;
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
    } else if (sel.type === "pending" && me.is_super) {
      main = <PendingPanel idx={idx} pending={pending} hubUrl={tree.hub_url} serverTime={tree.server_time} onChange={reload} onSelect={setSel} />;
    } else if (sel.type === "new-client" && me.is_super) {
      main = <NewClientPanel onChange={reload} onSelect={setSel} />;
    } else if (sel.type === "settings" && me.is_super) {
      main = <SettingsPanel />;
    } else if (sel.type === "billing" && me.is_super) {
      main = <BillingPanel />;
    } else if (sel.type === "logs" && me.is_super) {
      main = <LogsPanel />;
    }
  }

  return (
    <div className="shell">
      <header className="topbar">
        <span className="brand">marquee</span>
        <span className="spacer" />
        <span className="who">{me.name || me.email}{me.is_super && <span className="badge">супер админ</span>}</span>
        <button className="ghost" onClick={logout}>Изход</button>
      </header>
      <aside className="sidebar">
        {idx && <Sidebar me={me} idx={idx} pendingCount={pending.length} approvalCount={approvals.length} sel={sel} onSelect={setSel}
          serverTime={tree!.server_time} />}
      </aside>
      <main className="main">{main}</main>
    </div>
  );
}
