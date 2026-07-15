/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./internal/server/templates/*.html"],
  theme: {
    extend: {
      // Colors resolve to the CSS custom properties in assets/tailwind.input.css
      // (docs/frontend/DESIGN_TOKENS.md is the source of truth) so existing
      // utility class names (bg-ink-900, text-accent, ...) keep working while
      // the token layer becomes the single place values are defined.
      colors: {
        ink:  { 950: "var(--surface-page)", 900: "var(--surface-raised)", 850: "var(--surface-hover)", 800: "var(--surface-overlay)", 700: "var(--border-default)" },
        line: "var(--border-default)",
        fog:  "var(--text-muted)",
        accent: { DEFAULT: "var(--accent)", bright: "var(--accent-bright)", dim: "var(--accent-dim)" },
        // Signal Lamp severity system — additive, brand accent untouched above.
        status: { ok: "var(--status-ok)", warn: "var(--status-warn)", critical: "var(--status-critical)" },
        // Additional text-color roles from DESIGN_TOKENS.md §2 (for templates
        // migrating off Tailwind's stock zinc-* scale route by route).
        primary: "var(--text-primary)",
        secondary: "var(--text-secondary)",
        body: "var(--text-body)",
        structural: "var(--text-structural)",
        faint: "var(--text-faint)",
      },
      fontFamily: {
        sans: ['"IBM Plex Sans"', "ui-sans-serif", "system-ui", "sans-serif"],
        mono: ['"IBM Plex Mono"', "ui-monospace", "SFMono-Regular", "monospace"],
      },
    },
  },
};
