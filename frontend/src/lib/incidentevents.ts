import type { AgentEvent } from "@/lib/events";
import { clip } from "@/lib/utils";

const INCIDENT_SUBJECTS = new Set([
  "doctor.auto_repair",
  "agent.repair",
  "agent.wake",
  "agent.resolve",
]);

export function isIncidentFamilyEvent(
  event: Pick<AgentEvent, "subject"> | null | undefined,
): boolean {
  return INCIDENT_SUBJECTS.has(String(event?.subject || "").trim());
}

export function incidentBadgeItem(
  event: Pick<AgentEvent, "subject" | "payload"> | null | undefined,
) {
  return {
    subject: event?.subject,
    phase: event?.payload?.phase,
    mode: event?.payload?.mode,
  };
}

export function incidentEventSummary(
  event: Pick<AgentEvent, "subject" | "payload"> | null | undefined,
): string {
  const subject = String(event?.subject || "").trim();
  if (!isIncidentFamilyEvent({ subject })) return subject;
  const payload = event?.payload || {};
  const rollbackRewrite =
    String(payload.phase || "").trim() === "routing_rollback_completed" &&
    String(payload.routing_task_type || "").trim() &&
    Array.isArray(payload.routing_task_model_chain) &&
    payload.routing_task_model_chain.length
      ? `rolled back ${payload.routing_task_type} → ${payload.routing_task_model_chain.join(" → ")}`
      : "";
  const forcedChain =
    String(payload.phase || "").trim() === "resolution_applied" &&
    String(payload.resolution || "").trim() === "force_chain" &&
    String(payload.routing_task_type || "").trim() &&
    Array.isArray(payload.routing_task_model_chain) &&
    payload.routing_task_model_chain.length
      ? `forced ${payload.routing_task_type} → ${payload.routing_task_model_chain.join(" → ")}`
      : "";
  const forceGen =
    Number(payload.routing_force_generation || 0) > 1
      ? `gen ${payload.routing_force_generation}`
      : "";
  const routingRewrite =
    String(payload.routing_task_type || "").trim() &&
    Array.isArray(payload.routing_task_model_chain) &&
    payload.routing_task_model_chain.length
      ? `rewrote ${payload.routing_task_type} → ${payload.routing_task_model_chain.join(" → ")}`
      : "";
  const actor = String(
    payload.agent ||
      payload.root_agent ||
      payload.target_agent ||
      payload.delegate_to ||
      "",
  ).trim();
  const route = [
    payload.delegated_by ? `by ${payload.delegated_by}` : "",
    payload.delegate_to && payload.delegate_to !== actor
      ? `to ${payload.delegate_to}`
      : "",
  ]
    .filter(Boolean)
    .join(" · ");
  const detail = String(
    rollbackRewrite ||
      forcedChain ||
      routingRewrite ||
      payload.reason ||
      payload.error ||
      payload.resolution_summary ||
      "",
  ).trim();
  const bits = [actor, route, detail, forceGen, subject].filter(Boolean);
  return clip(bits.join(" · "), 160);
}
