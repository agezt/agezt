import { useSyncExternalStore } from "react";

// Advanced mode (global, persisted). OFF by default = the calm, visual console:
// metrics, gauges and motion stay, but text-heavy / diagnostic detail is folded
// away. Flip it ON to reveal the power-user prose, raw dumps and knobs. Mirrors
// lib/theme.ts: one module-level source of truth + an `.advanced` class on <html>
// so both React (<Advanced>) and CSS can react, shared by the header toggle and
// the ⌘K palette without desync.

const KEY = "agezt-advanced";

function read(): boolean {
  try {
    return localStorage.getItem(KEY) === "1";
  } catch {
    return false; // storage unavailable → calm default
  }
}

let current = read();
const listeners = new Set<() => void>();

// applyAdvanced reflects the current mode onto <html> (DOM-only, no persistence),
// so main.tsx can call it before first paint. Mirrors applyTheme.
export function applyAdvanced(): void {
  document.documentElement.classList.toggle("advanced", current);
}

export function getAdvanced(): boolean {
  return current;
}

export function setAdvanced(on: boolean): void {
  if (on === current) return;
  current = on;
  applyAdvanced();
  try {
    localStorage.setItem(KEY, on ? "1" : "0");
  } catch {
    /* storage unavailable — the DOM class is still applied */
  }
  for (const l of listeners) l();
}

export function toggleAdvanced(): void {
  setAdvanced(!current);
}

export function useAdvanced() {
  const advanced = useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    getAdvanced,
    getAdvanced,
  );
  return { advanced, toggle: toggleAdvanced };
}
