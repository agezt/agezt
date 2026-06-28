import type { AgentEvent } from "@/lib/events";
import type { AgentRuntimeStatus } from "@/lib/agentdetail";

interface AgentLivePatch {
  enabled?: boolean;
  retired?: boolean;
  removed?: boolean;
  status?: AgentRuntimeStatus;
}

export type AgentLivePatchMap = Record<string, AgentLivePatch>;

export interface AgentLiveTarget {
  slug: string;
  enabled?: boolean;
  retired?: boolean;
  status?: AgentRuntimeStatus;
}

const DEFAULT_REPAIR_COOLDOWN_MS = 30 * 60 * 1000;

export function shouldReloadAgentCatalog(ev: AgentEvent): boolean {
  return (
    ev.subject === "doctor.auto_repair" ||
    ev.kind === "roster.created" ||
    ev.kind === "roster.updated" ||
    ev.kind === "roster.removed" ||
    ev.kind === "task.received" ||
    ev.kind === "task.completed" ||
    ev.kind === "task.failed"
  );
}

export function reduceAgentLivePatchMap(prev: AgentLivePatchMap, ev: AgentEvent): AgentLivePatchMap {
  if (ev.subject === "doctor.auto_repair") {
    const slug = stringField(ev.payload, "agent") || stringField(ev.payload, "target_agent");
    if (!slug) return prev;
    const current = prev[slug] || {};
    const status = { ...(current.status || {}) };
    const phase = stringField(ev.payload, "phase");
    const mode = stringField(ev.payload, "mode");
    if (mode) status.repair_mode = mode;
    status.repair_state = phase || status.repair_state;
    status.repair_label =
      phase === "completed" ? (mode === "degraded" ? "doctor repaired" : "repaired") :
      phase === "resolution_applied" ? "manager applied" :
      phase === "routing_rollback_completed" ? "rolled back" :
      phase === "failed" ? (mode === "degraded" ? "doctor failed" : "repair failed") :
      phase === "routing_rollback_failed" ? "rollback failed" :
      phase === "queued" ? (mode === "degraded" ? "doctor queued" : "repair queued") :
      phase === "routing_rollback_queued" ? "rollback queued" :
      phase || status.repair_label;
    status.repair_last_ts_ms = ev.ts_unix_ms || status.repair_last_ts_ms;
    status.repair_next_eligible_ms = (ev.ts_unix_ms || 0) > 0 ? (ev.ts_unix_ms || 0) + DEFAULT_REPAIR_COOLDOWN_MS : status.repair_next_eligible_ms;
    if (phase === "queued" || phase === "routing_rollback_queued") {
      status.repair_inflight = Math.max(1, (status.repair_inflight || 0) + 1);
    } else if (phase === "completed" || phase === "failed" || phase === "resolution_applied" || phase === "routing_rollback_completed" || phase === "routing_rollback_failed") {
      status.repair_inflight = Math.max(0, (status.repair_inflight || 0) - 1);
    }
    if (phase === "failed" || phase === "routing_rollback_failed") {
      status.repair_last_error = stringField(ev.payload, "error");
    }
    status.repair_self_attempt = numberField(ev.payload, "self_repair_attempt") ?? status.repair_self_attempt;
    status.repair_self_max_attempts = numberField(ev.payload, "self_repair_max_attempts") ?? status.repair_self_max_attempts;
    status.repair_incident_id = stringField(ev.payload, "incident_id") || status.repair_incident_id;
    status.repair_root_incident_id = stringField(ev.payload, "root_incident_id") || status.repair_root_incident_id;
    status.repair_parent_incident_id = stringField(ev.payload, "parent_incident_id") || status.repair_parent_incident_id;
    status.repair_root_agent = stringField(ev.payload, "root_agent") || status.repair_root_agent;
    status.repair_chain_depth = numberField(ev.payload, "chain_depth") ?? status.repair_chain_depth;
    return { ...prev, [slug]: { ...current, status } };
  }
  if (ev.kind === "roster.updated") {
    const slug = stringField(ev.payload, "slug");
    if (!slug) return prev;
    const current = prev[slug] || {};
    const status = { ...(current.status || {}) };
    if (typeof ev.payload?.enabled === "boolean") current.enabled = ev.payload.enabled;
    if (typeof ev.payload?.retired === "boolean") {
      current.retired = ev.payload.retired;
      status.health_state = ev.payload.retired ? "retired" : status.health_state === "retired" ? "healthy" : status.health_state;
      status.health_label = ev.payload.retired ? "graveyard" : status.health_state === "healthy" ? "healthy" : status.health_label;
    }
    return { ...prev, [slug]: { ...current, status } };
  }
  if (ev.kind === "roster.removed") {
    const slug = stringField(ev.payload, "slug");
    if (!slug) return prev;
    return { ...prev, [slug]: { ...(prev[slug] || {}), removed: true } };
  }
  if (ev.kind === "task.received") {
    const slug = realAgentSlug(stringField(ev.payload, "agent") || stringField(ev.payload, "target_agent") || ev.actor || "");
    if (!slug) return prev;
    const current = prev[slug] || {};
    const status = { ...(current.status || {}) };
    status.active_run_count = Math.max(1, status.active_run_count || 0);
    status.active_correlation_id = ev.correlation_id || status.active_correlation_id;
    status.active_intent = stringField(ev.payload, "intent") || status.active_intent;
    status.active_model = stringField(ev.payload, "model") || status.active_model;
    status.active_started_ms = ev.ts_unix_ms || status.active_started_ms;
    status.active_last_event_ms = ev.ts_unix_ms || status.active_last_event_ms;
    status.active_last_event_kind = ev.kind;
    status.active_phase = "starting";
    status.operational_state = "running";
    status.operational_label = "starting";
    status.active_wake_source = stringField(ev.payload, "wake_source") || stringField(ev.payload, "source") || status.active_wake_source;
    status.active_wake_reason = stringField(ev.payload, "wake_reason") || stringField(ev.payload, "reason") || status.active_wake_reason;
    status.active_schedule_id = stringField(ev.payload, "schedule_id") || status.active_schedule_id;
    status.active_standing_id = stringField(ev.payload, "standing_id") || status.active_standing_id;
    status.active_standing_name = stringField(ev.payload, "standing_name") || status.active_standing_name;
    status.active_trigger_subject = stringField(ev.payload, "trigger_subject") || stringField(ev.payload, "event_subject") || status.active_trigger_subject;
    status.last_activity_ms = ev.ts_unix_ms || status.last_activity_ms;
    status.last_activity_kind = ev.kind;
    status.last_activity_correlation_id = ev.correlation_id || status.last_activity_correlation_id;
    status.last_activity_summary = status.active_intent ? `started ${status.active_intent}` : "run started";
    return { ...prev, [slug]: { ...current, status } };
  }
  if (ev.kind === "llm.request" || ev.kind === "llm.response" || ev.kind === "policy.decision" || ev.kind === "tool.invoked" || ev.kind === "tool.result") {
    const slug = liveEventAgentSlug(prev, ev);
    if (!slug) return prev;
    const current = prev[slug] || {};
    const status = { ...(current.status || {}) };
    const phase = livePhaseLabel(ev);
    status.active_run_count = Math.max(1, status.active_run_count || 0);
    status.active_correlation_id = ev.correlation_id || status.active_correlation_id;
    status.active_phase = phase;
    status.operational_state = "running";
    status.operational_label = phase;
    status.active_last_event_ms = ev.ts_unix_ms || status.active_last_event_ms;
    status.active_last_event_kind = ev.kind;
    status.active_tool = stringField(ev.payload, "tool") || stringField(ev.payload, "name") || stringField(ev.payload, "capability") || status.active_tool;
    status.active_model = stringField(ev.payload, "model") || status.active_model;
    status.active_iter = numberField(ev.payload, "iter") ?? status.active_iter;
    status.active_detail = [status.active_iter ? `iter ${status.active_iter}` : "", status.active_tool || ""].filter(Boolean).join(" · ") || status.active_detail;
    status.last_activity_ms = ev.ts_unix_ms || status.last_activity_ms;
    status.last_activity_kind = ev.kind;
    status.last_activity_correlation_id = ev.correlation_id || status.last_activity_correlation_id;
    status.last_activity_summary = phase;
    return { ...prev, [slug]: { ...current, status } };
  }
  if (ev.kind === "task.completed" || ev.kind === "task.failed") {
    const slug = liveEventAgentSlug(prev, ev);
    if (!slug) return prev;
    const current = prev[slug] || {};
    const status = { ...(current.status || {}) };
    if (ev.correlation_id && status.active_correlation_id && ev.correlation_id !== status.active_correlation_id) {
      return prev;
    }
    status.active_run_count = 0;
    status.active_phase = ev.kind === "task.completed" ? "completed" : "failed";
    status.active_last_event_ms = ev.ts_unix_ms || status.active_last_event_ms;
    status.active_last_event_kind = ev.kind;
    status.operational_state = current.retired ? "retired" : current.enabled === false ? "paused" : "sleeping";
    status.operational_label = status.operational_state;
    status.last_activity_ms = ev.ts_unix_ms || status.last_activity_ms;
    status.last_activity_kind = ev.kind;
    status.last_activity_correlation_id = ev.correlation_id || status.last_activity_correlation_id;
    status.last_activity_summary = ev.kind === "task.completed" ? "run completed" : stringField(ev.payload, "error") || "run failed";
    return { ...prev, [slug]: { ...current, status } };
  }
  return prev;
}

