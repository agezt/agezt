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
  task_model_chain?: string[];
  workdir?: string;
  memory_scope?: string;
  owner_agent?: string;
  parent_agent?: string;
  direct_callable?: boolean;
  retry_policy?: { max_attempts?: number; backoff?: string; base_delay_sec?: number; max_delay_sec?: number; retry_on?: string[] };
  health_policy?: { stale_after_sec?: number; failure_window?: number; failure_threshold?: number; doctor_agent?: string };
  self_repair?: { enabled?: boolean; max_attempts?: number; escalate_to?: string };
  noise_policy?: {
    silent_on_success?: boolean;
    disable_memory_writes?: boolean;
    min_notify_severity?: string;
    min_notify_interval_sec?: number;
  };
}
interface RepairFailure {
  correlation_id?: string;
  status?: string;
  started_unix_ms?: number;
}
interface RepairDenial {
  capability?: string;
  tool?: string;
  reason?: string;
  hard_denied?: boolean;
}
interface RepairToolError {
  tool?: string;
  output?: string;
}
export interface RepairContext {
  profile: RepairProfile;
  fail?: RepairFailure;
  denials?: RepairDenial[];
  toolErrors?: RepairToolError[];
  configIssues?: string[];
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
  task_type?: string;
  task_model_chain?: string[];
  config_overrides?: Record<string, string>;
}

const MAX_LINE = 240; // clip any single failure/error line so the brief stays lean

function clipLine(s: string, max = MAX_LINE): string {
  const one = s.replace(/\s+/g, " ").trim();
  return one.length > max ? one.slice(0, max - 1) + "…" : one;
}

function repairRetrySummary(p: RepairProfile): string {
  const policy = p.retry_policy;
  if (!policy?.max_attempts) return "";
  const bits = [`retry=${policy.max_attempts}x`];
  if (policy.backoff) bits.push(policy.backoff);
  if (policy.retry_on?.length) bits.push(`on ${policy.retry_on.join("/")}`);
  if (policy.base_delay_sec || policy.max_delay_sec) {
    bits.push(`delay ${policy.base_delay_sec || 0}-${policy.max_delay_sec || "?"}s`);
  }
  return bits.join(" ");
}

function repairHealthSummary(p: RepairProfile): string {
  const policy = p.health_policy;
  if (!policy) return "";
  const bits: string[] = [];
  if (policy.doctor_agent) bits.push(`doctor=${policy.doctor_agent}`);
  if (policy.failure_threshold) bits.push(`failure_threshold=${policy.failure_threshold}`);
  if (policy.failure_window) bits.push(`failure_window=${policy.failure_window}`);
  if (policy.stale_after_sec) bits.push(`stale_after=${policy.stale_after_sec}s`);
  return bits.join(" ");
}

function repairSelfSummary(p: RepairProfile): string {
  const policy = p.self_repair;
  if (!policy) return "";
  if (!policy.enabled) return "self_repair=off";
  const bits = [`self_repair=on${policy.max_attempts ? ` ${policy.max_attempts}x` : ""}`];
  if (policy.escalate_to) bits.push(`escalate=${policy.escalate_to}`);
  return bits.join(" ");
}

function repairCallPolicySummary(p: RepairProfile): string {
  const leader = p.parent_agent || p.owner_agent || "";
  if (p.direct_callable === false) return leader ? `call=managed by ${leader}` : "call=managed";
  if (leader) return `call=direct owner=${leader}`;
  return "";
}

