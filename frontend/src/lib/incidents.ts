import type { AgentEvent } from "@/lib/events";
import type { AutonomyItem } from "@/lib/autonomy";

export interface IncidentMeta {
  incidentId?: string;
  rootIncidentId?: string;
  parentIncidentId?: string;
}

export interface IncidentProfileLite {
  slug: string;
  name?: string;
  owner_agent?: string;
  parent_agent?: string;
  direct_callable?: boolean;
  enabled?: boolean;
  retired?: boolean;
  self_repair?: { enabled?: boolean };
  status?: {
    health_state?: string;
    repair_inflight?: number;
    routing_fallback_count?: number;
    escalation_open_count?: number;
    escalation_acked_count?: number;
  };
}

export interface IncidentActionContext {
  rootSlug?: string;
  rootProfile?: IncidentProfileLite;
  ownerSlug?: string;
  ownerProfile?: IncidentProfileLite;
  latest?: AutonomyItem;
  configIssues: string[];
  humanRequired: boolean;
  policyKind?: "routing_force_exhausted";
  policyLabel?: string;
  policySummary?: string;
  allowedResolutions: string[];
  routingTaskType?: string;
  routingTaskModelChain: string[];
  routingForceGeneration?: number;
  suppressDoctorRerun: boolean;
  preferOwnerWake: boolean;
}

export interface IncidentResolutionPreset {
  key: string;
  label: string;
  text: string;
}

export interface IncidentResolutionRow {
  id: string;
  tsMs?: number;
  subject: string;
  phase?: string;
  mode?: string;
  resolution?: string;
  resolutionSummary?: string;
  delegateTo?: string;
  routingTaskType?: string;
  routingTaskModelChain?: string[];
  routingForceGeneration?: number;
  payload: any;
}

export interface IncidentForceDraft {
  taskType: string;
  chainText: string;
  summary: string;
}

export interface IncidentDelegateDraft {
  delegateTo: string;
  summary: string;
}

export interface IncidentDelegateValidation {
  normalizedTarget: string;
  valid: boolean;
  reason?: string;
}

export interface IncidentDelegateCandidate {
  slug: string;
  name?: string;
  valid: boolean;
  reason?: string;
  enabled?: boolean;
  retired?: boolean;
  directCallable?: boolean;
  ownerAgent?: string;
  parentAgent?: string;
  preferred?: boolean;
  healthState?: string;
  escalationOpenCount?: number;
  escalationAckedCount?: number;
  routingFallbackCount?: number;
  repairInflight?: number;
}

export interface IncidentForceChainPreview {
  taskType: string;
  currentChain: string[];
  proposedChain: string[];
  valid: boolean;
  sameAsCurrent: boolean;
  reason?: string;
  added: string[];
  removed: string[];
}

export interface IncidentForcePreset {
  key: string;
  taskType: string;
  chainText: string;
  summary: string;
  generation?: number;
}

export function incidentMetaFromPayload(payload: any): IncidentMeta {
  return {
    incidentId: str(payload?.incident_id),
    rootIncidentId: str(payload?.root_incident_id),
    parentIncidentId: str(payload?.parent_incident_id),
  };
}

export function incidentMetaFromEvent(e: AgentEvent): IncidentMeta {
  return incidentMetaFromPayload(e.payload);
}

export function incidentMetaFromAutonomy(item: AutonomyItem): IncidentMeta {
  return {
    incidentId: str(item.incident_id),
    rootIncidentId: str(item.root_incident_id),
    parentIncidentId: str(item.parent_incident_id),
  };
}

export function incidentRootId(meta: IncidentMeta | null | undefined): string {
  return str(meta?.rootIncidentId) || str(meta?.incidentId);
}

export function incidentRef(meta: IncidentMeta | null | undefined): string {
  return str(meta?.incidentId) || str(meta?.rootIncidentId);
}

export function incidentMatches(
  meta: IncidentMeta | null | undefined,
  incidentId: string,
): boolean {
  const id = str(incidentId);
  if (!id) return false;
  return (
    str(meta?.incidentId) === id ||
    str(meta?.rootIncidentId) === id ||
    str(meta?.parentIncidentId) === id
  );
}

