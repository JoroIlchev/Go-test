import "@fontsource/space-grotesk/latin-700.css";

/** The "glow" mark: one light spreading out, on the brand colour. */
export function LogoMark({ size = 28 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 64 64" aria-hidden="true" className="logo-mark">
      <rect width="64" height="64" rx="16" fill="var(--brand)" />
      <circle cx="32" cy="32" r="22" fill="none" stroke="#fff" strokeWidth="3" opacity="0.28" />
      <circle cx="32" cy="32" r="14" fill="none" stroke="#fff" strokeWidth="3.5" opacity="0.6" />
      <circle cx="32" cy="32" r="7" fill="#fff" />
    </svg>
  );
}

/** Mark plus wordmark, as in the header and on the login page. */
export function Logo({ size = 28 }: { size?: number }) {
  return (
    <span className="logo" style={{ fontSize: size * 0.72 }}>
      <LogoMark size={size} />
      <span className="logo-word">marquee</span>
    </span>
  );
}
