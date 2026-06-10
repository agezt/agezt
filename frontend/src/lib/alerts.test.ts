import { describe, it, expect } from "vitest";
import { classifyAlert, isAlert, LEVEL_ORDER, attentionAlertCount } from "@/lib/alerts";
import type { AgentEvent } from "@/lib/events";

function ev(kind: string, payload: any = {}): AgentEvent {
  return { kind, payload } as AgentEvent;
}

describe("classifyAlert", () => {
  it("maps a self-health observer delta with its severity", () => {
    const a = classifyAlert(
      ev("observer.delta", {
        source: "self:health",
        kind: "health_degraded",
        summary: "daemon health healthy → degraded: tool errors 4/12 (33%)",
        hints: { severity: "high" },
      }),
    )!;
    expect(a.level).toBe("warning");
    expect(a.source).toBe("self:health");
    expect(a.title).toContain("degraded");
  });

  it("escalates a critical-severity delta to critical", () => {
    const a = classifyAlert(ev("observer.delta", { summary: "x", hints: { severity: "critical" } }))!;
    expect(a.level).toBe("critical");
  });

  it("treats a low/medium delta as info", () => {
    const a = classifyAlert(ev("observer.delta", { summary: "recovered", hints: { severity: "medium" } }))!;
    expect(a.level).toBe("info");
  });

  it("maps a briefing, escalating disposition=alert to warning", () => {
    expect(classifyAlert(ev("briefing.sent", { title: "CI broke", disposition: "alert" }))!.level).toBe("warning");
    expect(classifyAlert(ev("briefing.sent", { title: "fyi", disposition: "digest" }))!.level).toBe("info");
  });

  it("maps run failures with their reason", () => {
    const a = classifyAlert(ev("task.failed", { reason: "max_iters" }))!;
    expect(a.level).toBe("warning");
    expect(a.detail).toBe("max_iters");
  });

  it("maps egress blocks, budget, rate, halt, capability", () => {
    expect(classifyAlert(ev("netguard.blocked", { ip: "169.254.169.254", reason: "metadata" }))!.level).toBe("warning");
    expect(classifyAlert(ev("budget.exceeded"))!.level).toBe("critical");
    expect(classifyAlert(ev("rate.limited", { provider: "deepseek" }))!.level).toBe("warning");
    expect(classifyAlert(ev("halt", { reason: "operator" }))!.level).toBe("critical");
    expect(classifyAlert(ev("capability.rejected", { capability: "shell" }))!.level).toBe("info");
  });

  it("returns null for ordinary stream events", () => {
    expect(classifyAlert(ev("llm.token"))).toBeNull();
    expect(classifyAlert(ev("tool.invoked"))).toBeNull();
    expect(classifyAlert(ev("task.completed"))).toBeNull();
    expect(isAlert(ev("llm.request"))).toBe(false);
    expect(isAlert(ev("task.failed"))).toBe(true);
  });

  it("ranks severity", () => {
    expect(LEVEL_ORDER.critical).toBeGreaterThan(LEVEL_ORDER.warning);
    expect(LEVEL_ORDER.warning).toBeGreaterThan(LEVEL_ORDER.info);
  });
});

describe("attentionAlertCount (M779)", () => {
  it("counts warning/critical alerts and ignores info-level and non-alert events", () => {
    const events: AgentEvent[] = [
      ev("task.failed", { reason: "boom" }), // warning
      ev("budget.exceeded", {}), // critical
      ev("capability.rejected", { capability: "shell" }), // info → not counted
      ev("tool.result", {}), // not an alert
      ev("netguard.blocked", { ip: "1.2.3.4" }), // warning
    ];
    expect(attentionAlertCount(events)).toBe(3);
    expect(attentionAlertCount([])).toBe(0);
  });
});
