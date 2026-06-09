import { useState } from "react";

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

export function saveConsoleName(name: string): string {
  const clean = name.trim().slice(0, MAX_NAME) || DEFAULT_NAME;
  try {
    if (clean === DEFAULT_NAME) localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, clean);
  } catch {
    /* storage unavailable — appearance is best-effort */
  }
  applyConsoleTitle(clean);
  return clean;
}

// useConsoleName is the component-facing hook: current name + a setter that persists,
// updates the document title, and returns the normalized value.
export function useConsoleName() {
  const [name, setName] = useState<string>(() => loadConsoleName());
  return {
    name,
    setName: (n: string) => setName(saveConsoleName(n)),
  };
}
