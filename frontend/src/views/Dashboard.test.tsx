// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, waitFor, within, fireEvent } from "@testing-library/react";
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
    await waitFor(() => expect(screen.getByText("Agents overview")).toBeTruthy());
    // Fleet ops now renders as a labelled MiniMetric strip; repair + mailbox
    // backlog are surfaced as dedicated action buttons under the tiles.
    const opsCard = screen.getByText("Agents overview").closest("section") as HTMLElement;
    const ops = within(opsCard);
    // Labelled tiles for the key fleet states are present.
    expect(ops.getByText("paused")).toBeTruthy();
    expect(ops.getByText("inactive")).toBeTruthy();
    expect(ops.getByText("repair")).toBeTruthy();
    expect(ops.getByText("mailbox")).toBeTruthy();
    // 1 agent needs repair, 1 mailbox message waiting.
    expect(ops.getByText("1 need repair")).toBeTruthy();
    expect(ops.getByText("1 messages waiting")).toBeTruthy();
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

  it("shows incident-family events in the live event ticker", async () => {
    setAdvanced(true); // the raw ticker lives in Advanced mode
    const now = Date.now();
    liveEvents = [
      {
        id: "live-1",
        kind: "agent.wake",
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
    // The raw live stream now lives in the Advanced "Activity" tab; Radix only
    // mounts the active tab's content, so switch to it once the cockpit has
    // settled. Each event renders as kind + summary (agent · reason); incident
    // source/phase badges were dropped from this ticker in the redesign.
    await screen.findByText("Success rate");
    // Radix tabs use automatic activation — focusing the trigger selects it.
    fireEvent.focus(screen.getByRole("tab", { name: /Activity/ }));
    await waitFor(() =>
      expect(screen.getAllByText("agent.wake").length).toBeGreaterThan(0),
    );
    expect(screen.getByText("infra-lead · incident owner woke")).toBeTruthy();
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
    expect(screen.getByText("3 of 4 enabled")).toBeTruthy();
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
    expect(screen.getByText("2 of 3 enabled")).toBeTruthy();
  });

  it("shows failed-run and delegation counts from stats", async () => {
    getJSON.mockImplementation((url: string) => {
      switch (url) {
        case "/api/stats":
          return Promise.resolve({ total: 10, completed: 7, failed: 3, running: 0, delegations: 2, success_rate: 0.7 });
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
    // The run counters now surface Failed and Delegations as labelled widgets.
    await waitFor(() => expect(screen.getByText("Delegations")).toBeTruthy());
    const failedCard = screen.getByText("Failed").closest("div.rounded-xl") as HTMLElement;
    expect(within(failedCard).getByText("3")).toBeTruthy();
    const delegCard = screen.getByText("Delegations").closest("div.rounded-xl") as HTMLElement;
    expect(within(delegCard).getByText("2")).toBeTruthy();
    // Success rate widget reflects the reported rate.
    expect(screen.getByText("70%")).toBeTruthy();
  });
});
