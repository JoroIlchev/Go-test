import type { Device, Kind, Role, TreeNode, User } from "./api";

export const kindLabel: Record<Kind, string> = {
  client: "Клиент",
  group: "Направление",
  site: "Обект",
};

export class TreeIndex {
  byId = new Map<number, TreeNode>();
  children = new Map<number | null, TreeNode[]>();
  devicesBySite = new Map<number, Device[]>();

  constructor(public nodes: TreeNode[], public devices: Device[]) {
    for (const n of nodes) this.byId.set(n.id, n);
    for (const n of nodes) {
      // A node whose parent is not visible to this user is shown as a root.
      const key = n.parent_id !== null && this.byId.has(n.parent_id) ? n.parent_id : null;
      if (!this.children.has(key)) this.children.set(key, []);
      this.children.get(key)!.push(n);
    }
    for (const d of devices) {
      if (d.node_id === null) continue;
      if (!this.devicesBySite.has(d.node_id)) this.devicesBySite.set(d.node_id, []);
      this.devicesBySite.get(d.node_id)!.push(d);
    }
  }

  /** Ancestors from the top visible node down to n itself. */
  chain(n: TreeNode): TreeNode[] {
    const out: TreeNode[] = [];
    for (let cur: TreeNode | undefined = n; cur; cur = cur.parent_id ? this.byId.get(cur.parent_id) : undefined) {
      out.unshift(cur);
    }
    return out;
  }

  label(n: TreeNode): string {
    return this.chain(n).map((x) => x.name).join(" › ");
  }

  sites(): TreeNode[] {
    return this.nodes.filter((n) => n.kind === "site").sort((a, b) => this.label(a).localeCompare(this.label(b)));
  }

  /** Devices in n's whole subtree. */
  devicesUnder(n: TreeNode): Device[] {
    const out: Device[] = [...(this.devicesBySite.get(n.id) ?? [])];
    for (const c of this.children.get(n.id) ?? []) out.push(...this.devicesUnder(c));
    return out;
  }
}

export type Perm = "view" | "create" | "edit" | "publish" | "admin";

/** Mirrors auth.Can on the server; the server still enforces it. */
export function can(u: User, n: TreeNode, idx: TreeIndex, p: Perm): boolean {
  if (u.is_super) return true;
  if (u.client_id !== n.client_id) return false;
  const ids = new Set(idx.chain(n).map((x) => x.id));
  return u.roles.some((r) => {
    if (!ids.has(r.node_id)) return false;
    if (r.role === "admin") return true;
    return p === "view" || (p === "create" && r.can_create) || (p === "edit" && r.can_edit) || (p === "publish" && r.can_publish);
  });
}

export function isAdminOf(u: User, n: TreeNode, idx: TreeIndex): boolean {
  return can(u, n, idx, "admin");
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 ** 2) return `${(n / 1024).toFixed(0)} KB`;
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1)} MB`;
  return `${(n / 1024 ** 3).toFixed(2)} GB`;
}

export function formatDuration(ms: number): string {
  const s = Math.round(ms / 1000);
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

export function roleSummary(r: Role): string {
  if (r.role === "admin") return "Админ";
  const perms = [r.can_create && "създава", r.can_edit && "редактира", r.can_publish && "публикува"].filter(Boolean);
  return `Редактор (${perms.join(", ") || "само преглед"}${r.needs_approval ? ", с одобрение" : ""})`;
}

// Players poll at least every few minutes; treat 20 minutes of silence as offline.
const ONLINE_WINDOW_MS = 20 * 60 * 1000;
const LATE_GRACE_MS = 10 * 60 * 1000;

export function deviceState(d: Device, serverTime: string): "online" | "offline" | "pending" | "blocked" | "never" {
  if (d.status === "pending") return "pending";
  if (d.status === "blocked") return "blocked";
  if (!d.last_seen_at) return "never";
  if (d.offline_since) return "offline";
  // The device was told when to come back; a little late is still fine.
  if (d.next_check_at) return Date.parse(serverTime) < Date.parse(d.next_check_at) + LATE_GRACE_MS ? "online" : "offline";
  return Date.parse(serverTime) - Date.parse(d.last_seen_at) < ONLINE_WINDOW_MS ? "online" : "offline";
}

export function ago(iso: string | null, serverTime: string): string {
  if (!iso) return "никога";
  const s = Math.max(0, Math.round((Date.parse(serverTime) - Date.parse(iso)) / 1000));
  if (s < 60) return "току-що";
  if (s < 3600) return `преди ${Math.round(s / 60)} мин`;
  if (s < 86400) return `преди ${Math.round(s / 3600)} ч`;
  return `преди ${Math.round(s / 86400)} дни`;
}
