import { theme as antdTheme, type ThemeConfig } from "antd";

import type { Theme } from "./ThemeContext";

/*
 * Ant Design theme tokens built from the same Solarized palette as
 * index.css's --color-base03..base3 / --color-<accent> custom properties
 * (see the comment there for the palette's own reasoning). The two are kept
 * in sync by hand rather than one reading the other: antd's algorithm needs
 * concrete colour values to derive hover/active/disabled states from, which
 * a CSS var() string can't give it, so the seed colours are duplicated here
 * as plain hex. If the palette ever changes, update both places.
 *
 * Only the bespoke workflow canvas (WorkflowCanvas.tsx, nodes/**, edges/**)
 * still reads the CSS custom properties directly — antd has no diagram
 * primitives, so that part of the UI stays outside this theme entirely.
 */

const fontFamily =
  "ui-sans-serif, system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif";

const accents = {
  yellow: "#b58900",
  orange: "#cb4b16",
  red: "#dc322f",
  magenta: "#d33682",
  violet: "#6c71c4",
  blue: "#268bd2",
  cyan: "#2aa198",
  green: "#859900",
};

const sharedTokens: ThemeConfig["token"] = {
  colorPrimary: accents.blue,
  colorLink: accents.blue,
  colorInfo: accents.cyan,
  colorSuccess: accents.green,
  colorWarning: accents.yellow,
  colorError: accents.red,
  borderRadius: 4,
  fontFamily,
};

export const solarizedDarkTheme: ThemeConfig = {
  algorithm: antdTheme.darkAlgorithm,
  token: {
    ...sharedTokens,
    colorBgBase: "#002b36", // base03 — surface-canvas
    colorTextBase: "#839496", // base0 — ink-body
    colorBgContainer: "#073642", // base02 — surface-raised
    colorBgElevated: "#073642",
    colorBgLayout: "#002b36",
    colorBorder: "#0d4b5a", // surface-edge
    colorBorderSecondary: "#0d4b5a",
    colorTextSecondary: "#93a1a1", // base1 — ink-strong
    colorTextTertiary: "#586e75", // base01 — ink-muted
    colorTextQuaternary: "#586e75",
    colorFillSecondary: "#0b4351", // surface-hover
    colorFillTertiary: "#0b4351",
  },
};

export const solarizedLightTheme: ThemeConfig = {
  algorithm: antdTheme.defaultAlgorithm,
  token: {
    ...sharedTokens,
    colorBgBase: "#fdf6e3", // base3 — surface-canvas
    colorTextBase: "#657b83", // base00 — ink-body
    colorBgContainer: "#eee8d5", // base2 — surface-raised
    colorBgElevated: "#eee8d5",
    colorBgLayout: "#fdf6e3",
    colorBorder: "#ddd6c1", // surface-edge
    colorBorderSecondary: "#ddd6c1",
    colorTextSecondary: "#586e75", // base01 — ink-strong
    colorTextTertiary: "#93a1a1", // base1 — ink-muted
    colorTextQuaternary: "#93a1a1",
    colorFillSecondary: "#e4dcc4", // surface-hover
    colorFillTertiary: "#e4dcc4",
  },
};

export function antdThemeFor(theme: Theme): ThemeConfig {
  return theme === "dark" ? solarizedDarkTheme : solarizedLightTheme;
}
