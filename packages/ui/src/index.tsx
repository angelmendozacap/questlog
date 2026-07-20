import type { PropsWithChildren } from "react";

/**
 * System-F panel. `variant="signature"` renders the full frame treatment
 * (accent border + corner rivets) reserved for the user's own panel.
 * Placeholder implementation — grows with the design system (Phase 4+).
 */
export function Panel({
  variant = "default",
  children,
}: PropsWithChildren<{ variant?: "default" | "signature" }>) {
  const base: React.CSSProperties = {
    background: "var(--ql-panel)",
    padding: "18px 20px",
    border:
      variant === "signature"
        ? "1px solid color-mix(in srgb, var(--ql-accent) 55%, transparent)"
        : "1px solid var(--ql-frame-dim)",
  };
  return <div style={base}>{children}</div>;
}
