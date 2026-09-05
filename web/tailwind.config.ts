import type { Config } from "tailwindcss";

// The Signet palette, carried over from signet-ui's design tokens so the
// platform, the console, and the protocol's own docs read as one product:
//
//   primary  slate blue    — trust, depth. Headings, body, dark surfaces.
//   accent   sunset orange — CTAs, selected states, warnings. Used sparingly.
//   neutral  warm stone    — backgrounds and borders, warm-tinted to sit with
//                            the accent rather than fight it.
//
// Surfaces resolve through CSS variables (globals.css) so the whole console
// inverts for dark mode without every component knowing about both themes.
const config: Config = {
  darkMode: "class",
  content: [
    "./app/**/*.{ts,tsx}",
    "./components/**/*.{ts,tsx}",
    "./lib/**/*.{ts,tsx}",
    "./providers/**/*.{ts,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        primary: {
          50: "#f0f4f8",
          100: "#d9e2ec",
          200: "#bcccdc",
          300: "#9fb3c8",
          400: "#829ab1",
          500: "#627d98",
          600: "#486581",
          700: "#334e68",
          800: "#243b53",
          900: "#102a43",
          950: "#0a1929",
        },
        accent: {
          50: "#fff8f1",
          100: "#feecdc",
          200: "#fcd9bd",
          300: "#fdba8c",
          400: "#f6a354",
          500: "#e8873c",
          600: "#cb6d2a",
          700: "#a65521",
          800: "#8a4520",
          900: "#6f3720",
          950: "#3d1c0e",
        },
        neutral: {
          50: "#faf9f7",
          100: "#f0eeeb",
          200: "#e2dfd9",
          300: "#ccc8c0",
          400: "#aca69c",
          500: "#918a7e",
          600: "#756e63",
          700: "#5e5850",
          800: "#4a453f",
          900: "#38342f",
          950: "#1e1c19",
        },
        success: {
          50: "#ecfdf5",
          100: "#d1fae5",
          400: "#4ade80",
          500: "#3d9f6f",
          600: "#2f8459",
          700: "#236b47",
        },
        error: {
          50: "#fef2f0",
          100: "#fde3de",
          400: "#e8614d",
          500: "#c4523b",
          600: "#a8412e",
        },
        // Theme-resolved surfaces. Every component uses these rather than a
        // literal shade, so light and dark stay in step by construction.
        bg: "rgb(var(--bg) / <alpha-value>)",
        surface: "rgb(var(--surface) / <alpha-value>)",
        raised: "rgb(var(--raised) / <alpha-value>)",
        fg: "rgb(var(--fg) / <alpha-value>)",
        muted: "rgb(var(--muted) / <alpha-value>)",
        faint: "rgb(var(--faint) / <alpha-value>)",
        edge: "rgb(var(--edge) / <alpha-value>)",
      },
      fontFamily: {
        sans: ["var(--font-sans)", "ui-sans-serif", "system-ui", "sans-serif"],
        mono: ["var(--font-mono)", "ui-monospace", "SFMono-Regular", "Menlo", "monospace"],
      },
      maxWidth: {
        page: "1200px",
        prose: "72ch",
      },
      borderRadius: {
        xl: "0.875rem",
        "2xl": "1.125rem",
        "3xl": "1.5rem",
      },
      boxShadow: {
        card: "0 1px 2px rgb(var(--shadow) / 0.04), 0 16px 40px -24px rgb(var(--shadow) / 0.22)",
        pop: "0 24px 70px -32px rgb(var(--shadow) / 0.45)",
        glow: "0 1px 2px rgb(var(--shadow) / 0.05), 0 12px 30px -12px rgba(232, 135, 60, 0.30)",
      },
      keyframes: {
        rise: {
          from: { opacity: "0", transform: "translateY(20px)" },
          to: { opacity: "1", transform: "none" },
        },
        shimmer: {
          "100%": { transform: "translateX(100%)" },
        },
      },
      animation: {
        rise: "rise 0.7s cubic-bezier(0.22, 1, 0.36, 1) both",
      },
    },
  },
  plugins: [],
};

export default config;