export function incidentActionContext(
  items: AutonomyItem[] | null | undefined,
  profiles: IncidentProfileLite[] | null | undefined,
  events?: AgentEvent[] | null,
): IncidentActionContext {
  const rows = (items || []).filter((row) => row.category === "doctor");
  const rowsByTs = [...rows].sort((a, b) => (b.ts_unix_ms || 0) - (a.ts_unix_ms || 0));
  const latest = rows.reduce<AutonomyItem | undefined>(
    (best, row) =>
      !best || (row.ts_unix_ms || 0) > (best.ts_unix_ms || 0) ? row : best,
    undefined,
  );
  const bySlug = new Map(
    (profiles || [])
      .filter((row): row is IncidentProfileLite => !!row?.slug)
      .map((row) => [row.slug, row]),
  );
  const rootSlug = firstNonBlank(
    latest?.root_agent,
    ...rows.map((row) => row.root_agent),
    latest?.agent,
  );
  const rootProfile = rootSlug ? bySlug.get(rootSlug) : undefined;
  const ownerSlug = firstNonBlank(
    latest?.delegate_to,
    latest?.target_agent,
    latest?.delegated_by,
    rootProfile?.parent_agent,
    rootProfile?.owner_agent,
  );
  const ownerProfile = ownerSlug ? bySlug.get(ownerSlug) : undefined;
  const configIssues = collectIncidentIssues(events, rootSlug);
  const exhausted = rowsByTs.find(
    (row) =>
      String(row.phase || "").trim() === "routing_force_exhausted_detected",
  );
  const policyKind = exhausted ? "routing_force_exhausted" : undefined;
  const routingTaskType = str(exhausted?.routing_task_type);
  const routingTaskModelChain = Array.isArray(exhausted?.routing_task_model_chain)
    ? exhausted?.routing_task_model_chain.filter((v): v is string => !!str(v))
    : [];
  const routingForceGeneration =
    typeof exhausted?.routing_force_generation === "number"
      ? exhausted.routing_force_generation
      : undefined;
  const allowedResolutions = exhausted
    ? ["paused", "retired", "delegated", "force_chain"]
    : [];
  const policySummary = exhausted
    ? [
        "owner-forced chain exhausted",
        routingTaskType ? `task ${routingTaskType}` : "",
        routingTaskModelChain.length > 0
          ? `chain ${routingTaskModelChain.join(" → ")}`
          : "",
        routingForceGeneration && routingForceGeneration > 1
          ? `generation ${routingForceGeneration}`
          : "",
      ]
        .filter(Boolean)
        .join(" · ")
    : undefined;
  return {
    rootSlug,
    rootProfile,
    ownerSlug,
    ownerProfile,
    latest,
    configIssues,
    humanRequired: !!exhausted,
    policyKind,
    policyLabel: exhausted ? "Forced-chain exhaustion" : undefined,
    policySummary,
    allowedResolutions,
    routingTaskType,
    routingTaskModelChain,
    routingForceGeneration,
    suppressDoctorRerun: !!exhausted,
    preferOwnerWake: !!exhausted,
  };
}

export function incidentResolutionPresets(
  ctx: IncidentActionContext,
): IncidentResolutionPreset[] {
  if (ctx.policyKind !== "routing_force_exhausted" || !ctx.rootSlug) return [];
  const target = ctx.ownerSlug || ctx.rootSlug;
  const summary = ctx.policySummary || "owner-forced chain exhausted";
  const task = ctx.routingTaskType ? ` task ${ctx.routingTaskType}` : "";
  const chain =
    ctx.routingTaskModelChain.length > 0
      ? ` chain ${ctx.routingTaskModelChain.join(" → ")}`
      : "";
  return [
    {
      key: "pause",
      label: "Ask pause",
      text: `Resolution request for ${target}: choose paused for ${ctx.rootSlug}. ${summary}. Pause the root agent now if ownership is not ready to apply a stronger routing decision.`,
    },
    {
      key: "retire",
      label: "Ask retire",
      text: `Resolution request for ${target}: choose retired for ${ctx.rootSlug}. ${summary}. Retire the root agent if this role should leave the active roster.`,
    },
    {
      key: "delegate",
      label: "Ask delegate",
      text: `Resolution request for ${target}: choose delegated for ${ctx.rootSlug}. ${summary}. Delegate this exhausted routing incident to the concrete owner who can take responsibility for${task}${chain}.`,
    },
    {
      key: "force_chain",
      label: "Ask new chain",
      text: `Resolution request for ${target}: choose force_chain for ${ctx.rootSlug} only if you have a concrete new${task}${chain ? ` replacing${chain}` : ""}. Do not reuse the exhausted chain.`,
    },
  ];
}

