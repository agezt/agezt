// agentrepair (M960) — the pure core of the Self-Repair / Iterate flow. The
// Repair tab runs the agent AS ITSELF (/api/run with `agent: slug`) on a brief
// built from its own recent failures, lets it fix its own skills/scripts/workdir
// files with its tools, and then auto-applies any identity changes (soul / model
// / fallbacks) it proposes. Both halves — composing the brief and extracting the
// proposal from the run's final answer — are pure so they're unit-testable.

// Minimal shapes (decoupled from the UI row/profile types so this stays pure).
export interface RepairProfile {
  slug: string;
  name?: string;
  soul?: string;
  model?: string;
  fallbacks?: string[];
  task_type?: string;
  workdir?: string;
  memory_scope?: string;
}
export interface RepairFailure {
  correlation_id?: string;
  status?: string;
  started_unix_ms?: number;
}
export interface RepairDenial {
  capability?: string;
  tool?: string;
  reason?: string;
  hard_denied?: boolean;
}
export interface RepairToolError {
  tool?: string;
  output?: string;
}
export interface RepairContext {
  profile: RepairProfile;
  fail?: RepairFailure;
  denials?: RepairDenial[];
  toolErrors?: RepairToolError[];
  runs?: number;
  // Final-answer text of each prior repair round this session, oldest first.
  // Lets "Iterate ×N" show the agent what it already tried so each round builds
  // on the last instead of repeating it.
  priorRounds?: string[];
}

// The proposal an agent may return to change its own identity. Every field is
// optional; an empty object means "no identity change, the file fixes stand".
export interface RepairProposal {
  soul?: string;
  model?: string;
  fallbacks?: string[];
}

const MAX_LINE = 240; // clip any single failure/error line so the brief stays lean

function clipLine(s: string, max = MAX_LINE): string {
  const one = s.replace(/\s+/g, " ").trim();
  return one.length > max ? one.slice(0, max - 1) + "…" : one;
}

// buildRepairBrief composes the self-repair instruction the agent receives. It
// states who the agent is, hands it the concrete evidence of what has been going
// wrong, tells it to fix its own files with its tools, and asks it to close with
// a single fenced JSON block carrying any identity change it wants applied.
export function buildRepairBrief(ctx: RepairContext): string {
  const p = ctx.profile;
  const out: string[] = [];
  out.push(
    `You are the agent "${p.slug}"${p.name && p.name !== p.slug ? ` (${p.name})` : ""}. ` +
      `This is a self-repair run: diagnose why your recent work has been failing and fix yourself.`,
  );

  const id: string[] = [];
  if (p.model) id.push(`model=${p.model}`);
  if (p.fallbacks?.length) id.push(`fallbacks=${p.fallbacks.join(" → ")}`);
  if (p.task_type) id.push(`task_type=${p.task_type}`);
  if (p.workdir) id.push(`workdir=${p.workdir}`);
  if (p.memory_scope) id.push(`memory_scope=${p.memory_scope}`);
  if (id.length) out.push(`Your current configuration: ${id.join(", ")}.`);

  out.push("");
  out.push("## Evidence");
  if (ctx.fail) {
    out.push(
      `- Most recent failed run: ${ctx.fail.correlation_id || "(run)"}` +
        (ctx.fail.status ? ` — status "${clipLine(ctx.fail.status, 120)}"` : ""),
    );
  }
  const denials = (ctx.denials || []).slice(0, 12);
  if (denials.length) {
    out.push(`- Capability denials (${denials.length}):`);
    for (const d of denials) {
      out.push(
        `  • ${d.capability || "?"}${d.tool ? ` via ${d.tool}` : ""}${d.hard_denied ? " [hard-denied]" : ""}` +
          (d.reason ? ` — ${clipLine(d.reason, 160)}` : ""),
      );
    }
  }
  const errs = (ctx.toolErrors || []).slice(0, 12);
  if (errs.length) {
    out.push(`- Tool errors (${errs.length}):`);
    for (const e of errs) out.push(`  • ${e.tool || "?"}: ${clipLine(e.output || "error", 200)}`);
  }
  if (!ctx.fail && !denials.length && !errs.length) {
    out.push("- No failures, denials, or tool errors are recorded — look for latent weaknesses instead (unclear soul, missing skills, brittle scripts).");
  }

  if (ctx.priorRounds?.length) {
    out.push("");
    out.push("## What you already tried this session");
    ctx.priorRounds.forEach((r, i) => out.push(`Round ${i + 1}: ${clipLine(r, 400)}`));
    out.push("Do not repeat those steps — go further or take a different angle.");
  }

  out.push("");
  out.push("## Your task");
  out.push(
    "1. Diagnose the root cause from the evidence above.\n" +
      "2. FIX IT YOURSELF using your tools: edit your own skill/script/workdir files (file, code_exec), add or repair a skill, or correct a broken command. Make the change, don't just describe it.\n" +
      "3. If your IDENTITY is the problem (an unclear or wrong soul/system-prompt, a bad primary model, or a weak fallback chain), end your answer with EXACTLY ONE fenced code block tagged json containing only the fields you want changed:\n" +
      "```json\n" +
      '{ "soul": "<full revised system prompt>", "model": "<primary model>", "fallbacks": ["<m1>", "<m2>"] }\n' +
      "```\n" +
      "Include only the keys you are actually changing. Omit the block entirely if no identity change is needed. The block is applied to your profile automatically.",
  );
  return out.join("\n");
}

