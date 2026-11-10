import { useEffect, useState } from "react";
import { api, errorText, type PollInfo, type PollPolicy, type TreeNode } from "./api";

/** How often the screens under a node ask for new content. */
export function PollCard({ node }: { node: TreeNode }) {
  const [info, setInfo] = useState<PollInfo | null>(null);
  const [editing, setEditing] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    api<PollInfo>("GET", `/api/nodes/${node.id}/poll`).then(setInfo).catch((e) => setError(errorText(e)));
  }, [node.id]);

  if (!info) return error ? <div className="error">{error}</div> : null;

  async function save(policy: PollPolicy | null) {
    setError("");
    try {
      setInfo(await api<PollInfo>("PUT", `/api/nodes/${node.id}/poll`, { policy }));
      setEditing(false);
    } catch (e) {
      setError(errorText(e));
    }
  }

  const source = info.own ? "зададено тук" : info.from_node_id ? `наследено от „${info.from_name}“` : "по подразбиране за системата";
  return (
    <div className="card">
      <div className="title-row">
        <h3>Проверка за ново съдържание</h3>
        <span className="spacer" />
        {info.can_edit && !editing && <button className="ghost" onClick={() => setEditing(true)}>Промени</button>}
      </div>
      {!editing && (
        <p>
          Екраните тук проверяват <strong>{info.description}</strong> <span className="muted small">({source})</span>
        </p>
      )}
      {editing && <PollForm initial={info.effective} canInherit={!!info.own} onSave={save} onCancel={() => setEditing(false)} />}
      {error && <div className="error">{error}</div>}
      <p className="muted small">
        Смяната на кампания в точния час не зависи от това: екранът я има предварително и превключва сам. Проверката определя само
        колко бързо стига нова или променена публикация. Ако екранът няма интернет в часа на проверката, опитва отново при следващата.
      </p>
    </div>
  );
}

function PollForm({ initial, canInherit, onSave, onCancel }: {
  initial: PollPolicy; canInherit: boolean; onSave: (p: PollPolicy | null) => void; onCancel: () => void;
}) {
  const [mode, setMode] = useState(initial.mode);
  const [minutes, setMinutes] = useState(initial.mode === "interval" ? initial.minutes : 15);
  const [times, setTimes] = useState((initial.times ?? ["00:00", "12:00"]).join(", "));
  const [spread, setSpread] = useState(initial.mode === "times" ? initial.spread_min : 10);

  function submit() {
    if (mode === "interval") onSave({ mode, minutes, times: null, spread_min: 0 });
    else onSave({ mode, minutes: 0, times: times.split(/[,\s]+/).filter(Boolean), spread_min: spread });
  }

  return (
    <div className="poll-form">
      <label className="radio">
        <input type="radio" checked={mode === "interval"} onChange={() => setMode("interval")} />
        На всеки <input type="number" min={1} max={1440} value={minutes} disabled={mode !== "interval"} className="narrow"
          onChange={(e) => setMinutes(Number(e.target.value))} /> минути
      </label>
      <label className="radio">
        <input type="radio" checked={mode === "times"} onChange={() => setMode("times")} />
        В определени часове: <input value={times} disabled={mode !== "times"} placeholder="00:00, 12:00"
          onChange={(e) => setTimes(e.target.value)} />
      </label>
      {mode === "times" && (
        <label className="radio sub">
          Разпредели устройствата в <input type="number" min={0} max={120} value={spread} className="narrow"
            onChange={(e) => setSpread(Number(e.target.value))} /> минути след всеки час
          <span className="muted small"> (за да не теглят всички в една секунда)</span>
        </label>
      )}
      <div className="inline-form">
        <button className="primary" onClick={submit}>Запази</button>
        {canInherit && <button className="ghost" onClick={() => onSave(null)}>Наследи отгоре</button>}
        <button className="ghost" onClick={onCancel}>Отказ</button>
      </div>
    </div>
  );
}
