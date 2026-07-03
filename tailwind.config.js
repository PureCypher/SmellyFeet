/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./internal/server/templates/*.html"],
  theme: {
    extend: {
      colors: {
        ink:  { 950: "#0a0a0c", 900: "#0f0f13", 850: "#15151b", 800: "#1c1c24", 700: "#26262f" },
        line: "#26262f",
        fog:  "#8b8b97",
        accent: { DEFAULT: "#f5b13d", bright: "#ffc964", dim: "#c08a33" },
      },
      fontFamily: {
        sans: ['"IBM Plex Sans"', "ui-sans-serif", "system-ui", "sans-serif"],
        mono: ['"IBM Plex Mono"', "ui-monospace", "SFMono-Regular", "monospace"],
      },
    },
  },
};
