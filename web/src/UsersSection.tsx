import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api, errorText, type Role, type TreeNode, type User } from "./api";
import { roleSummary, type TreeIndex, isStaff } from "./model";

interface Props {
  me: User;
  idx: TreeIndex;
  node: TreeNode;
}

const emptyRole = (nodeId: number): Role => ({
  node_id: nodeId, role: "editor", can_create: true, can_edit: true, can_publish: false, needs_approval: true,
});

export function UsersSection({ me, idx, node }: Props) {
  const [users, setUsers] = useState<User[]>([]);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<User | "new" | null>(null);

  const staff = isStaff(me);
  const load = useCallback(async () => {
    try {
      const q = staff ? `?client_id=${node.client_id}` : "";
      setUsers(await api<User[]>("GET", "/api/users" + q));
    } catch (e) {
      setError(errorText(e));
    }
  }, [staff, node.client_id]);

  useEffect(() => { load(); }, [load]);

  const chainIds = new Set(idx.chain(node).map((n) => n.id));
  const here = users.filter((u) => u.roles.some((r) => r.node_id === node.id));
  const inherited = users.filter((u) => !here.includes(u) &&
    u.roles.some((r) => r.role === "admin" && chainIds.has(r.node_id)));

  return (
    <div className="card">
      <div className="title-row">
        <h3>Потребители</h3>
        <span className="spacer" />
        {editing === null && <button className="ghost" onClick={() => setEditing("new")}>+ Нов потребител</button>}
      </div>
      {error && <div className="error">{error}</div>}
      {editing !== null && (
        <UserForm key={editing === "new" ? "new" : editing.id} me={me} node={node}
          user={editing === "new" ? null : editing}
          onDone={async () => { setEditing(null); await load(); }} />
      )}
      {here.length === 0 ? <p className="muted">Няма потребители, назначени на това звено.</p> : (
        <table className="table">
          <thead><tr><th>Име</th><th>Имейл</th><th>Права тук</th><th /></tr></thead>
          <tbody>
            {here.map((u) => {
              const r = u.roles.find((x) => x.node_id === node.id)!;
              return (
                <tr key={u.id} className={u.active ? "" : "inactive"}>
                  <td>{u.name}{u.id === me.id && <span className="muted"> (вие)</span>}</td>
                  <td>{u.email}</td>
                  <td>{roleSummary(r)}{!u.active && <span className="pill">спрян</span>}</td>
                  <td>{u.id !== me.id && <button className="link" onClick={() => setEditing(u)}>Редакция</button>}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
      {inherited.length > 0 && (
        <p className="muted small">
          Достъп и от по-горе: {inherited.map((u) => u.name || u.email).join(", ")}
        </p>
      )}
    </div>
  );
}

function UserForm({ me, node, user, onDone }: { me: User; node: TreeNode; user: User | null; onDone: () => Promise<void> }) {
  const existingRole = user?.roles.find((r) => r.node_id === node.id);
  const [email, setEmail] = useState(user?.email ?? "");
  const [name, setName] = useState(user?.name ?? "");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<Role>(existingRole ?? emptyRole(node.id));
  const [active, setActive] = useState(user?.active ?? true);
  const [error, setError] = useState("");

  const set = (patch: Partial<Role>) => setRole({ ...role, ...patch });

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    try {
      if (user === null) {
        await api("POST", "/api/users", {
          email, name, password, roles: [role],
          client_id: isStaff(me) ? node.client_id : undefined,
        });
      } else {
        // Roles are replaced as a whole: keep the user's roles elsewhere untouched.
        const roles = [...user.roles.filter((r) => r.node_id !== node.id), role];
        await api("PATCH", `/api/users/${user.id}`, {
          name, roles, active, password: password || undefined,
        });
      }
      await onDone();
    } catch (err) {
      setError(errorText(err));
    }
  }

  return (
    <form className="subcard form" onSubmit={submit}>
      <h4>{user ? `Редакция: ${user.email}` : `Нов потребител в „${node.name}“`}</h4>
      <div className="grid2">
        <label>Имейл<input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required disabled={!!user} /></label>
        <label>Име<input value={name} onChange={(e) => setName(e.target.value)} /></label>
        <label>{user ? "Нова парола (по желание)" : "Парола"}
          <input type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)}
            required={!user} minLength={10} placeholder="поне 10 символа" />
        </label>
      </div>
      <fieldset>
        <legend>Роля в „{node.name}“ и всичко под него</legend>
        <label className="radio"><input type="radio" checked={role.role === "admin"} onChange={() => set({ role: "admin" })} />
          Админ: всичко, вкл. потребители и одобрения</label>
        <label className="radio"><input type="radio" checked={role.role === "editor"} onChange={() => set({ role: "editor" })} />
          Редактор с права:</label>
        {role.role === "editor" && (
          <div className="perms">
            <label className="check"><input type="checkbox" checked={role.can_create} onChange={(e) => set({ can_create: e.target.checked })} /> създаване</label>
            <label className="check"><input type="checkbox" checked={role.can_edit} onChange={(e) => set({ can_edit: e.target.checked })} /> редактиране</label>
            <label className="check"><input type="checkbox" checked={role.can_publish} onChange={(e) => set({ can_publish: e.target.checked })} /> публикуване</label>
            <label className="check"><input type="checkbox" checked={role.needs_approval} onChange={(e) => set({ needs_approval: e.target.checked })} /> публикациите изискват одобрение от админ</label>
          </div>
        )}
      </fieldset>
      {user && (
        <label className="check"><input type="checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} /> Активен (спирането го изхвърля веднага)</label>
      )}
      {error && <div className="error">{error}</div>}
      <div className="buttons">
        <button className="primary">{user ? "Запази" : "Създай"}</button>
        <button type="button" className="ghost" onClick={() => onDone()}>Отказ</button>
      </div>
    </form>
  );
}
