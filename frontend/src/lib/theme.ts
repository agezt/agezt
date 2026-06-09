import { useSyncExternalStore } from "react";

export type Theme = "dark" | "light";
const KEY = "agezt-theme";

function read(): Theme {
  // An explicit, stored choice always wins.
  try {
    const stored = localStorage.getItem(KEY);
    if (stored === "dark" || stored === "light") return stored;
  } catch {
    /* storage unavailable — fall through to the OS preference */
  }
  // First run (no stored choice): honour the OS preference, defaulting to dark when
  // it's unknown or unavailable. The toggle then persists an explicit choice that
  // overrides the OS from then on.
  try {
    if (typeof matchMedia !== "undefined" && matchMedia("(prefers-color-scheme: light)").matches) {
      return "light";
    }
  } catch {
    /* matchMedia unavailable — keep the dark default */
  }
  return "dark";
}

// Single source of truth: module-level state + subscribers, so the header toggle
// and the ⌘K command share one theme and never desync (the old per-hook useState
// let the palette flip the class while the header's state went stale).
let current: Theme = read();
const listeners = new Set<() => void>();

// applyTheme reflects the current theme onto <html>. DOM-only (no persistence), so
// main.tsx can call it before first paint without locking in an OS-derived default —
// only an explicit toggle persists a choice (see setTheme). Mirrors applyAccentHue.
export function applyTheme(): void {
  document.documentElement.classList.toggle("dark", current === "dark");
}

export function getTheme(): Theme {
  return current;
}

// toggleTheme flips dark/light, applies it, and notifies every subscriber, so both
// the header button and the command palette re-render in lockstep.
export function toggleTheme(): void {
  setTheme(current === "dark" ? "light" : "dark");
}

// setTheme sets a specific theme (toggle / appearance import) — an EXPLICIT choice, so
// it persists (overriding the OS preference from then on). No-op if unchanged.
export function setTheme(theme: Theme): void {
  if (theme === current) return;
  current = theme;
  applyTheme();
  try {
    localStorage.setItem(KEY, current);
  } catch {
    /* storage unavailable — the DOM class is still applied */
  }
  for (const l of listeners) l();
}

export function useTheme() {
  const theme = useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    getTheme,
    getTheme,
  );
  return { theme, toggle: toggleTheme };
}
