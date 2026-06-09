import { useSyncExternalStore } from "react";

// Console name (M719): the one word in the header — "agezt · console" — is yours to
// change. A per-device appearance preference (like the accent), stored locally and
// reflected into document.title so the browser tab matches too.

const KEY = "agezt-console-name";
export const DEFAULT_NAME = "agezt";
const MAX_NAME = 32;

export function loadConsoleName(): string {
  const raw = typeof localStorage !== "undefined" ? localStorage.getItem(KEY) : null;
  const n = (raw ?? "").trim();
  return n || DEFAULT_NAME;
}

export function applyConsoleTitle(name: string): void {
  if (typeof document !== "undefined") document.title = `${name} · console`;
}

// Shared store: one current name + subscribers, so the header rename control and an
// external setter (appearance import) stay in lockstep — like lib/theme & accent.
let currentName = loadConsoleName();
const listeners = new Set<() => void>();

export function saveConsoleName(name: string): string {
  const clean = name.trim().slice(0, MAX_NAME) || DEFAULT_NAME;
  currentName = clean;
  try {
    if (clean === DEFAULT_NAME) localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, clean);
  } catch {
    /* storage unavailable — appearance is best-effort */
  }
  applyConsoleTitle(clean);
  for (const l of listeners) l();
  return clean;
}

export function getConsoleName(): string {
  return currentName;
}

// useConsoleName is the component-facing hook: current name + a setter that persists,
// updates the document title, and re-renders every consumer on change.
export function useConsoleName() {
  const name = useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    getConsoleName,
    getConsoleName,
  );
  return { name, setName: saveConsoleName };
}
