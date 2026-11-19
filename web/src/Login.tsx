import { useState, type FormEvent } from "react";
import { Logo } from "./Logo";
import { LangSwitch } from "./LangSwitch";
import { t } from "./i18n";
import { api, errorText, type User } from "./api";

export function Login({ onLogin }: { onLogin: (u: User) => void }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      onLogin(await api<User>("POST", "/api/login", { email, password }));
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="login-wrap">
      <form className="card login" onSubmit={submit}>
        <h1 className="brand"><Logo size={40} /></h1>
        <label>
          {t("Имейл")}
          <input type="email" autoComplete="username" value={email} onChange={(e) => setEmail(e.target.value)} required autoFocus />
        </label>
        <label>
          {t("Парола")}
          <input type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
        </label>
        {error && <div className="error">{error}</div>}
        <button className="primary" disabled={busy}>{t("Вход")}</button>
        <div className="login-lang"><LangSwitch /></div>
      </form>
    </div>
  );
}
