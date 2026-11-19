import { useSyncExternalStore } from "react";
import { en } from "./i18n/en";

// The admin speaks Bulgarian by default. Texts stay in Bulgarian in the code
// and t() looks up the English one by that text; a missing translation falls
// back to Bulgarian. The choice is stored with the user (PUT /api/me/lang)
// and, for the login page, in this browser.

export type Lang = "bg" | "en";

const KEY = "marquee.lang";

function stored(): Lang {
  try {
    return localStorage.getItem(KEY) === "en" ? "en" : "bg";
  } catch {
    return "bg";
  }
}

let current: Lang = stored();
document.documentElement.lang = current;
const listeners = new Set<() => void>();

export function lang(): Lang {
  return current;
}

export function setLang(l: Lang) {
  if (l === current) return;
  current = l;
  document.documentElement.lang = l;
  try {
    localStorage.setItem(KEY, l);
  } catch {
    // private mode: the choice lasts until reload
  }
  listeners.forEach((f) => f());
}

// useLang re-renders the caller when the language changes.
export function useLang(): Lang {
  return useSyncExternalStore(
    (f) => {
      listeners.add(f);
      return () => listeners.delete(f);
    },
    () => current,
  );
}

const missing = new Set<string>();

// t translates a Bulgarian text; {name} placeholders are filled from vars.
export function t(bg: string, vars?: Record<string, string | number>): string {
  let s = bg;
  if (current === "en") {
    const tr = en[bg];
    if (tr !== undefined) s = tr;
    else if (import.meta.env.DEV && !missing.has(bg)) {
      missing.add(bg);
      console.warn("i18n: no English for", JSON.stringify(bg));
    }
  }
  if (vars) s = s.replace(/\{(\w+)\}/g, (m, k: string) => (k in vars ? String(vars[k]) : m));
  return s;
}

// locale is for dates and numbers.
export function locale(): string {
  return current === "en" ? "en-GB" : "bg-BG";
}
