import { describe, it, expect } from "vitest";
import {
  classifyAlert,
  isAlert,
  LEVEL_ORDER,
  attentionAlertCount,
  recentAttentionAlerts,
  daemonHalted,
} from "@/lib/alerts";
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

describe("recentAttentionAlerts (M780)", () => {
  it("returns warning/critical alerts newest-first, deduped, capped, with title+detail", () => {
    const mk = (id: string, kind: string, ts: number, payload: any = {}) => ({ ...ev(kind, payload), id, ts_unix_ms: ts });
    const events = [
      { ...ev("task.failed", { reason: "boom" }), id: "a", ts_unix_ms: 30, correlation_id: "c-a" },
      mk("b", "budget.exceeded", 40),
      mk("a", "task.failed", 30, { reason: "boom" }), // dup id → ignored
      mk("c", "capability.rejected", 50, { capability: "shell" }), // info → excluded
      mk("d", "tool.result", 60), // not an alert
      mk("e", "netguard.blocked", 20, { ip: "1.2.3.4" }),
    ];
    const out = recentAttentionAlerts(events, 5);
    expect(out.map((r) => r.id)).toEqual(["b", "a", "e"]); // ts 40,30,20; info+dup+non-alert dropped
    expect(out[0].title).toBe("budget ceiling exceeded");
    expect(out[1].detail).toBe("boom");
    expect(out[1].correlationId).toBe("c-a"); // correlation threaded for M781
  });

  it("respects the limit", () => {
    const mk = (id: string, ts: number) => ({ ...ev("task.failed", {}), id, ts_unix_ms: ts });
    const out = recentAttentionAlerts([mk("a", 1), mk("b", 2), mk("c", 3)], 2);
    expect(out.map((r) => r.id)).toEqual(["c", "b"]);
  });
});

describe("daemonHalted (M913)", () => {
  const at = (kind: string, ts: number) => ({ ...ev(kind), ts_unix_ms: ts });
  it("is true only when the latest halt/resume transition is a halt", () => {
    expect(daemonHalted([])).toBe(false);
    expect(daemonHalted([at("halt", 10)])).toBe(true);
    expect(daemonHalted([at("halt", 10), at("resume", 20)])).toBe(false); // resumed after
    expect(daemonHalted([at("resume", 5), at("halt", 30)])).toBe(true); // halted again
  });
});

describe("attention de-staling (M913)", () => {
  const at = (id: string, kind: string, ts: number, p: any = {}) => ({ ...ev(kind, p), id, ts_unix_ms: ts });

  it("drops a halt alert once a later resume cleared it", () => {
    const events = [at("h", "halt", 10, { reason: "manual" }), at("r", "resume", 20)];
    expect(recentAttentionAlerts(events).map((a) => a.id)).not.toContain("h");
    expect(attentionAlertCount(events)).toBe(0);
  });

  it("keeps a halt that is still in effect (no resume after it)", () => {
    const events = [at("h", "halt", 30, { reason: "manual" }), at("r", "resume", 10)];
    expect(recentAttentionAlerts(events).map((a) => a.id)).toContain("h");
    expect(attentionAlertCount(events)).toBe(1);
  });

  it("ages out alerts older than the recency window when nowMs is given", () => {
    const now = 1_000_000_000_000;
    const fresh = at("fresh", "task.failed", now - 60_000); // 1 min ago
    const stale = at("stale", "task.failed", now - 3 * 24 * 60 * 60 * 1000); // 3 days ago
    const out = recentAttentionAlerts([fresh, stale], { nowMs: now });
    expect(out.map((a) => a.id)).toEqual(["fresh"]);
    expect(attentionAlertCount([fresh, stale], { nowMs: now })).toBe(1);
    // Without nowMs the window is off — both still count (legacy behavior).
    expect(attentionAlertCount([fresh, stale])).toBe(2);
  });
});
