// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
vi.mock("@/app/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
}));
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ events: [], connected: true, subscribe: () => () => {} }),
}));

import { Health, runDiagnostics, worstLevel, type Diagnostic } from "@/views/Health";

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  getJSON.mockResolvedValue({});
});

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

  it("downgrades a provider failover to info once the last one is >24h old", () => {
    const dayAndBit = 25 * 60 * 60 * 1000;
    const st = {
      halted: false,
      model: "m",
      provider_fallbacks: { count: 8, last_reason: "400 tools[12].name", last_ms: Date.now() - dayAndBit },
      pending_approvals: 0,
      schedules: { running: 0, resident: true },
    };
    const d = runDiagnostics(st, { total: 10, failed: 0 }, true);
    const prov = d.find((x) => x.id === "provider");
    expect(prov?.level).toBe("info");
    expect(prov?.title).toBe("Past provider failovers");
    // A RECENT failover still warns, and the detail carries the clock time.
    const fresh = {
      ...st,
      provider_fallbacks: { count: 1, last_reason: "500", last_ms: Date.now() - 60_000 },
    };
    const d2 = runDiagnostics(fresh, { total: 10, failed: 0 }, true);
    expect(d2.find((x) => x.id === "provider")?.level).toBe("warn");
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

// The schedules tile came from the merged System view (2026-09). It is the only
// place the console distinguishes "enabled but the cadence resident is offline"
// — schedules that will never fire — from a healthy idle count, so the display
// logic moved here with it rather than being dropped.
describe("Health schedules tile (merged from System)", () => {
  const base = {
    daemon: "test",
    model: "mock",
    halted: false,
    uptime_seconds: 2,
    active_runs: 0,
    pending_approvals: 0,
    journal_head: 7,
    tools: 3,
  };

  it("shows live schedule firings instead of only enabled/total counts", async () => {
    getJSON.mockImplementation((p: string) =>
      p === "/api/status"
        ? Promise.resolve({ ...base, schedules: { total: 5, enabled: 4, running: 1, resident: true } })
        : Promise.resolve({}),
    );
    render(<Health />);
    await waitFor(() => expect(screen.getByText("1 live")).toBeTruthy());
    expect(screen.queryByText("4/5")).toBeNull();
  });

  it("shows offline when enabled schedules have no cadence resident", async () => {
    getJSON.mockImplementation((p: string) =>
      p === "/api/status"
        ? Promise.resolve({ ...base, schedules: { total: 3, enabled: 2, running: 0, resident: false } })
        : Promise.resolve({}),
    );
    render(<Health />);
    await waitFor(() => expect(screen.getByText("offline")).toBeTruthy());
  });

  it("keeps the System view's own counters", async () => {
    getJSON.mockImplementation((p: string) =>
      p === "/api/status"
        ? Promise.resolve({ ...base, world_entities: 12, active_skills: 4, schedules: { total: 0, enabled: 0 } })
        : Promise.resolve({}),
    );
    render(<Health />);
    await waitFor(() => expect(screen.getByText("journal head")).toBeTruthy());
    expect(screen.getByText("world entities")).toBeTruthy();
    expect(screen.getByText("active skills")).toBeTruthy();
    expect(screen.getByText("daemon")).toBeTruthy();
  });
});
