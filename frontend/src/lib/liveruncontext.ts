import { type AgentEvent } from "@/lib/events";

// LiveRunContext is the reconstructed "what is this run doing right now" view of a
// correlation, folded from the live event stream: which agent owns it, the current
// phase (starting / thinking / using tool / …), the tool in flight, the model, and
// how it was woken. Shared by the Overseer and the Runs monitors so both surface
// the same live picture without duplicating the fold logic.
export interface LiveRunContext {
  agent?: string;
  phase?: string;
  detail?: string;
  tool?: string;
  model?: string;
  wakeSource?: string;
  scheduleId?: string;
  standingName?: string;
  lastEventMs?: number;
}

// buildLiveRunContexts replays the event buffer oldest-first (events arrive
// newest-first), so for each correlation the latest event wins each field and the
// `phase` reflects the most recent activity.
export function buildLiveRunContexts(events: AgentEvent[]): Record<string, LiveRunContext> {
  const out: Record<string, LiveRunContext> = {};
  for (const e of [...events].reverse()) {
    const corr = e.correlation_id || "";
    if (!corr) continue;
    const p = e.payload || {};
    const row = out[corr] || {};
    if (e.actor && !row.agent) row.agent = e.actor;
    row.lastEventMs = Math.max(row.lastEventMs || 0, e.ts_unix_ms || 0) || row.lastEventMs;
    switch (e.kind) {
      case "task.received":
        row.agent = firstText(row.agent, e.actor, p.agent);
        row.phase = firstText(row.phase, "starting");
        row.detail = firstText(row.detail, p.intent);
        row.wakeSource = firstText(row.wakeSource, p.wake_source, p.source);
        row.scheduleId = firstText(row.scheduleId, p.schedule_id);
        row.standingName = firstText(row.standingName, p.standing_name, p.standing_id);
        break;
      case "llm.request":
        row.phase = "thinking";
        row.model = firstText(p.model, row.model);
        break;
      case "tool.invoked":
        row.phase = "using tool";
        row.tool = firstText(p.tool, p.name, row.tool);
        row.detail = firstText(row.tool, row.detail);
        break;
      case "tool.result":
        row.phase = "observing tool";
        row.tool = firstText(p.tool, p.name, row.tool);
        row.detail = firstText(row.tool, row.detail);
        break;
      case "task.continued":
        row.phase = "continuing";
        break;
      case "task.completed":
        row.phase = "completed";
        break;
      case "task.failed":
        row.phase = "failed";
        row.detail = firstText(p.error, row.detail);
        break;
    }
    out[corr] = row;
  }
  return out;
}

export function liveWakeLabel(ctx: LiveRunContext): string {
  const source = ctx.wakeSource || "wake";
  if (ctx.standingName) return `${source} ${ctx.standingName}`;
  if (ctx.scheduleId) return `${source} ${ctx.scheduleId}`;
  return source;
}

function firstText(...items: unknown[]): string | undefined {
  for (const item of items) {
    if (typeof item !== "string") continue;
    const v = item.trim();
    if (v) return v;
  }
  return undefined;
}
