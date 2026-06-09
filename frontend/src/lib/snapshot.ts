import { getJSON } from "@/lib/api";

// Full snapshot (M741): a READ-ONLY record of the daemon's whole customizable state —
// persona, prompts, routing, standing orders, schedules, memory and the world model —
// gathered into one JSON for backup, audit, or planning a migration. Deliberately
// export-only: restoring autonomy/knowledge in bulk is safety-sensitive and stays a
// per-domain action, so this never writes anything. The importable subset (persona +
// prompts + routing) is the separate config bundle (lib/configbackup).

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
    s.config.persona.trim() ? "persona" : "no persona",
    `${s.config.prompts.length} prompts`,
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
  const [persona, prompts, routing, standing, schedules, memory, world] = await Promise.all([
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
    exported_note: "Read-only snapshot for backup/audit. Restore is per-domain; the importable subset is the config bundle.",
    config: { persona: persona.system || "", prompts: prompts.prompts || [], chains: routing.chains || {} },
    standing: standing.orders || [],
    schedules: schedules.schedules || [],
    memory: memory.records || [],
    world: { entities: world.entities || [], relations: world.relations || world.edges || [] },
  };
}
