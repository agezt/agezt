// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, waitFor } from "@testing-library/react";
import type { AgentEvent } from "@/lib/events";

const getJSON = vi.fn();
let liveEvents: AgentEvent[] = [];
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
}));
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ events: liveEvents, connected: true, subscribe: () => () => {} }),
}));
vi.mock("@/lib/runfocus", () => ({ focusRun: vi.fn() }));
vi.mock("@/views/Agents", () => ({
  summarizeRoots: () => [],
}));

import { Dashboard, dashboardFleetOpsSummary } from "@/views/Dashboard";
import { setAdvanced } from "@/lib/advanced";

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  liveEvents = [];
  setAdvanced(false); // calm by default; the ticker test opts in
});

describe("Dashboard", () => {
  it("summarizes roster operations, repair pressure, graveyard, and mailbox backlog", () => {
    expect(
      dashboardFleetOpsSummary(
        [
          { slug: "ops", enabled: true, status: { active_run_count: 1 } },
          { slug: "builder", enabled: true, status: { health_state: "degraded", repair_state: "queued" } },
          { slug: "worker", enabled: true, kind: "subagent" },
          { slug: "guardian", enabled: true, system: true, kind: "system" },
          { slug: "paused", enabled: false },
          { slug: "old", retired: true, status: { health_state: "degraded" } },
        ],
        [
          { id: "m1", from: "user", to: "ops" },
          { id: "m2", from: "user", to: "ops", acked_by: ["ops"] },
          { id: "m3", from: "user", to: "*", acked_by: ["builder", "guardian", "paused", "old"] },
          { id: "r1", from: "worker", reply_to: "m3" },
        ],
      ),
    ).toEqual({
      total: 6,
      active: 4,
      paused: 1,
      running: 1,
      repair: 1,
      graveyard: 1,
      mailboxAgents: 1,
      mailboxBacklog: 2,
      system: 1,
      subagents: 1,
    });
  });

  it("shows roster operations on the cockpit", async () => {
    getJSON.mockImplementation((url: string) => {
      switch (url) {
        case "/api/stats":
        case "/api/budget":
          return Promise.resolve({});
        case "/api/status":
          return Promise.resolve({ journal_head: 1, schedules: { total: 0, enabled: 0 } });
        case "/api/journal":
          return Promise.resolve({ events: [] });
        case "/api/runs":
          return Promise.resolve({ runs: [] });
        case "/api/agents":
          return Promise.resolve({
            profiles: [
              { slug: "ops", enabled: true, status: { active_run_count: 1 } },
              { slug: "builder", enabled: true, status: { health_state: "degraded", repair_state: "queued" } },
              { slug: "paused", enabled: false },
              { slug: "old", retired: true },
            ],
          });
        case "/api/board":
          return Promise.resolve({ messages: [{ id: "m1", from: "user", to: "ops" }] });
        default:
          return Promise.resolve({});
      }
    });

    render(<Dashboard />);
    await waitFor(() => expect(screen.getByText("Agent operations")).toBeTruthy());
    expect(screen.getByText("2 active · 1 paused · 1 graveyard")).toBeTruthy();
    expect(screen.getByText("1 agent need repair attention")).toBeTruthy();
    expect(screen.getByText("1 mailbox message waiting across 1 agent")).toBeTruthy();
  });

  it("shows doctor provenance and phase badges in needs-attention strip", async () => {
    const now = Date.now();
    getJSON.mockImplementation((url: string) => {
      switch (url) {
        case "/api/stats":
          return Promise.resolve({});
        case "/api/budget":
          return Promise.resolve({});
        case "/api/status":
          return Promise.resolve({ journal_head: 1, schedules: { total: 0, enabled: 0 } });
        case "/api/journal":
          return Promise.resolve({
            events: [
              {
                id: "e1",
                kind: "info",
                subject: "doctor.auto_repair",
                ts_unix_ms: now,
                payload: {
                  agent: "builder",
                  phase: "delegation_failed",
                  reason: "target agent infra-lead is paused",
                  delegate_to: "infra-lead",
                },
              },
            ],
          });
        case "/api/runs":
          return Promise.resolve({ runs: [] });
        default:
          return Promise.resolve({});
      }
    });

    render(<Dashboard />);
    await waitFor(() =>
      expect(screen.getByText("delegated wake failed")).toBeTruthy(),
    );
    expect(screen.getAllByText("doctor").length).toBeGreaterThan(0);
    expect(screen.getByText("delegate failed")).toBeTruthy();
  });

  it("shows operator/phase badges in the live event ticker for incident-family events", async () => {
    setAdvanced(true); // the raw ticker lives in Advanced mode
    const now = Date.now();
    liveEvents = [
      {
        id: "live-1",
        kind: "info",
        subject: "agent.wake",
        ts_unix_ms: now,
        payload: {
          agent: "infra-lead",
          phase: "completed",
          reason: "incident owner woke",
        },
      } as AgentEvent,
    ];
    getJSON.mockImplementation((url: string) => {
      switch (url) {
        case "/api/stats":
        case "/api/budget":
          return Promise.resolve({});
        case "/api/status":
          return Promise.resolve({ journal_head: 1, schedules: { total: 0, enabled: 0 } });
        case "/api/journal":
          return Promise.resolve({ events: [] });
        case "/api/runs":
          return Promise.resolve({ runs: [] });
        default:
          return Promise.resolve({});
      }
    });

    render(<Dashboard />);
    await waitFor(() =>
      expect(screen.getByText("operator")).toBeTruthy(),
    );
    expect(screen.getAllByText("completed").length).toBeGreaterThan(0);
    expect(screen.getByText(/infra-lead · incident owner woke · agent\.wake/)).toBeTruthy();
  });

  it("surfaces live schedule firings in the cockpit schedule gauge", async () => {
    getJSON.mockImplementation((url: string) => {
      switch (url) {
        case "/api/stats":
        case "/api/budget":
          return Promise.resolve({});
        case "/api/status":
          return Promise.resolve({ journal_head: 1, schedules: { total: 4, enabled: 3, running: 2 } });
        case "/api/journal":
          return Promise.resolve({ events: [] });
        case "/api/runs":
          return Promise.resolve({ runs: [] });
        default:
          return Promise.resolve({});
      }
    });

    render(<Dashboard />);

    await waitFor(() => expect(screen.getByText("2 live")).toBeTruthy());
    expect(screen.getByText("of 4 schedules")).toBeTruthy();
  });

  it("warns when enabled schedules have no resident cadence engine", async () => {
    getJSON.mockImplementation((url: string) => {
      switch (url) {
        case "/api/stats":
        case "/api/budget":
          return Promise.resolve({});
        case "/api/status":
          return Promise.resolve({ journal_head: 1, schedules: { total: 3, enabled: 2, running: 0, resident: false } });
        case "/api/journal":
          return Promise.resolve({ events: [] });
        case "/api/runs":
          return Promise.resolve({ runs: [] });
        default:
          return Promise.resolve({});
      }
    });

    render(<Dashboard />);

    await waitFor(() => expect(screen.getByText("offline")).toBeTruthy());
    expect(screen.getByText("of 3 schedules")).toBeTruthy();
  });

  it("shows error rate and delegation count from stats", async () => {
    getJSON.mockImplementation((url: string) => {
      switch (url) {
        case "/api/stats":
          return Promise.resolve({ total: 10, completed: 7, failed: 3, running: 0, delegations: 2 });
        case "/api/budget":
          return Promise.resolve({});
        case "/api/status":
          return Promise.resolve({ journal_head: 1, schedules: { total: 0, enabled: 0 } });
        case "/api/journal":
          return Promise.resolve({ events: [] });
        case "/api/runs":
          return Promise.resolve({ runs: [] });
        default:
          return Promise.resolve({});
      }
    });

    render(<Dashboard />);
    await waitFor(() => expect(screen.getByText("error rate")).toBeTruthy());
    expect(screen.getByText("30%")).toBeTruthy();
    expect(screen.getByText("delegations")).toBeTruthy();
  });
});