// parseRepairProposal pulls the identity-change object out of a run's final
// answer. It prefers the LAST ```json fenced block (the agent's closing
// proposal), then falls back to the last bare {...} object. Returns null when
// there's no parseable proposal or it carries none of the three known keys.
export function parseRepairProposal(finalText: string): RepairProposal | null {
  if (!finalText) return null;
  const candidates: string[] = [];

  // Fenced ```json ... ``` blocks (also accept a bare ``` fence).
  const fence = /```(?:json)?\s*([\s\S]*?)```/gi;
  let m: RegExpExecArray | null;
  while ((m = fence.exec(finalText)) !== null) candidates.push(m[1]);

  // As a fallback, the last balanced-looking {...} run in the text.
  if (candidates.length === 0) {
    const last = finalText.lastIndexOf("{");
    const end = finalText.lastIndexOf("}");
    if (last >= 0 && end > last) candidates.push(finalText.slice(last, end + 1));
  }

  for (let i = candidates.length - 1; i >= 0; i--) {
    const raw = candidates[i].trim();
    let obj: unknown;
    try {
      obj = JSON.parse(raw);
    } catch {
      continue;
    }
    if (!obj || typeof obj !== "object") continue;
    const o = obj as Record<string, unknown>;
    const prop: RepairProposal = {};
    if (typeof o.soul === "string" && o.soul.trim()) prop.soul = o.soul;
    if (typeof o.model === "string" && o.model.trim()) prop.model = o.model.trim();
    if (Array.isArray(o.fallbacks)) {
      const fb = o.fallbacks.filter((x): x is string => typeof x === "string" && x.trim() !== "").map((x) => x.trim());
      if (fb.length) prop.fallbacks = fb;
    }
    if (prop.soul !== undefined || prop.model !== undefined || prop.fallbacks !== undefined) return prop;
  }
  return null;
}

// applyProposal merges a proposal onto a profile, returning the new profile to
// POST to /api/agents/edit. Pure so the auto-apply + Undo (apply the captured
// previous values) share one code path.
export function applyProposal<T extends RepairProfile>(profile: T, prop: RepairProposal): T {
  const next: T = { ...profile };
  if (prop.soul !== undefined) next.soul = prop.soul;
  if (prop.model !== undefined) next.model = prop.model;
  if (prop.fallbacks !== undefined) next.fallbacks = prop.fallbacks;
  return next;
}

// proposalSummary renders a short human label of what a proposal changes, for
// the applied-change banner and Undo affordance.
export function proposalSummary(prop: RepairProposal): string {
  const parts: string[] = [];
  if (prop.soul !== undefined) parts.push("soul");
  if (prop.model !== undefined) parts.push(`model → ${prop.model}`);
  if (prop.fallbacks !== undefined) parts.push(`fallbacks → ${prop.fallbacks.join(" → ") || "(none)"}`);
  return parts.length ? parts.join(", ") : "no identity change";
}