export function incidentResolutionHistory(
  events: AgentEvent[] | null | undefined,
  incidentId: string,
): IncidentResolutionRow[] {
  const out: IncidentResolutionRow[] = [];
  for (const ev of events || []) {
    if (String(ev.subject || "").trim() !== "agent.resolve") continue;
    if (!incidentMatches(incidentMetaFromEvent(ev), incidentId)) continue;
    const payload = ev.payload || {};
    out.push({
      id: String(ev.id || `${ev.kind}-${ev.seq || out.length}`),
      tsMs: ev.ts_unix_ms,
      subject: String(ev.subject || ""),
      phase: str(payload.phase),
      mode: str(payload.mode),
      resolution: str(payload.resolution),
      resolutionSummary: str(payload.resolution_summary),
      delegateTo: str(payload.delegate_to),
      routingTaskType: str(payload.routing_task_type),
      routingTaskModelChain: Array.isArray(payload.routing_task_model_chain)
        ? payload.routing_task_model_chain
            .map((row: unknown) => str(row))
            .filter(Boolean)
        : undefined,
      routingForceGeneration:
        typeof payload.routing_force_generation === "number"
          ? payload.routing_force_generation
          : undefined,
      payload,
    });
  }
  out.sort((a, b) => (b.tsMs || 0) - (a.tsMs || 0));
  return out;
}

export function incidentResolutionForceDraft(
  row: IncidentResolutionRow | null | undefined,
): IncidentForceDraft | null {
  if (!row || row.resolution !== "force_chain") return null;
  const taskType = str(row.routingTaskType);
  const chain = (row.routingTaskModelChain || []).filter(Boolean);
  if (!taskType || chain.length === 0) return null;
  return {
    taskType,
    chainText: chain.join(", "),
    summary:
      row.resolutionSummary ||
      `branch a new ${taskType} chain from prior force_chain resolution`,
  };
}

export function incidentResolutionDelegateDraft(
  row: IncidentResolutionRow | null | undefined,
): IncidentDelegateDraft | null {
  if (!row || row.resolution !== "delegated") return null;
  const delegateTo = str(row.delegateTo);
  if (!delegateTo) return null;
  return {
    delegateTo,
    summary:
      row.resolutionSummary ||
      `delegate this incident to ${delegateTo}`,
  };
}

export function validateIncidentDelegateTarget(
  target: string | null | undefined,
  ctx: {
    rootSlug?: string;
    ownerSlug?: string;
    profiles?: IncidentProfileLite[] | null;
  },
): IncidentDelegateValidation {
  const normalizedTarget = str(target);
  const rootSlug = str(ctx.rootSlug);
  const ownerSlug = str(ctx.ownerSlug);
  if (!normalizedTarget) {
    return {
      normalizedTarget,
      valid: false,
      reason: "delegate target required",
    };
  }
  if (rootSlug && normalizedTarget === rootSlug) {
    return {
      normalizedTarget,
      valid: false,
      reason: "delegate target cannot be the root agent",
    };
  }
  if (ownerSlug && normalizedTarget === ownerSlug) {
    return {
      normalizedTarget,
      valid: false,
      reason: "delegate target already owns this incident",
    };
  }
  const profiles = ctx.profiles || [];
  const found = profiles.find((row) => str(row.slug) === normalizedTarget);
  if (!found) {
    return {
      normalizedTarget,
      valid: false,
      reason: "delegate target does not exist in the roster",
    };
  }
  if (found.retired) {
    return {
      normalizedTarget,
      valid: false,
      reason: "delegate target is retired",
    };
  }
  if (found.direct_callable === false) {
    return {
      normalizedTarget,
      valid: false,
      reason: "delegate target is a managed sub-agent",
    };
  }
  return { normalizedTarget, valid: true };
}

export function incidentDelegateCandidates(
  profiles: IncidentProfileLite[] | null | undefined,
  ctx: { rootSlug?: string; ownerSlug?: string; preferredSlugs?: string[] },
): IncidentDelegateCandidate[] {
  const preferredOrder = new Map<string, number>();
  for (const slug of ctx.preferredSlugs || []) {
    const normalized = str(slug);
    if (!normalized || preferredOrder.has(normalized)) continue;
    preferredOrder.set(normalized, preferredOrder.size);
  }
  const rows = (profiles || [])
    .filter((row): row is IncidentProfileLite => !!str(row?.slug))
    .map((row) => {
      const validation = validateIncidentDelegateTarget(row.slug, {
        ...ctx,
        profiles,
      });
      return {
        slug: row.slug,
        name: str(row.name) || undefined,
        valid: validation.valid,
        reason: validation.reason,
        enabled: row.enabled,
        retired: row.retired,
        directCallable: row.direct_callable,
        ownerAgent: str(row.owner_agent) || undefined,
        parentAgent: str(row.parent_agent) || undefined,
        preferred: preferredOrder.has(str(row.slug)),
        healthState: str(row.status?.health_state) || undefined,
        escalationOpenCount: numberish(row.status?.escalation_open_count),
        escalationAckedCount: numberish(row.status?.escalation_acked_count),
        routingFallbackCount: numberish(row.status?.routing_fallback_count),
        repairInflight: numberish(row.status?.repair_inflight),
      };
    });
  rows.sort((a, b) => {
    if (a.valid !== b.valid) return a.valid ? -1 : 1;
    const aPref = preferredOrder.has(a.slug) ? preferredOrder.get(a.slug)! : Number.MAX_SAFE_INTEGER;
    const bPref = preferredOrder.has(b.slug) ? preferredOrder.get(b.slug)! : Number.MAX_SAFE_INTEGER;
    if (aPref !== bPref) return aPref - bPref;
    if ((a.enabled ?? false) !== (b.enabled ?? false)) return a.enabled ? -1 : 1;
    const aLoad = candidateLoadScore(a);
    const bLoad = candidateLoadScore(b);
    if (aLoad !== bLoad) return aLoad - bLoad;
    return a.slug.localeCompare(b.slug);
  });
  return rows;
}

