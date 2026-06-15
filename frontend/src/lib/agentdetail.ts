// agentdetail.ts (M953) — pure helpers for the per-agent Command Center deep
// panel (components/AgentDetail). Kept separate from lib/fleet.ts (the M952
// census adapter, reused as-is) so the deep panel adds no surface to it. All
// functions are pure + unit-tested; the React component owns fetching/rendering.

// MemoryRecord mirrors the wire shape of /api/memory records (recordView in
// kernel/controlplane/memory.go). Only the fields the panel reads are typed.
export interface MemoryRecord {
  id?: string;
  type?: string;
  subject?: string;
  content?: string;
  confidence?: number;
  created_ms?: number;
  last_seen_ms?: number;
  tags?: Record<string, string>;
  added_by?: string;
}

// SkillLite mirrors the /api/skills list fields the panel reads.
export interface SkillLite {
  id?: string;
  name?: string;
  description?: string;
  status?: string;
  agent?: string;
  triggers?: string[];
}

// CorrelatedRow is any diagnostics row that can be attributed to a run/agent
// (policy decisions, tool invocations). Both carry a correlation id and an actor.
export interface CorrelatedRow {
  correlation_id?: string;
  actor?: string;
}

// RunLite is the subset of /api/runs the panel folds for spend/last-active.
export interface RunLite {
  correlation_id?: string;
  agent?: string;
  status?: string;
  spent_mc?: number;
  started_unix_ms?: number;
}

// agentScope resolves the memory scope an agent writes to: its explicit
// memory_scope, or — when blank — its slug (the kernel default, M915).
export function agentScope(slug: string, memoryScope?: string): string {
  const s = (memoryScope || "").trim();
  return s !== "" ? s : slug;
}

// agentCorrelations is the set of run correlation ids belonging to an agent
// (runs started AS this agent). Used to attribute journal-derived diagnostics
// (policy denials, tool errors) back to the agent.
export function agentCorrelations(runs: RunLite[], slug: string): Set<string> {
  const out = new Set<string>();
  for (const r of runs) {
    if (r.agent === slug && r.correlation_id) out.add(r.correlation_id);
  }
  return out;
}

// filterByCorrelation keeps diagnostics rows attributable to the agent: either
// the row's correlation belongs to one of the agent's runs, or the row's actor
// is the agent slug directly. Pure; preserves input order.
export function filterByCorrelation<T extends CorrelatedRow>(
  rows: T[],
  corrs: Set<string>,
  slug: string,
): T[] {
  return rows.filter(
    (r) => (r.correlation_id != null && corrs.has(r.correlation_id)) || r.actor === slug,
  );
}

// filterAgentMemory keeps records private to this agent: tagged with the agent's
// scope, or authored by the agent. Shared records (empty/absent scope) are
// excluded — they show in the global Memory view, not the agent's private one.
export function filterAgentMemory(records: MemoryRecord[], slug: string, memoryScope?: string): MemoryRecord[] {
  const scope = agentScope(slug, memoryScope);
  return records.filter((r) => (r.tags?.scope || "") === scope || r.added_by === slug);
}

// filterAgentSkills keeps skills owned by (private to) the agent (Skill.Agent
// == slug), mirroring the Roster per-agent skill count (M943).
export function filterAgentSkills(skills: SkillLite[], slug: string): SkillLite[] {
  return skills.filter((s) => s.agent === slug);
}

export interface AgentSummary {
  runs: number;
  totalSpentMc: number;
  lastStartedMs?: number;
}

// summarizeAgent folds an agent's own runs into the headline counters the
// Overview tab shows (run count, total spend, most-recent start).
export function summarizeAgent(runs: RunLite[], slug: string): AgentSummary {
  let count = 0;
  let totalSpentMc = 0;
  let lastStartedMs: number | undefined;
  for (const r of runs) {
    if (r.agent !== slug) continue;
    count++;
    totalSpentMc += r.spent_mc || 0;
    if (r.started_unix_ms != null && (lastStartedMs == null || r.started_unix_ms > lastStartedMs)) {
      lastStartedMs = r.started_unix_ms;
    }
  }
  return { runs: count, totalSpentMc, lastStartedMs };
}

// lastFailure returns the most recent failed run for the agent (the "ne bok
// yedi" headline), or undefined when the agent has no failures.
export function lastFailure(runs: RunLite[], slug: string): RunLite | undefined {
  let worst: RunLite | undefined;
  for (const r of runs) {
    if (r.agent !== slug) continue;
    if ((r.status || "").toLowerCase() !== "failed") continue;
    if (!worst || (r.started_unix_ms || 0) > (worst.started_unix_ms || 0)) worst = r;
  }
  return worst;
}
