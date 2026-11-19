import { useState, type FormEvent } from "react";
import { api, errorText, type Device, type Kind, type TreeNode, type User } from "./api";
import { ago, deviceState, isAdminOf, kindLabel, type TreeIndex, isStaff } from "./model";
import type { Selection } from "./App";
import { UsersSection } from "./UsersSection";
import { MediaLibrary } from "./MediaLibrary";
import { CampaignList } from "./CampaignList";
import { PollCard } from "./PollCard";
import { t } from "./i18n";

interface Props {
  me: User;
  idx: TreeIndex;
  node: TreeNode;
  serverTime: string;
  onChange: () => Promise<void>;
  onSelect: (s: Selection) => void;
}

export function NodePanel({ me, idx, node, serverTime, onChange, onSelect }: Props) {
  const admin = isAdminOf(me, node, idx);
  const parent = node.parent_id !== null ? idx.byId.get(node.parent_id) : undefined;
  const canDelete = me.is_super || (parent !== undefined && isAdminOf(me, parent, idx));
  const children = idx.children.get(node.id) ?? [];
  const devices = idx.devicesUnder(node);
  const [error, setError] = useState("");
  const [tab, setTab] = useState<"overview" | "campaigns" | "media">(node.house ? "campaigns" : "overview");

  if (node.house) {
    return (
      <section>
        <div className="title-row"><h2>★ {t("Моята реклама")}</h2></div>
        <p className="muted">
          {t("Вашите реклами, които се въртят на екраните на клиентите: всяка кампания тук се пуска определен брой пъти на ден, разпределени равномерно в часовете, в които екранът показва съдържание. Клиентите не виждат този раздел и не могат да променят рекламите.")}
        </p>
        <div className="tabs">
          <button className={tab === "campaigns" ? "on" : ""} onClick={() => setTab("campaigns")}>{t("Кампании")}</button>
          <button className={tab === "media" ? "on" : ""} onClick={() => setTab("media")}>{t("Медия")}</button>
        </div>
        {tab === "campaigns" && <CampaignList me={me} idx={idx} node={node} onSelect={onSelect} />}
        {tab === "media" && <MediaLibrary me={me} idx={idx} node={node} />}
      </section>
    );
  }

  async function rename() {
    const name = prompt(t("Ново име"), node.name);
    if (!name || name === node.name) return;
    try {
      await api("PATCH", `/api/nodes/${node.id}`, { name });
      await onChange();
    } catch (e) {
      setError(errorText(e));
    }
  }

  async function remove() {
    if (!confirm(t("Да изтрия ли „{name}“?", { name: node.name }))) return;
    try {
      await api("DELETE", `/api/nodes/${node.id}`);
      await onChange();
      onSelect(parent ? { type: "node", id: parent.id } : null);
    } catch (e) {
      setError(errorText(e));
    }
  }

  return (
    <section>
      <Breadcrumb idx={idx} node={node} onSelect={onSelect} />
      <div className="title-row">
        <h2>{node.name}</h2>
        <span className={"pill kind-" + node.kind}>{kindLabel[node.kind]}</span>
        <span className="spacer" />
        {admin && <button className="ghost" onClick={rename}>{t("Преименувай")}</button>}
        {canDelete && <button className="ghost danger" onClick={remove}>{t("Изтрий")}</button>}
      </div>
      {error && <div className="error">{error}</div>}

      <div className="tabs">
        <button className={tab === "overview" ? "on" : ""} onClick={() => setTab("overview")}>{t("Общ преглед")}</button>
        <button className={tab === "campaigns" ? "on" : ""} onClick={() => setTab("campaigns")}>{t("Кампании")}</button>
        <button className={tab === "media" ? "on" : ""} onClick={() => setTab("media")}>{t("Медия")}</button>
      </div>

      {tab === "campaigns" && <CampaignList me={me} idx={idx} node={node} onSelect={onSelect} />}
      {tab === "media" && <MediaLibrary me={me} idx={idx} node={node} />}
      {tab === "overview" && <>
      {node.kind !== "site" && (
        <div className="card">
          <h3>{t("Подзвена")}</h3>
          {children.length === 0 ? <p className="muted">{t("Няма подзвена.")}</p> : (
            <ul className="list">
              {children.map((c) => (
                <li key={c.id}>
                  <button className="link" onClick={() => onSelect({ type: "node", id: c.id })}>{c.name}</button>
                  <span className={"pill kind-" + c.kind}>{kindLabel[c.kind]}</span>
                  <span className="muted">{t("{n} устр.", { n: idx.devicesUnder(c).length })}</span>
                </li>
              ))}
            </ul>
          )}
          {admin && <AddChild parent={node} onChange={onChange} onSelect={onSelect} />}
        </div>
      )}

      <div className="card">
        <h3>{t("Устройства")} {devices.length > 0 && <span className="muted">({devices.length})</span>}</h3>
        <DeviceTable devices={devices} idx={idx} showSite={node.kind !== "site"} serverTime={serverTime} onSelect={onSelect} />
        {isStaff(me) && node.kind === "site" && <AddLinkDevice site={node} onChange={onChange} onSelect={onSelect} />}
      </div>

      <PollCard node={node} />

      {admin && <UsersSection me={me} idx={idx} node={node} />}
      </>}
    </section>
  );
}

