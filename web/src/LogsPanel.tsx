import { useCallback, useEffect, useState } from "react";
import { api, errorText } from "./api";
import { t } from "./i18n";

interface Logs {
  day: string;
  days: string[];
  lines: string[];
  truncated: boolean;
  file: string;
}

type Level = "all" | "warn" | "error";
// Level words as the hub writes them, in either hub language.
const LEVEL_OF: [string, Level][] = [["ГРЕШКА", "error"], ["ERROR", "error"], ["ВНИМАНИЕ", "warn"], ["WARN", "warn"]];

/** The hub's log for a day, newest first, with a level filter and search. */
export function LogsPanel() {
  const [day, setDay] = useState("");
  const [logs, setLogs] = useState<Logs | null>(null);
  const [level, setLevel] = useState<Level>("all");
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(() => {
    api<Logs>("GET", `/api/logs${day ? `?day=${day}` : ""}`).then(setLogs).catch((e) => setError(errorText(e)));
  }, [day]);
  useEffect(() => { load(); }, [load]);

  const q = query.trim().toLowerCase();
  const shown = (logs?.lines ?? []).filter((l) => {
    const lv = lineLevel(l);
    if (level === "error" && lv !== "error") return false;
    if (level === "warn" && lv === "info") return false;
    return !q || l.toLowerCase().includes(q);
  }).reverse();

  return (
    <section>
      <div className="title-row">
        <h2>{t("Логове")}</h2>
        <span className="spacer" />
        <select value={logs?.day ?? ""} onChange={(e) => setDay(e.target.value)}>
          {(logs?.days.length ? logs.days : [logs?.day ?? ""]).map((d) => <option key={d} value={d}>{d}</option>)}
        </select>
        <button className="ghost" onClick={load}>{t("Обнови")}</button>
      </div>
      {error && <div className="error">{error}</div>}
      <div className="toolbar">
        <div className="seg">
          <button className={level === "all" ? "on" : ""} onClick={() => setLevel("all")}>{t("Всички")}</button>
          <button className={level === "warn" ? "on" : ""} onClick={() => setLevel("warn")}>{t("Внимание и грешки")}</button>
          <button className={level === "error" ? "on" : ""} onClick={() => setLevel("error")}>{t("Само грешки")}</button>
        </div>
        <input className="search" placeholder={t("Търсене…")} value={query} onChange={(e) => setQuery(e.target.value)} />
        <span className="spacer" />
        <span className="muted small">{t("{n} реда, най-новите отгоре", { n: shown.length })}</span>
      </div>
      {logs?.truncated && <p className="muted small">{t("Показани са последните 5000 реда от деня.")}</p>}
      <div className="log-view">
        {shown.length === 0 ? <div className="muted">{t("Няма записи.")}</div> : shown.map((l, i) => (
          <div key={i} className={"log-line lv-" + lineLevel(l)}>{l}</div>
        ))}
      </div>
      {logs && <p className="muted small">{t("Файл:")} <span className="mono">{logs.file}</span>. {t("Отваря се и с Notepad. Пазят се 90 дни.")}</p>}
    </section>
  );
}

function lineLevel(l: string): "info" | "warn" | "error" {
  const head = l.slice(19, 31);
  for (const [word, lv] of LEVEL_OF) if (head.includes(word)) return lv === "error" ? "error" : "warn";
  return "info";
}
