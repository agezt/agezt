import { getJSON, postJSON, postAction } from "@/lib/api";
import { applyConfigBundle } from "@/lib/configbackup";
import { parseStandingJSON } from "@/views/Standing";
import { parseSchedulesJSON } from "@/views/Schedules";
import { parseMemoryJSON } from "@/views/Memory";
import { parseWorldJSON } from "@/views/World";

// Full snapshot (M741): a record of daemon-level defaults and knowledge — default
// identity, prompt templates, routing, standing orders, schedules, memory and the
// world model — gathered into one JSON for backup, audit or migration. Originally
// export-only; M752 made it RESTORABLE by replaying each section through the same
// per-domain importers the individual views use (config replaces; standing/schedules
// are additive; memory/world dedupe). Restore is gated behind an explicit confirm
// in the Backup view.

export interface FullSnapshot {
  version: number;
  exported_note: string;
  config: { persona: string; prompts: unknown[]; chains: Record<string, string[]> };
  standing: unknown[];
  schedules: unknown[];
  memory: unknown[];
  world: { entities: unknown[]; relations: unknown[] };
}

// snapshotCounts summarises a snapshot for the UI (so you see roughly what you grabbed).
export function snapshotCounts(s: FullSnapshot): string {
  return [
    s.config.persona.trim() ? "default identity" : "no default identity",
    `${s.config.prompts.length} prompt templates`,
    `${Object.keys(s.config.chains).length} chains`,
    `${s.standing.length} standing`,
    `${s.schedules.length} schedules`,
    `${s.memory.length} memories`,
    `${s.world.entities.length} entities`,
  ].join(" · ");
}

// fetchFullSnapshot reads every customizable surface in parallel and assembles the
// record. A failed section degrades to empty rather than failing the whole export.
export async function fetchFullSnapshot(): Promise<FullSnapshot> {
  const safe = async <T>(p: Promise<T>, fallback: T): Promise<T> => {
    try {
      return await p;
    } catch {
      return fallback;
    }
  };
  const [defaultIdentity, promptTemplates, routing, standing, schedules, memory, world] = await Promise.all([
    safe(getJSON<{ system?: string }>("/api/persona"), {}),
    safe(getJSON<{ prompts?: unknown[] }>("/api/prompts"), {}),
    safe(getJSON<{ chains?: Record<string, string[]> }>("/api/routing"), {}),
    safe(getJSON<{ orders?: unknown[] }>("/api/standing"), {}),
    safe(getJSON<{ schedules?: unknown[] }>("/api/schedules"), {}),
    safe(getJSON<{ records?: unknown[] }>("/api/memory"), {}),
    safe(getJSON<{ entities?: unknown[]; edges?: unknown[]; relations?: unknown[] }>("/api/world"), {}),
  ]);
  return {
    version: 1,
    exported_note: "Whole-daemon snapshot. Restore replays each section through the per-domain importers; standing/schedules are additive, memory/world dedupe.",
    config: { persona: defaultIdentity.system || "", prompts: promptTemplates.prompts || [], chains: routing.chains || {} },
    standing: standing.orders || [],
    schedules: schedules.schedules || [],
    memory: memory.records || [],
    world: { entities: world.entities || [], relations: world.relations || world.edges || [] },
  };
}