export function Breadcrumb({ idx, node, onSelect }: { idx: TreeIndex; node: TreeNode; onSelect: (s: Selection) => void }) {
  const chain = idx.chain(node).slice(0, -1);
  if (chain.length === 0) return null;
  return (
    <div className="crumbs">
      {chain.map((n) => (
        <span key={n.id}>
          <button className="link" onClick={() => onSelect({ type: "node", id: n.id })}>{n.name}</button> ›{" "}
        </span>
      ))}
    </div>
  );
}

const stateLabel = {
  get online() { return t("онлайн"); },
  get offline() { return t("офлайн"); },
  get pending() { return t("чака"); },
  get blocked() { return t("блокирано"); },
  get never() { return t("не се е свързвало"); },
};

export function DeviceTable({ devices, idx, showSite, serverTime, onSelect }: {
  devices: Device[]; idx: TreeIndex; showSite: boolean; serverTime: string; onSelect: (s: Selection) => void;
}) {
  if (devices.length === 0) return <p className="muted">{t("Няма устройства.")}</p>;
  return (
    <table className="table">
      <thead>
        <tr><th>{t("Устройство")}</th>{showSite && <th>{t("Обект")}</th>}<th>{t("Състояние")}</th><th>{t("Последно")}</th></tr>
      </thead>
      <tbody>
        {devices.map((d) => {
          const st = deviceState(d, serverTime);
          const site = d.node_id !== null ? idx.byId.get(d.node_id) : undefined;
          return (
            <tr key={d.id}>
              <td><button className="link" onClick={() => onSelect({ type: "device", id: d.id })}>{d.name || t("Без име")}</button></td>
              {showSite && <td>{site?.name}</td>}
              <td><span className={"dot " + st} /> {stateLabel[st]}</td>
              <td className="muted">{ago(d.last_seen_at, serverTime)}</td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

function AddChild({ parent, onChange, onSelect }: { parent: TreeNode; onChange: () => Promise<void>; onSelect: (s: Selection) => void }) {
  const [kind, setKind] = useState<Kind>("group");
  const [name, setName] = useState("");
  const [error, setError] = useState("");

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    try {
      const n = await api<TreeNode>("POST", "/api/nodes", { parent_id: parent.id, kind, name });
      setName("");
      await onChange();
      onSelect({ type: "node", id: n.id });
    } catch (err) {
      setError(errorText(err));
    }
  }

  return (
    <form className="inline-form" onSubmit={submit}>
      <select value={kind} onChange={(e) => setKind(e.target.value as Kind)}>
        <option value="group">{t("Направление")}</option>
        <option value="site">{t("Обект")}</option>
      </select>
      <input value={name} onChange={(e) => setName(e.target.value)} placeholder={kind === "site" ? t("Кафе Център") : t("Кафета")} required />
      <button className="primary">{t("Добави")}</button>
      {error && <span className="error">{error}</span>}
    </form>
  );
}

function AddLinkDevice({ site, onChange, onSelect }: { site: TreeNode; onChange: () => Promise<void>; onSelect: (s: Selection) => void }) {
  const [name, setName] = useState("");
  const [error, setError] = useState("");

  async function submit(e: FormEvent) {
    e.preventDefault();
    try {
      const d = await api<Device>("POST", "/api/devices", { site_id: site.id, name });
      setName("");
      await onChange();
      onSelect({ type: "device", id: d.id });
    } catch (err) {
      setError(errorText(err));
    }
  }

  return (
    <form className="inline-form" onSubmit={submit}>
      <input value={name} onChange={(e) => setName(e.target.value)} placeholder={t("Име на екрана")} required />
      <button className="ghost">+ {t("Устройство с личен линк")}</button>
      {error && <span className="error">{error}</span>}
      <span className="muted small">{t("Самостоятелно регистриралите се устройства се свързват от „Чакащи устройства“.")}</span>
    </form>
  );
}
