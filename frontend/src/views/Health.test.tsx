// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { runDiagnostics, worstLevel } from "@/views/Health";

describe("runDiagnostics (M921)", () => {
  it("reports daemon-unreachable when status is missing", () => {
    const d = runDiagnostics(null, null, null);
    expect(d.map((x) => x.id)).toEqual(["daemon"]);
    expect(d[0].level).toBe("fail");
  });

  it("is empty (all healthy) for a clean daemon", () => {
    const st = { halted: false, model: "deepseek-chat", provider_fallbacks: { count: 0 }, pending_approvals: 0, schedules: { running: 0, resident: true } };
    const stats = { total: 100, failed: 1 };
    expect(runDiagnostics(st, stats, true)).toEqual([]);
  });

  it("flags halt, journal break, provider fallover, missing model, fail-rate, and pending approvals", () => {
    const st = {
      halted: true,
      model: "",
      provider_fallbacks: { count: 3, last_reason: "401 unauthorized" },
      pending_approvals: 2,
    };
    const stats = { total: 10, failed: 5 };
    const d = runDiagnostics(st, stats, false);
    const byId = Object.fromEntries(d.map((x) => [x.id, x]));
    expect(byId.halted.level).toBe("fail");
    expect(byId.journal.level).toBe("fail");
    expect(byId.provider.level).toBe("warn");
    expect(byId.provider.detail).toContain("401 unauthorized");
    expect(byId.model.level).toBe("warn");
    expect(byId.failrate.level).toBe("warn");
    expect(byId.approvals.level).toBe("info");
    // each actionable issue carries a deep-link.
    expect(byId.provider.fixHash).toBe("providers");
    expect(byId.approvals.fixHash).toBe("approvals");
  });

  it("surfaces active autonomous schedule work as an informational diagnostic", () => {
    const st = {
      halted: false,
      model: "deepseek-chat",
      provider_fallbacks: { count: 0 },
      pending_approvals: 0,
      schedules: { total: 4, enabled: 3, running: 2, resident: true },
    };
    const d = runDiagnostics(st, { total: 10, failed: 0 }, true);
    const sched = d.find((x) => x.id === "schedule-running");
    expect(sched?.level).toBe("info");
    expect(sched?.title).toBe("2 schedules running");
    expect(sched?.fixHash).toBe("schedules");
  });

  it("warns when enabled schedules exist but the cadence resident is offline", () => {
    const st = {
      halted: false,
      model: "deepseek-chat",
      provider_fallbacks: { count: 0 },
      pending_approvals: 0,
      schedules: { total: 2, enabled: 2, running: 0, resident: false },
    };
    const d = runDiagnostics(st, { total: 10, failed: 0 }, true);
    const sched = d.find((x) => x.id === "schedule-resident");
    expect(sched?.level).toBe("warn");
    expect(sched?.detail).toContain("2 enabled schedules");
    expect(sched?.fixHash).toBe("status");
  });

  it("ignores a high failure ratio when the sample is tiny", () => {
    const st = { halted: false, model: "m", provider_fallbacks: { count: 0 } };
    // 1 of 2 failed is 50% but only 2 runs — below the min-sample of 5.
    expect(runDiagnostics(st, { total: 2, failed: 1 }, true).some((x) => x.id === "failrate")).toBe(false);
  });
});

describe("worstLevel (M921)", () => {
  it("returns the most severe level, ok when empty", () => {
    expect(worstLevel([])).toBe("ok");
    expect(
      worstLevel([
        { id: "a", level: "info", title: "", detail: "" },
        { id: "b", level: "warn", title: "", detail: "" },
      ]),
    ).toBe("warn");
    expect(
      worstLevel([
        { id: "a", level: "warn", title: "", detail: "" },
        { id: "b", level: "fail", title: "", detail: "" },
      ]),
    ).toBe("fail");
  });
});
