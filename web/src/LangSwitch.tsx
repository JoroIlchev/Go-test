import { api } from "./api";
import { setLang, useLang, type Lang } from "./i18n";

// LangSwitch flips the admin between Bulgarian and English; signed in, the
// choice is saved with the user.
export function LangSwitch({ save }: { save?: boolean }) {
  const cur = useLang();
  function pick(l: Lang) {
    setLang(l);
    if (save) api("PUT", "/api/me/lang", { lang: l }).catch(() => {});
  }
  return (
    <span className="lang-switch" role="group" aria-label="Language">
      {(["bg", "en"] as const).map((l) => (
        <button key={l} type="button" className={l === cur ? "on" : ""} aria-pressed={l === cur} onClick={() => pick(l)}>
          {l === "bg" ? "БГ" : "EN"}
        </button>
      ))}
    </span>
  );
}
