import { useSyncExternalStore } from "react";

// Accent theming (M716): the UI's accent colour is the one brand knob that makes
// the console feel "yours". We customize only the HUE — lightness and chroma stay
// per-theme (see --accent in index.css), so the accent reads well in both dark and
// light. The chosen hue is stored locally (appearance is a per-device preference)
// and applied to :root as --accent-hue, which the --accent oklch() references.

const KEY = "agezt-accent-hue";
export const DEFAULT_HUE = 255; // blue — the original accent

export interface AccentPreset {
  name: string;
  hue: number;
}

// A spread of distinct, legible hues around the oklch wheel.
export const ACCENTS: AccentPreset[] = [
  { name: "Blue", hue: 255 },
  { name: "Indigo", hue: 280 },
  { name: "Violet", hue: 300 },
  { name: "Magenta", hue: 330 },
  { name: "Rose", hue: 10 },
  { name: "Orange", hue: 45 },
  { name: "Amber", hue: 75 },
  { name: "Green", hue: 150 },
  { name: "Teal", hue: 185 },
  { name: "Cyan", hue: 220 },
];

// swatchColor renders a preset as a visible chip in either theme (a mid lightness).
export function swatchColor(hue: number): string {
  return `oklch(0.65 0.18 ${hue})`;
}

export function loadAccentHue(): number {
  const raw = typeof localStorage !== "undefined" ? localStorage.getItem(KEY) : null;
  const n = raw == null ? NaN : Number(raw);
  return Number.isFinite(n) ? n : DEFAULT_HUE;
}

// applyAccentHue sets the CSS variable the theme's --accent references. Setting the
// default removes the override so the stylesheet fallback (255) governs.
export function applyAccentHue(hue: number): void {
  const root = document.documentElement;
  if (hue === DEFAULT_HUE) root.style.removeProperty("--accent-hue");
  else root.style.setProperty("--accent-hue", String(hue));
}

// Shared store: a single current hue + subscribers, so the header picker and any
// external setter (appearance import) stay in lockstep — like lib/theme. (The old
// per-hook useState meant an import couldn't update the picker's highlighted swatch.)
let currentHue = loadAccentHue();
const listeners = new Set<() => void>();

// saveAccentHue persists, applies, and notifies — the one setter everything uses.
export function saveAccentHue(hue: number): void {
  currentHue = hue;
  try {
    localStorage.setItem(KEY, String(hue));
  } catch {
    /* storage unavailable — appearance is best-effort */
  }
  applyAccentHue(hue);
  for (const l of listeners) l();
}

export function getAccentHue(): number {
  return currentHue;
}

// useAccent is the component-facing hook: current hue + a setter that persists and
// applies immediately, re-rendering every consumer on change.
export function useAccent() {
  const hue = useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    getAccentHue,
    getAccentHue,
  );
  return { hue, setHue: saveAccentHue };
}
