// toolbox.ts (M956) — pure helpers + the install stream client for the CLI
// Toolbox page. The catalog + detection live in the Go backend (install runs
// real host package managers there); this module shapes the wire data for the
// view and is unit-tested. Mirrors the lib/fleet.ts / lib/agentdetail.ts
// discipline: pure logic here, rendering in views/Toolbox.tsx.
import { withToken } from "@/lib/api";
import { parseSSEChunk, type ChatFrame } from "@/lib/chat";

// ToolStatus mirrors kernel/toolbox.ToolStatus (the /api/toolbox wire shape).
export interface ToolStatus {
  name: string;
  category: string;
  description: string;
  installed: boolean;
  version?: string;
  path?: string;
  installable: boolean;
  manager?: string;
  command?: string;
}

export interface Inventory {
  os: string;
  managers: string[];
  tools: ToolStatus[];
  installed_count: number;
  missing_count: number;
}

export type ToolCategory =
  | "runtime" | "pkgmgr" | "vcs" | "search" | "data"
  | "media" | "build" | "net" | "archive" | "cloud" | "shell" | "ai";

// CATEGORY_LABELS — display order matches kernel/toolbox.Categories().
export const CATEGORY_LABELS: Record<ToolCategory, string> = {
  runtime: "Runtimes",
  pkgmgr: "Package managers",
  vcs: "Version control",
  search: "Search & files",
  data: "Databases & data",
  media: "Media & docs",
  build: "Build toolchain",
  net: "Network",
  archive: "Archives",
  cloud: "Cloud & k8s",
  shell: "Shell tools",
  ai: "AI",
};

export type ToolFilter = "all" | "installed" | "missing" | ToolCategory;

// filterTools applies the active chip + free-text search. "installed"/"missing"
// are status filters; anything else is a category. Pure + unit-tested.
export function filterTools(tools: ToolStatus[], filter: ToolFilter, query: string): ToolStatus[] {
  const q = query.trim().toLowerCase();
  return tools.filter((t) => {
    if (filter === "installed" && !t.installed) return false;
    if (filter === "missing" && t.installed) return false;
    if (filter !== "all" && filter !== "installed" && filter !== "missing" && t.category !== filter) return false;
    if (!q) return true;
    return (
      t.name.toLowerCase().includes(q) ||
      (t.description || "").toLowerCase().includes(q) ||
      (t.manager || "").toLowerCase().includes(q)
    );
  });
}

export interface ToolCensus {
  total: number;
  installed: number;
  missing: number;
  outdated: number;
  installableMissing: number;
}

// census folds the inventory + the outdated set into the headline counters.
export function census(tools: ToolStatus[], outdated: Set<string>): ToolCensus {
  const c: ToolCensus = { total: tools.length, installed: 0, missing: 0, outdated: 0, installableMissing: 0 };
  for (const t of tools) {
    if (t.installed) {
      c.installed++;
      if (outdated.has(t.name)) c.outdated++;
    } else {
      c.missing++;
      if (t.installable) c.installableMissing++;
    }
  }
  return c;
}

// categoriesPresent returns the categories that actually appear in the catalog,
// in the canonical display order, so chips don't show empty groups.
export function categoriesPresent(tools: ToolStatus[]): ToolCategory[] {
  const present = new Set(tools.map((t) => t.category));
  return (Object.keys(CATEGORY_LABELS) as ToolCategory[]).filter((c) => present.has(c));
}

// InstallProgress is one tool's streamed outcome (kernel/toolbox.InstallResult).
export interface InstallProgress {
  tool: string;
  ok: boolean;
  skipped?: boolean;
  manager?: string;
  command?: string;
  version?: string;
  output_tail?: string;
  error?: string;
}

// streamInstall POSTs the names to /api/toolbox/install and yields each SSE
// frame (open → per-tool progress → done/error), reusing the chat SSE parser.
export async function streamInstall(
  names: string[],
  onFrame: (f: ChatFrame) => void,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch(withToken("/api/toolbox/install"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ names }),
    signal,
  });
  if (!res.ok || !res.body) {
    let msg = `HTTP ${res.status}`;
    try {
      const j = await res.json();
      if (j?.error) msg = String(j.error);
    } catch {
      /* no JSON body */
    }
    throw new Error(msg);
  }
  const reader = res.body.getReader();
  const dec = new TextDecoder();
  let buf = "";
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buf += dec.decode(value, { stream: true });
    const { frames, rest } = parseSSEChunk(buf);
    buf = rest;
    for (const f of frames) onFrame(f);
  }
}