export function applyAgentLivePatches<T extends AgentLiveTarget>(items: T[], patches: AgentLivePatchMap): T[] {
  if (!items.length || Object.keys(patches).length === 0) return items;
  const out: T[] = [];
  for (const item of items) {
    const patch = patches[item.slug];
    if (!patch) {
      out.push(item);
      continue
    }
    if (patch.removed) continue;
    out.push({
      ...item,
      enabled: patch.enabled ?? item.enabled,
      retired: patch.retired ?? item.retired,
      status: patch.status ? { ...(item.status || {}), ...patch.status } : item.status,
    });
  }
  return out;
}

function stringField(obj: any, key: string): string {
  const value = obj?.[key];
  return typeof value === "string" ? value : "";
}

function numberField(obj: any, key: string): number | undefined {
  const value = obj?.[key];
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function realAgentSlug(value: string): string {
  if (!value || value.startsWith("agent-run-") || value.startsWith("run-")) return "";
  return value;
}

function liveEventAgentSlug(prev: AgentLivePatchMap, ev: AgentEvent): string {
  const direct = realAgentSlug(stringField(ev.payload, "agent") || stringField(ev.payload, "target_agent") || ev.actor || "");
  if (direct) return direct;
  if (!ev.correlation_id) return "";
  for (const [slug, patch] of Object.entries(prev)) {
    if (patch.status?.active_correlation_id === ev.correlation_id) return slug;
  }
  return "";
}

function livePhaseLabel(ev: AgentEvent): string {
  switch (ev.kind) {
    case "llm.request":
      return "thinking";
    case "llm.response":
      return stringField(ev.payload, "tool") ? "planned tools" : "answered draft";
    case "policy.decision":
      return ev.payload?.allow === false ? "blocked tool" : "checking policy";
    case "tool.invoked":
      return "using tool";
    case "tool.result":
      return ev.payload?.error ? "tool error" : "observed tool";
    default:
      return ev.kind || "working";
  }
}
