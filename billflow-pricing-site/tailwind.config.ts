import type { Config } from "tailwindcss";

export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        graphite: "#F6F8FA",
        charcoal: "#FFFFFF",
        ivory: "#111827",
        muted: "#4B5563",
        line: "#E5E7EB",
        slate: "#F1F5F9",
        gold: "#B8872D",
        steel: "#12324A",
        success: "#16803C",
      },
      fontFamily: {
        sans: [
          "Inter",
          "IBM Plex Sans Thai",
          "Noto Sans Thai",
          "ui-sans-serif",
          "system-ui",
          "sans-serif",
        ],
      },
      boxShadow: {
        card: "0 10px 28px rgba(17, 24, 39, 0.07)",
        soft: "0 6px 18px rgba(17, 24, 39, 0.06)",
      },
    },
  },
  plugins: [],
} satisfies Config;
