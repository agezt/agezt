// Daemon-config backup (M738): the daemon default identity, prompt-template
// library, and per-task routing chains bundled into one portable file. Complements
// the appearance bundle (M735, which is per-device localStorage): this carries the
// daemon-level defaults without pretending to export every roster agent identity.
// Each section already has its own get/set command; this just bundles + restores them.

import { getJSON, postJSON } from "@/lib/api";

export interface ConfigBundle {
  persona?: string;
  prompts?: unknown[];
  chains?: Record<string, string[]>;
}

// parseConfigBundle normalises an imported file into a partial bundle, keeping only
// recognised, well-typed sections (a foreign/garbage file can't partially apply
// junk). Accepts a bare object or a `{config:{…}}` wrapper. Throws on bad JSON or a
// file with no usable section.
export function parseConfigBundle(text: string): ConfigBundle {
  const data = JSON.parse(text);
  const src = data && typeof data.config === "object" && data.config ? data.config : data;
  if (!src || typeof src !== "object" || Array.isArray(src)) {
    throw new Error("expected a config object (or a {config:{…}} wrapper)");
  }
  const out: ConfigBundle = {};
  // persona: any string (including "" — a deliberate "no default identity", restored as-is).
  if (typeof src.persona === "string") out.persona = src.persona;
  // prompts: pass the array through (the prompts_set command validates entries, the
  // same path the prompt import/export uses).
  if (Array.isArray(src.prompts)) out.prompts = src.prompts;
  // chains: keep only {task: [model strings]} entries with a non-empty chain.
  if (src.chains && typeof src.chains === "object" && !Array.isArray(src.chains)) {
    const chains: Record<string, string[]> = {};
    for (const [task, v] of Object.entries(src.chains)) {
      if (!Array.isArray(v)) continue;
      const models = v.filter((m): m is string => typeof m === "string" && m.trim() !== "").map((m) => m.trim());
      if (task.trim() && models.length) chains[task.trim()] = models;
    }
    if (Object.keys(chains).length) out.chains = chains;
  }
  if (Object.keys(out).length === 0) {
    throw new Error("no valid config sections (persona / prompts / chains) found");
  }
  return out;
}

// fetchConfigBundle gathers the daemon default identity + prompt templates + routing
// into a versioned, wrapped bundle (the shape downloadText writes / parseConfigBundle reads).
export async function fetchConfigBundle(): Promise<{ version: number; config: Required<ConfigBundle> }> {
  const [p, pr, r] = await Promise.all([
    getJSON<{ system?: string }>("/api/persona"),
    getJSON<{ prompts?: unknown[] }>("/api/prompts"),
    getJSON<{ chains?: Record<string, string[]> }>("/api/routing"),
  ]);
  return { version: 1, config: { persona: p.system || "", prompts: pr.prompts || [], chains: r.chains || {} } };
}

// applyConfigBundle restores each section a bundle carries via the existing /set
// endpoints, returning the section names it applied (for a confirmation message).
export async function applyConfigBundle(b: ConfigBundle): Promise<string[]> {
  const applied: string[] = [];
  if (b.persona != null) {
    await postJSON("/api/persona/set", { system: b.persona });
    applied.push("default identity");
  }
  if (b.prompts) {
    await postJSON("/api/prompts/set", { prompts: b.prompts });
    applied.push("prompt templates");
  }
  if (b.chains) {
    await postJSON("/api/routing/set", { chains: b.chains });
    applied.push("routing");
  }
  return applied;
}
