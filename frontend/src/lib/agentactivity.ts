import type { AgentEvent } from "@/lib/events";

export interface AgentRunRef {
  correlation_id?: string;
  status?: string;
}

export interface AgentActivityPulse {
  liveRuns: number;
  doctorEvents: number;
  incidentEvents: number;
  delegations: number;
  mailboxEvents: number;
  value: string;
  detail: string;
  tone: "good" | "warn" | "muted";
}

export interface AgentActivityOperationalState {
  value: string;
  detail: string;
  tone: "good" | "warn" | "accent" | "muted";
}

export function agentRunCorrelations(runs: AgentRunRef[] | null | undefined): Set<string> {
  const out = new Set<string>();
  for (const row of runs || []) {
    const corr = String(row?.correlation_id || "").trim();
    if (corr) out.add(corr);
  }
  return out;
}

export function agentActivityEventMatches(ev: AgentEvent, slug: string, runCorrs: Set<string>): boolean {
  const s = slug.trim().toLowerCase();
  if (String(ev.actor || "").trim().toLowerCase() === s) return true;
  if (ev.correlation_id && runCorrs.has(String(ev.correlation_id))) return true;
  const payload = ev.payload || {};
  const payloadAgent = String(payload.agent || "").trim().toLowerCase();
  const payloadTarget = String(payload.target_agent || "").trim().toLowerCase();
  if (ev.subject === "doctor.auto_repair" && (payloadAgent === s || payloadTarget === s)) return true;
  if (ev.subject === "agent.repair" && payloadAgent === s) return true;
  if (ev.kind === "agent.retry" && payloadAgent === s) return true;
  if (ev.kind === "board.posted") {
    const from = String(payload.from || "").trim().toLowerCase();
    const to = String(payload.to || "").trim().toLowerCase();
    if (from === s || to === s) return true;
    if (payload.to === "*" && from !== s) return true;
    if (Array.isArray(payload.acked_by) && payload.acked_by.some((x: unknown) => String(x).trim().toLowerCase() === s)) return true;
  }
  if (ev.kind === "roster.updated" && String(payload.slug || "").trim().toLowerCase() === s) return true;
  return false;
}

export function filterAgentLogEvents(
  events: AgentEvent[],
  slug: string,
  runCorrs: Set<string>,
  limit = 60,
): AgentEvent[] {
  return events.filter((ev) => agentActivityEventMatches(ev, slug, runCorrs)).slice(0, limit);
}

export function agentActivityPulse(
  runs: AgentRunRef[] | null | undefined,
  logs: AgentEvent[] | null | undefined,
): AgentActivityPulse {
  const liveRuns = (runs || []).filter((r) => {
    const status = String(r.status || "").trim().toLowerCase();
    return status === "running" || status === "queued" || status === "retrying";
  }).length;
  let doctorEvents = 0;
  let incidentEvents = 0;
  let delegations = 0;
  let mailboxEvents = 0;
  for (const ev of logs || []) {
    const kind = String(ev.kind || "").trim().toLowerCase();
    const subject = String(ev.subject || "").trim().toLowerCase();
    if (subject === "doctor.auto_repair" || kind === "agent.repair" || kind.includes("repair")) doctorEvents++;
    if (subject === "doctor.auto_repair" || subject === "agent.wake" || kind === "agent.retry") incidentEvents++;
    if (kind === "subagent.spawned" || kind === "subagent.completed") delegations++;
    if (kind === "board.posted") mailboxEvents++;
  }
  const value = liveRuns > 0
    ? `${liveRuns} live ${liveRuns === 1 ? "run" : "runs"}`
    : doctorEvents > 0
      ? `${doctorEvents} doctor signal${doctorEvents === 1 ? "" : "s"}`
      : delegations > 0
        ? `${delegations} delegation${delegations === 1 ? "" : "s"}`
        : mailboxEvents > 0
          ? `${mailboxEvents} mailbox event${mailboxEvents === 1 ? "" : "s"}`
          : "quiet";
  const detail = [
    liveRuns > 0 ? `${liveRuns} active run${liveRuns === 1 ? "" : "s"}` : "",
    doctorEvents > 0 ? `${doctorEvents} doctor/repair` : "",
    incidentEvents > 0 ? `${incidentEvents} incident-linked` : "",
    delegations > 0 ? `${delegations} delegation` : "",
    mailboxEvents > 0 ? `${mailboxEvents} mailbox` : "",
  ].filter(Boolean).join(" · ") || "no live run, doctor, delegation, or mailbox activity in the current event tail";
  return {
    liveRuns,
    doctorEvents,
    incidentEvents,
    delegations,
    mailboxEvents,
    value,
    detail,
    tone: liveRuns > 0 ? "good" : doctorEvents > 0 || incidentEvents > 0 ? "warn" : "muted",
  };
}

export function agentActivityOperationalState(pulse: AgentActivityPulse): AgentActivityOperationalState {
  if (pulse.liveRuns > 0) {
    return {
      value: "awake",
      detail: pulse.detail,
      tone: "accent",
    };
  }
  if (pulse.doctorEvents > 0 || pulse.incidentEvents > 0) {
    return {
      value: "self-repair",
      detail: pulse.detail,
      tone: "warn",
    };
  }
  if (pulse.delegations > 0) {
    return {
      value: "delegating",
      detail: pulse.detail,
      tone: "good",
    };
  }
  if (pulse.mailboxEvents > 0) {
    return {
      value: "mailbox active",
      detail: pulse.detail,
      tone: "good",
    };
  }
  return {
    value: "sleeping",
    detail: "no active run, repair, delegation, or mailbox event in the current live tail",
    tone: "muted",
  };
}