function repairNoiseSummary(p: RepairProfile): string {
  const policy = p.noise_policy;
  if (!policy) return "";
  const bits: string[] = [];
  if (policy.silent_on_success) bits.push("silent_on_success");
  if (policy.disable_memory_writes) bits.push("memory_writes=off");
  if (policy.min_notify_severity) bits.push(`notify>=${policy.min_notify_severity}`);
  if (policy.min_notify_interval_sec) bits.push(`notify_cooldown=${policy.min_notify_interval_sec}s`);
  return bits.length ? `noise=${bits.join("/")}` : "";
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
  if (p.task_model_chain?.length) id.push(`task_model_chain=${p.task_model_chain.join(" → ")}`);
  if (p.workdir) id.push(`workdir=${p.workdir}`);
  if (p.memory_scope) id.push(`memory_scope=${p.memory_scope}`);
  if (id.length) out.push(`Your current configuration: ${id.join(", ")}.`);

  const governance = [
    repairRetrySummary(p),
    repairHealthSummary(p),
    repairSelfSummary(p),
    repairCallPolicySummary(p),
    repairNoiseSummary(p),
  ].filter(Boolean);
  if (governance.length) out.push(`Your resilience and governance contract: ${governance.join(", ")}.`);

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
  const cfg = (ctx.configIssues || []).slice(0, 12);
  if (cfg.length) {
    out.push(`- Invalid runtime overrides (${cfg.length}):`);
    for (const issue of cfg) out.push(`  • ${clipLine(issue, 200)}`);
  }
  if (!ctx.fail && !denials.length && !errs.length && !cfg.length) {
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
      "3. If your durable profile or routing is the problem (an unclear or wrong soul/system-prompt, a bad primary model, a weak fallback chain, the wrong task_type or task_model_chain, or broken config_overrides), end your answer with EXACTLY ONE fenced code block tagged json containing only the fields you want changed:\n" +
      "```json\n" +
      '{ "soul": "<full revised system prompt>", "model": "<primary model>", "fallbacks": ["<m1>", "<m2>"], "task_type": "<task type>", "task_model_chain": ["<m1>", "<m2>"], "config_overrides": { "AGEZT_MAX_ITER": "6" } }\n' +
      "```\n" +
      "Include only the keys you are actually changing. Use task_model_chain only when you are intentionally changing the durable task routing for this agent's task type. Omit the block entirely if no durable change is needed. The block is applied automatically.",
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
    if (typeof o.task_type === "string" && o.task_type.trim()) prop.task_type = o.task_type.trim();
    if (Array.isArray(o.task_model_chain)) {
      const chain = o.task_model_chain.filter((x): x is string => typeof x === "string" && x.trim() !== "").map((x) => x.trim());
      if (chain.length) prop.task_model_chain = chain;
    }
    if (o.config_overrides && typeof o.config_overrides === "object" && !Array.isArray(o.config_overrides)) {
      const cfg: Record<string, string> = {};
      for (const [k, v] of Object.entries(o.config_overrides as Record<string, unknown>)) {
        if (typeof v !== "string") continue;
        const key = k.trim().toUpperCase();
        if (!key) continue;
        cfg[key] = v.trim();
      }
      if (Object.keys(cfg).length > 0) prop.config_overrides = cfg;
    }
    if (
      prop.soul !== undefined ||
      prop.model !== undefined ||
      prop.fallbacks !== undefined ||
      prop.task_type !== undefined ||
      prop.task_model_chain !== undefined ||
      prop.config_overrides !== undefined
    ) {
      return prop;
    }
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
  if (prop.task_type !== undefined) next.task_type = prop.task_type;
  if (prop.config_overrides !== undefined) (next as T & { config_overrides?: Record<string, string> }).config_overrides = prop.config_overrides;
  return next;
}

// proposalSummary renders a short human label of what a proposal changes, for
// the applied-change banner and Undo affordance.
export function proposalSummary(prop: RepairProposal): string {
  const parts: string[] = [];
  if (prop.soul !== undefined) parts.push("soul");
  if (prop.model !== undefined) parts.push(`model → ${prop.model}`);
  if (prop.fallbacks !== undefined) parts.push(`fallbacks → ${prop.fallbacks.join(" → ") || "(none)"}`);
  if (prop.task_type !== undefined) parts.push(`task_type → ${prop.task_type}`);
  if (prop.task_model_chain !== undefined) parts.push(`task_model_chain → ${prop.task_model_chain.join(" → ") || "(none)"}`);
  if (prop.config_overrides !== undefined) parts.push(`config_overrides → ${Object.keys(prop.config_overrides).join(", ") || "(none)"}`);
  return parts.length ? parts.join(", ") : "no identity change";
}
