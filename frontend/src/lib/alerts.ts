import type { AgentEvent } from "@/lib/events";

// Alerts: the daemon's PROACTIVE signals — what it flagged on its own, distinct
// from the raw event firehose. Pulse observer deltas (e.g. the self-health
// monitor), the briefings pulse decided to send, run failures, blocked egress,
// budget/rate trips, and halts. Pure + unit-tested; the view owns the rolling
// list and the live subscription.

export type AlertLevel = "critical" | "warning" | "info";

export interface Alert {
  level: AlertLevel;
  title: string;
  detail: string;
  source: string; // short origin label (e.g. "self:health", "pulse", "egress")
}

// LEVEL_ORDER ranks severity for sorting/filtering (higher = more severe).
export const LEVEL_ORDER: Record<AlertLevel, number> = { info: 0, warning: 1, critical: 2 };

function str(v: unknown): string {
  return v == null ? "" : String(v);
}

// classifyAlert maps an event to an Alert, or null when the event is not a
// proactive signal worth surfacing. Kept deliberately narrow: the Alerts view is
// "what should I look at", not the whole stream.
export function classifyAlert(e: AgentEvent): Alert | null {
  const k = (e.kind || "").toLowerCase();
  const p: any = e.payload || {};

  switch (k) {
    case "observer.delta": {
      // A pulse observer detected a meaningful change (self-health, disk, CI…).
      const sev = String(p.hints?.severity || "").toLowerCase();
      const level: AlertLevel = sev === "critical" ? "critical" : sev === "high" ? "warning" : "info";
      return {
        level,
        title: str(p.summary) || str(p.kind) || "observed change",
        detail: "",
        source: str(p.source) || "pulse",
      };
    }
    case "briefing.sent": {
      // Pulse decided this was worth telling the operator.
      const disp = String(p.disposition || "").toLowerCase();
      const level: AlertLevel = disp === "alert" ? "warning" : "info";
      return { level, title: str(p.title) || "briefing", detail: str(p.body), source: "pulse" };
    }
    case "task.failed":
      return { level: "warning", title: "run failed", detail: str(p.reason) || str(p.error), source: "run" };
    case "netguard.blocked":
      return {
        level: "warning",
        title: "egress blocked",
        detail: [str(p.ip), str(p.reason)].filter(Boolean).join(" — "),
        source: str(p.tool) || "egress",
      };
    case "budget.exceeded":
      return { level: "critical", title: "budget ceiling exceeded", detail: "", source: "budget" };
    case "rate.limited":
      return { level: "warning", title: "provider rate-limited", detail: str(p.provider), source: "provider" };
    case "halt":
      return { level: "critical", title: "daemon halted", detail: str(p.reason), source: "kernel" };
    case "capability.rejected":
      return { level: "info", title: "capability rejected", detail: str(p.capability), source: "policy" };
    default:
      return null;
  }
}

// isAlert is the boolean form used to filter a stream.
export function isAlert(e: AgentEvent): boolean {
  return classifyAlert(e) !== null;
}

export interface RankedAlert extends Alert {
  id: string;
  tsMs?: number;
  correlationId?: string; // the run this alert belongs to, when there is one (M781)
}

// DEFAULT_ATTENTION_WINDOW_MS bounds how long an alert counts as "needs
// attention" (M913). The event buffer backfills history from the journal, so an
// old halt or a run that failed days ago would otherwise sit in "Needs
// attention" forever — the operator asked why these never clear. A day is long
// enough to not miss something actionable, short enough that stale signals age
// out on their own.
export const DEFAULT_ATTENTION_WINDOW_MS = 24 * 60 * 60 * 1000;

// daemonHalted reports whether the kernel is CURRENTLY halted, by the most
// recent halt/resume transition in the stream — so a "daemon halted" alert that
// a later resume already cleared no longer demands attention (M913). Pure.
export function daemonHalted(events: AgentEvent[]): boolean {
  let haltMs = -1;
  let resumeMs = -1;
  for (const e of events) {
    const k = (e.kind || "").toLowerCase();
    const ts = e.ts_unix_ms ?? 0;
    if (k === "halt") haltMs = Math.max(haltMs, ts);
    else if (k === "resume") resumeMs = Math.max(resumeMs, ts);
  }
  return haltMs >= 0 && haltMs > resumeMs;
}

// AttentionOpts tunes the "needs attention" filtering (M913). nowMs enables the
// recency window (omit to keep every alert regardless of age); windowMs defaults
// to a day. The 2nd arg may also be a bare number for the legacy `limit`.
export interface AttentionOpts {
  limit?: number;
  nowMs?: number;
  windowMs?: number;
}

// attentionFilter drops an alert that no longer needs attention: a "daemon
// halted" already cleared by a later resume, or one older than the recency
// window. Shared by the cockpit strip and the nav badge so they always agree.
function attentionFilter(e: AgentEvent, halted: boolean, nowMs?: number, windowMs = DEFAULT_ATTENTION_WINDOW_MS): boolean {
  if ((e.kind || "").toLowerCase() === "halt" && !halted) return false; // resolved by a resume
  const ts = e.ts_unix_ms;
  if (nowMs != null && ts != null && nowMs - ts > windowMs) return false; // aged out
  return true;
}

// recentAttentionAlerts classifies a stream and returns the warning/critical alerts only,
// deduped by id and newest-first, capped at `limit` (M780). Used by the cockpit to show
// "what needs attention" inline, reusing the exact rules of the Alerts view. Resolved
// halts and (when nowMs is given) alerts past the recency window are dropped (M913).
export function recentAttentionAlerts(events: AgentEvent[], limitOrOpts: number | AttentionOpts = {}): RankedAlert[] {
  const opts = typeof limitOrOpts === "number" ? { limit: limitOrOpts } : limitOrOpts;
  const { limit = 5, nowMs, windowMs } = opts;
  const halted = daemonHalted(events);
  const seen = new Set<string>();
  const out: RankedAlert[] = [];
  for (const e of events) {
    const a = classifyAlert(e);
    if (!a || a.level === "info") continue;
    if (!attentionFilter(e, halted, nowMs, windowMs)) continue;
    const id = e.id || `${e.kind}-${e.seq ?? ""}`;
    if (seen.has(id)) continue;
    seen.add(id);
    out.push({ ...a, id, tsMs: e.ts_unix_ms, correlationId: e.correlation_id });
  }
  out.sort((x, y) => (y.tsMs ?? 0) - (x.tsMs ?? 0));
  return out.slice(0, limit);
}

// attentionAlertCount counts the events that classify as a warning- or critical-level
// alert — the ones worth a badge (M779). Info-level signals (e.g. a rejected capability)
// are real alerts but don't demand attention, so they're excluded from the count. Applies
// the same resolved-halt + recency filtering as the cockpit strip (M913) so the badge and
// the strip never disagree. Pass nowMs to enable the recency window.
export function attentionAlertCount(events: AgentEvent[], opts: AttentionOpts = {}): number {
  const { nowMs, windowMs } = opts;
  const halted = daemonHalted(events);
  const seen = new Set<string>();
  let n = 0;
  for (const e of events) {
    const a = classifyAlert(e);
    if (!a || a.level === "info") continue;
    if (!attentionFilter(e, halted, nowMs, windowMs)) continue;
    const id = e.id || `${e.kind}-${e.seq ?? ""}`;
    if (seen.has(id)) continue; // dedupe so the badge matches the strip's count
    seen.add(id);
    n++;
  }
  return n;
}