export function incidentForceChainPresets(
  rows: IncidentResolutionRow[] | null | undefined,
): IncidentForcePreset[] {
  const out: IncidentForcePreset[] = [];
  const seen = new Set<string>();
  for (const row of rows || []) {
    const draft = incidentResolutionForceDraft(row);
    if (!draft) continue;
    const key = `${draft.taskType}::${draft.chainText}`;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push({
      key,
      taskType: draft.taskType,
      chainText: draft.chainText,
      summary: draft.summary,
      generation: row.routingForceGeneration,
    });
  }
  return out;
}

export function previewIncidentForceChain(
  taskType: string | null | undefined,
  currentChain: string[] | null | undefined,
  chainText: string | null | undefined,
): IncidentForceChainPreview {
  const task = str(taskType);
  const current = (currentChain || []).map((row) => str(row)).filter(Boolean);
  const proposed = parseIncidentChainText(chainText);
  if (!task) {
    return {
      taskType: task,
      currentChain: current,
      proposedChain: proposed,
      valid: false,
      sameAsCurrent: false,
      reason: "force_chain task type required",
      added: [],
      removed: [],
    };
  }
  if (proposed.length === 0) {
    return {
      taskType: task,
      currentChain: current,
      proposedChain: proposed,
      valid: false,
      sameAsCurrent: false,
      reason: "force_chain model chain required",
      added: [],
      removed: [],
    };
  }
  const sameAsCurrent = sameChain(current, proposed);
  const currentSet = new Set(current.map((row) => row.toLowerCase()));
  const proposedSet = new Set(proposed.map((row) => row.toLowerCase()));
  const added = proposed.filter((row) => !currentSet.has(row.toLowerCase()));
  const removed = current.filter((row) => !proposedSet.has(row.toLowerCase()));
  return {
    taskType: task,
    currentChain: current,
    proposedChain: proposed,
    valid: !sameAsCurrent,
    sameAsCurrent,
    reason: sameAsCurrent
      ? "proposed chain matches the exhausted chain; branch to a genuinely new chain"
      : undefined,
    added,
    removed,
  };
}

function collectIncidentIssues(
  events: AgentEvent[] | null | undefined,
  rootSlug?: string,
): string[] {
  const slug = str(rootSlug);
  if (!slug) return [];
  const out: string[] = [];
  const seen = new Set<string>();
  for (const ev of events || []) {
    const payload = ev.payload || {};
    const issues = Array.isArray(payload.issues) ? payload.issues : [];
    if (issues.length === 0) continue;
    const agent = firstNonBlank(payload.agent, payload.root_agent, payload.source_agent);
    if (agent !== slug) continue;
    for (const issue of issues) {
      const text = str(issue);
      if (!text || seen.has(text)) continue;
      seen.add(text);
      out.push(text);
    }
  }
  return out;
}

function str(v: unknown): string {
  return v == null ? "" : String(v).trim();
}

function firstNonBlank(...items: Array<unknown>): string {
  for (const item of items) {
    const value = str(item);
    if (value) return value;
  }
  return "";
}

function parseIncidentChainText(text: string | null | undefined): string[] {
  return str(text)
    .split(/[\n,>]+/)
    .map((row) => row.replace(/^[-\s>]+/, "").trim())
    .filter(Boolean);
}

function sameChain(left: string[], right: string[]): boolean {
  if (left.length !== right.length) return false;
  for (let i = 0; i < left.length; i += 1) {
    if (left[i]!.toLowerCase() !== right[i]!.toLowerCase()) return false;
  }
  return true;
}

function numberish(v: unknown): number {
  return typeof v === "number" && Number.isFinite(v) ? v : 0;
}

function candidateLoadScore(row: IncidentDelegateCandidate): number {
  let score = 0;
  score += (row.escalationOpenCount || 0) * 10;
  score += (row.escalationAckedCount || 0) * 3;
  score += (row.routingFallbackCount || 0) * 2;
  score += (row.repairInflight || 0) * 4;
  const health = str(row.healthState);
  if (health && health !== "healthy") score += health === "stale" ? 2 : 8;
  return score;
}
