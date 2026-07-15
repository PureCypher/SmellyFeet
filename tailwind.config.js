/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./internal/server/templates/*.html"],
  theme: {
    extend: {
      // Colors resolve to the CSS custom properties in assets/tailwind.input.css
      // (docs/frontend/DESIGN_TOKENS.md is the source of truth) so existing
      // utility class names (bg-ink-900, text-accent, ...) keep working while
      // the token layer becomes the single place values are defined.
      //
      // Uses rgb(var(--x-rgb) / <alpha-value>), not a bare var(--x), because
      // Tailwind's opacity modifiers (bg-accent/10, border-line/50, ...) need
      // to decompose the color into channels at build time — a bare var()
      // reference is opaque to Tailwind and silently drops the modifier.
      colors: {
        ink: {
          950: "rgb(var(--surface-page-rgb) / <alpha-value>)",
          900: "rgb(var(--surface-raised-rgb) / <alpha-value>)",
          850: "rgb(var(--surface-hover-rgb) / <alpha-value>)",
          800: "rgb(var(--surface-overlay-rgb) / <alpha-value>)",
          700: "rgb(var(--border-default-rgb) / <alpha-value>)",
        },
        line: "rgb(var(--border-default-rgb) / <alpha-value>)",
        fog:  "rgb(var(--text-muted-rgb) / <alpha-value>)",
        accent: {
          DEFAULT: "rgb(var(--accent-rgb) / <alpha-value>)",
          bright: "rgb(var(--accent-bright-rgb) / <alpha-value>)",
          dim: "rgb(var(--accent-dim-rgb) / <alpha-value>)",
        },
        // Signal Lamp severity system — additive, brand accent untouched above.
        // status.warn deliberately reuses --accent-rgb (DESIGN_TOKENS.md §5).
        status: {
          ok: "rgb(var(--status-ok-rgb) / <alpha-value>)",
          warn: "rgb(var(--accent-rgb) / <alpha-value>)",
          critical: "rgb(var(--status-critical-rgb) / <alpha-value>)",
        },
        // Additional text-color roles from DESIGN_TOKENS.md §2 (for templates
        // migrating off Tailwind's stock zinc-* scale route by route).
        primary: "rgb(var(--text-primary-rgb) / <alpha-value>)",
        secondary: "rgb(var(--text-secondary-rgb) / <alpha-value>)",
        body: "rgb(var(--text-body-rgb) / <alpha-value>)",
        structural: "rgb(var(--text-structural-rgb) / <alpha-value>)",
        faint: "rgb(var(--text-faint-rgb) / <alpha-value>)",
      },
      fontFamily: {
        sans: ['"IBM Plex Sans"', "ui-sans-serif", "system-ui", "sans-serif"],
        mono: ['"IBM Plex Mono"', "ui-monospace", "SFMono-Regular", "monospace"],
      },
    },
  },
};