// parseSnapshotJSON validates + normalises an imported snapshot file into a FullSnapshot,
// tolerating missing sections (a partial snapshot still restores what it has). Throws on
// bad JSON, a non-object, or a snapshot with nothing restorable.
export function parseSnapshotJSON(text: string): FullSnapshot {
  const d = JSON.parse(text);
  if (!d || typeof d !== "object" || Array.isArray(d)) throw new Error("expected a snapshot object");
  const cfg = d.config && typeof d.config === "object" && !Array.isArray(d.config) ? d.config : {};
  const world = d.world && typeof d.world === "object" && !Array.isArray(d.world) ? d.world : {};
  const arr = (v: unknown): unknown[] => (Array.isArray(v) ? v : []);
  const snap: FullSnapshot = {
    version: typeof d.version === "number" ? d.version : 1,
    exported_note: typeof d.exported_note === "string" ? d.exported_note : "",
    config: {
      persona: typeof cfg.persona === "string" ? cfg.persona : "",
      prompts: arr(cfg.prompts),
      chains: cfg.chains && typeof cfg.chains === "object" && !Array.isArray(cfg.chains) ? cfg.chains : {},
    },
    standing: arr(d.standing),
    schedules: arr(d.schedules),
    memory: arr(d.memory),
    world: { entities: arr(world.entities), relations: arr(world.relations).length ? arr(world.relations) : arr(world.edges) },
  };
  const total =
    (snap.config.persona.trim() ? 1 : 0) +
    snap.config.prompts.length +
    Object.keys(snap.config.chains).length +
    snap.standing.length +
    snap.schedules.length +
    snap.memory.length +
    snap.world.entities.length;
  if (total === 0) throw new Error("snapshot has no restorable content");
  return snap;
}

// applyFullSnapshot restores a snapshot by replaying each section through the SAME
// per-domain importers the individual views use. Config (persona/prompts/routing) is
// replaced; standing & schedules are additive (re-adding duplicates — intended for a
// fresh daemon); memory & world are content-addressed so they dedupe. Each section is
// best-effort — one failing section never aborts the rest. Returns a human summary of
// what landed (e.g. "config (default identity+prompt templates)", "2/2 standing", "3/3 memories").
export async function applyFullSnapshot(snap: FullSnapshot): Promise<string[]> {
  const summary: string[] = [];

  // Config: default identity + prompt templates + routing (replace via the config bundle's /set calls).
  try {
    const applied = await applyConfigBundle({
      persona: snap.config.persona,
      prompts: snap.config.prompts,
      chains: snap.config.chains,
    });
    if (applied.length) summary.push(`config (${applied.join("+")})`);
  } catch {
    /* skip config, keep restoring */
  }

  // Standing orders (additive). parseStandingJSON throws on an empty/invalid section.
  try {
    const orders = parseStandingJSON(JSON.stringify(snap.standing));
    let n = 0;
    for (const o of orders) {
      try {
        await postJSON("/api/standing/add", { order: o });
        n++;
      } catch {
        /* skip */
      }
    }
    if (orders.length) summary.push(`${n}/${orders.length} standing`);
  } catch {
    /* empty/invalid */
  }

  // Schedules (additive).
  try {
    const scheds = parseSchedulesJSON(JSON.stringify(snap.schedules));
    let n = 0;
    for (const a of scheds) {
      try {
        await postJSON("/api/schedule/add", a);
        n++;
      } catch {
        /* skip */
      }
    }
    if (scheds.length) summary.push(`${n}/${scheds.length} schedules`);
  } catch {
    /* empty/invalid */
  }

  // Memory (content-addressed → idempotent).
  try {
    const mems = parseMemoryJSON(JSON.stringify(snap.memory));
    let n = 0;
    for (const a of mems) {
      try {
        await postJSON("/api/memory/add", a);
        n++;
      } catch {
        /* skip */
      }
    }
    if (mems.length) summary.push(`${n}/${mems.length} memories`);
  } catch {
    /* empty/invalid */
  }

  // World (content-addressed → idempotent): entities first, then relations.
  try {
    const { entities, relations } = parseWorldJSON(
      JSON.stringify({ entities: snap.world.entities, edges: snap.world.relations }),
    );
    let ne = 0;
    for (const e of entities) {
      try {
        await postJSON("/api/world/add", e);
        ne++;
      } catch {
        /* skip */
      }
    }
    let nr = 0;
    for (const r of relations) {
      try {
        await postAction("/api/world/relate", r as Record<string, string>);
        nr++;
      } catch {
        /* skip */
      }
    }
    if (entities.length) summary.push(`${ne}/${entities.length} entities${relations.length ? ` + ${nr} relations` : ""}`);
  } catch {
    /* empty/invalid */
  }

  return summary;
}
