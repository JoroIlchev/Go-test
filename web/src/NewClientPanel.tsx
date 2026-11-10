import { useState, type FormEvent } from "react";
import { api, errorText, type TreeNode } from "./api";
import type { Selection } from "./App";

export function NewClientPanel({ onChange, onSelect }: { onChange: () => Promise<void>; onSelect: (s: Selection) => void }) {
  const [name, setName] = useState("");
  const [error, setError] = useState("");

  async function submit(e: FormEvent) {
    e.preventDefault();
    try {
      const n = await api<TreeNode>("POST", "/api/nodes", { parent_id: null, kind: "client", name });
      await onChange();
      onSelect({ type: "node", id: n.id });
    } catch (err) {
      setError(errorText(err));
    }
  }

  return (
    <section>
      <h2>Нов клиент</h2>
      <form className="card form" onSubmit={submit}>
        <label>
          Име на клиента
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Иванов ЕООД" required autoFocus />
        </label>
        {error && <div className="error">{error}</div>}
        <div><button className="primary">Създай</button></div>
        <p className="muted">След това добавете направления и обекти, и създайте главен админ на клиента.</p>
      </form>
    </section>
  );
}
