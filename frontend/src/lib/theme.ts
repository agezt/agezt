import { useSyncExternalStore } from "react";

export type Theme = "dark" | "light";
const KEY = "agezt-theme";

function read(): Theme {
  try {
    return (localStorage.getItem(KEY) as Theme) || "dark";
  } catch {
    return "dark";
  }
}

// Single source of truth: module-level state + subscribers, so the header toggle
// and the ⌘K command share one theme and never desync (the old per-hook useState
// let the palette flip the class while the header's state went stale).
let current: Theme = read();
const listeners = new Set<() => void>();

// applyTheme reflects the current theme onto <html> and persists it. Exported so
// main.tsx can call it before first paint (no flash of the wrong theme), mirroring
// applyAccentHue / applyConsoleTitle.
export function applyTheme(): void {
  document.documentElement.classList.toggle("dark", current === "dark");
  try {
    localStorage.setItem(KEY, current);
  } catch {
    /* storage unavailable — the DOM class is still applied */
  }
}

export function getTheme(): Theme {
  return current;
}

// toggleTheme flips dark/light, applies it, and notifies every subscriber, so both
// the header button and the command palette re-render in lockstep.
export function toggleTheme(): void {
  current = current === "dark" ? "light" : "dark";
  applyTheme();
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
