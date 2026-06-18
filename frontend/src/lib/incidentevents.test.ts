import { describe, expect, it } from "vitest";
import {
  incidentBadgeItem,
  incidentEventSummary,
  isIncidentFamilyEvent,
} from "@/lib/incidentevents";

describe("incidentevents", () => {
  it("recognizes doctor/operator incident-family subjects", () => {
    expect(isIncidentFamilyEvent({ subject: "doctor.auto_repair" })).toBe(true);
    expect(isIncidentFamilyEvent({ subject: "agent.wake" })).toBe(true);
    expect(isIncidentFamilyEvent({ subject: "agent.resolve" })).toBe(true);
    expect(isIncidentFamilyEvent({ subject: "task.failed" })).toBe(false);
  });

  it("extracts badge item fields from event payload", () => {
    expect(
      incidentBadgeItem({
        subject: "doctor.auto_repair",
        payload: { phase: "failed", mode: "degraded" },
      }),
    ).toEqual({
      subject: "doctor.auto_repair",
      phase: "failed",
      mode: "degraded",
    });
  });

  it("builds a compact incident summary from payload fields", () => {
    expect(
      incidentEventSummary({
        subject: "doctor.auto_repair",
        payload: {
          agent: "builder",
          delegate_to: "infra-lead",
          reason: "provider timeout",
        },
      }),
    ).toBe("builder · to infra-lead · provider timeout · doctor.auto_repair");
  });

  it("prefers routing rewrite detail when present", () => {
    expect(
      incidentEventSummary({
        subject: "doctor.auto_repair",
        payload: {
          agent: "builder",
          routing_task_type: "code",
          routing_task_model_chain: ["gpt-4.1", "deepseek-chat"],
        },
      }),
    ).toBe(
      "builder · rewrote code → gpt-4.1 → deepseek-chat · doctor.auto_repair",
    );
  });

  it("prefers routing rollback detail when present", () => {
    expect(
      incidentEventSummary({
        subject: "doctor.auto_repair",
        payload: {
          agent: "builder",
          phase: "routing_rollback_completed",
          routing_task_type: "code",
          routing_task_model_chain: ["gpt-5", "gpt-4.1"],
        },
      }),
    ).toBe(
      "builder · rolled back code → gpt-5 → gpt-4.1 · doctor.auto_repair",
    );
  });

  it("prefers forced-chain detail when a resolution was applied", () => {
    expect(
      incidentEventSummary({
        subject: "doctor.auto_repair",
        payload: {
          agent: "builder",
          phase: "resolution_applied",
          resolution: "force_chain",
          routing_task_type: "code",
          routing_task_model_chain: ["gpt-5", "gpt-4.1"],
        },
      }),
    ).toBe("builder · forced code → gpt-5 → gpt-4.1 · doctor.auto_repair");
  });

  it("includes force-chain generation when it is beyond the first apply", () => {
    expect(
      incidentEventSummary({
        subject: "doctor.auto_repair",
        payload: {
          agent: "builder",
          phase: "resolution_applied",
          resolution: "force_chain",
          routing_task_type: "code",
          routing_task_model_chain: ["gpt-5", "gpt-4.1"],
          routing_force_generation: 2,
        },
      }),
    ).toBe(
      "builder · forced code → gpt-5 → gpt-4.1 · gen 2 · doctor.auto_repair",
    );
  });
});
