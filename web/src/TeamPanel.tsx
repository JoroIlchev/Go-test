import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api, errorText, type TreeNode, type User } from "./api";

type Kind = "super" | "manager";

interface Access {
  kind: Kind;
  all_clients: boolean;
  clients: number[];
  can_billing: boolean;
  can_logs: boolean;
}

const accessOf = (u: User): Access => ({
  kind: u.is_super ? "super" : "manager", all_clients: u.all_clients, clients: u.clients,
  can_billing: u.can_billing, can_logs: u.can_logs,
});

/** The hub operator's people: super admins and managers (super admin only). */
export function TeamPanel({ me, clients, onMeChange }: { me: User; clients: TreeNode[]; onMeChange: () => void }) {
  const [staff, setStaff] = useState<User[]>([]);
  const [edit, setEdit] = useState<User | null | undefined>(undefined); // null = new
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      setStaff(await api<User[]>("GET", "/api/staff"));
    } catch (e) {
      setError(errorText(e));
    }
  }, []);
  useEffect(() => { load(); }, [load]);

  const clientName = (id: number) => clients.find((c) => c.id === id)?.name ?? `#${id}`;
  function scope(u: User) {
    if (u.is_super) return "Всичко, вкл. настройки и екип";
    const list = u.all_clients ? "Всички клиенти" : u.clients.length ? u.clients.map(clientName).join(", ") : "Няма клиенти";
    const extra = [u.can_billing && "таксуване", u.can_logs && "логове"].filter(Boolean).join(", ");
    return extra ? `${list} · ${extra}` : list;
  }

  return (
    <section>
      <h2>Екип</h2>
      <p className="muted">
        Хората на хъба. <strong>Супер Админ</strong> може всичко. <strong>Мениджър</strong> работи с дадените му клиенти
        (и с тези, които сам създаде): звена, потребители, кампании, одобрения и свързване на устройства.
      </p>
      <div className="card">
        <table className="table">
          <thead><tr><th>Име</th><th>Имейл</th><th>Роля</th><th>Достъп</th><th /></tr></thead>
          <tbody>
            {staff.map((u) => (
              <tr key={u.id} className={u.active ? "" : "muted"}>
                <td>{u.name || "—"}{u.id === me.id && <span className="muted small"> (вие)</span>}</td>
                <td>{u.email}</td>
                <td>{u.is_super ? "Супер Админ" : "Мениджър"}{!u.active && <span className="muted small"> · спрян</span>}</td>
                <td className="small">{scope(u)}</td>
                <td><button className="ghost" onClick={() => setEdit(u)}>Редакция</button></td>
              </tr>
            ))}
          </tbody>
        </table>
        {edit === undefined && <p><button className="primary" onClick={() => setEdit(null)}>+ Добави човек</button></p>}
        {error && <div className="error">{error}</div>}
      </div>
      {edit !== undefined && (
        <StaffForm key={edit?.id ?? "new"} user={edit} me={me} clients={clients} onCancel={() => setEdit(undefined)}
          onDone={async () => {
            setEdit(undefined);
            await load();
            if (edit?.id === me.id) onMeChange();
          }} />
      )}
    </section>
  );
}

function StaffForm({ user, me, clients, onCancel, onDone }: {
  user: User | null; me: User; clients: TreeNode[]; onCancel: () => void; onDone: () => Promise<void>;
}) {
  const [email, setEmail] = useState(user?.email ?? "");
  const [name, setName] = useState(user?.name ?? "");
  const [password, setPassword] = useState("");
  const [active, setActive] = useState(user?.active ?? true);
  const [a, setA] = useState<Access>(user ? accessOf(user) :
    { kind: "manager", all_clients: false, clients: [], can_billing: false, can_logs: false });
  const [error, setError] = useState("");
  const set = (p: Partial<Access>) => setA({ ...a, ...p });
  const toggle = (id: number, on: boolean) =>
    set({ clients: on ? [...a.clients, id] : a.clients.filter((c) => c !== id) });

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    try {
      if (user === null) {
        await api("POST", "/api/staff", { email, name, password, ...a });
      } else {
        if (user.id === me.id && (!active || a.kind !== "super") &&
          !confirm("Ще загубите правата на Супер Админ и няма да виждате тази страница. Продължаване?")) return;
        await api("PATCH", `/api/staff/${user.id}`, { name, active, password: password || undefined, access: a });
      }
      await onDone();
    } catch (err) {
      setError(errorText(err));
    }
  }

  return (
    <form className="card form" onSubmit={submit}>
      <h3>{user ? `Редакция: ${user.email}` : "Нов човек в екипа"}</h3>
      <div className="grid2">
        <label>Имейл<input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required disabled={!!user} /></label>
        <label>Име<input value={name} onChange={(e) => setName(e.target.value)} /></label>
        <label>{user ? "Нова парола (по желание)" : "Парола"}
          <input type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)}
            required={!user} minLength={10} placeholder="поне 10 символа" />
        </label>
      </div>
      <fieldset>
        <legend>Роля</legend>
        <label className="radio"><input type="radio" checked={a.kind === "super"} onChange={() => set({ kind: "super" })} />
          Супер Админ: всичко, вкл. настройки на хъба и екипа</label>
        <label className="radio"><input type="radio" checked={a.kind === "manager"} onChange={() => set({ kind: "manager" })} />
          Мениджър: работи с избраните клиенти</label>
      </fieldset>
      {a.kind === "manager" && (
        <fieldset>
          <legend>Клиенти</legend>
          <label className="check"><input type="checkbox" checked={a.all_clients} onChange={(e) => set({ all_clients: e.target.checked })} />
            <strong>Всички клиенти</strong> (вкл. бъдещите)</label>
          {!a.all_clients && (
            clients.length === 0 ? <p className="muted small">Още няма клиенти.</p> : (
              <div className="perms client-picks">
                {clients.map((c) => (
                  <label key={c.id} className="check">
                    <input type="checkbox" checked={a.clients.includes(c.id)} onChange={(e) => toggle(c.id, e.target.checked)} /> {c.name}
                  </label>
                ))}
              </div>
            )
          )}
          <p className="muted small">Клиентите, които мениджърът създаде сам, се добавят към него автоматично.</p>
          <label className="check"><input type="checkbox" checked={a.can_billing} onChange={(e) => set({ can_billing: e.target.checked })} />
            Достъп до таксуване (само за неговите клиенти)</label>
          <label className="check"><input type="checkbox" checked={a.can_logs} onChange={(e) => set({ can_logs: e.target.checked })} />
            Достъп до логовете на хъба</label>
        </fieldset>
      )}
      {user && (
        <label className="check"><input type="checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} /> Активен (спирането го изхвърля веднага)</label>
      )}
      {error && <div className="error">{error}</div>}
      <div className="buttons">
        <button className="primary">{user ? "Запази" : "Създай"}</button>
        <button type="button" className="ghost" onClick={onCancel}>Отказ</button>
      </div>
    </form>
  );
}
