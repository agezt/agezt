// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, fireEvent } from "@testing-library/react";

// Mutable handle the mocked useEvents reads from, so each test can vary the
// live buffer / connection state (vi.hoisted survives vi.mock hoisting).
const h = vi.hoisted(() => ({ events: [] as any[], connected: true }));
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ events: h.events, connected: h.connected, subscribe: () => () => {} }),
}));

import { FleetNowBar, fleetNowSummary, liveRunsFromEvents } from "@/components/FleetNowBar";

afterEach(cleanup);
beforeEach(() => {
  h.events = [];
  h.connected = true;
});

describe("FleetNowBar", () => {
  it("builds a live operational summary for awake, repair, tool, model and agentless runs", () => {
    expect(
      fleetNowSummary([
        { corr: "c1", agent: "builder", phase: "using tool", tool: "shell" },
        { corr: "c2", agent: "doctor", phase: "repair queued" },
        { corr: "c3", phase: "thinking", model: "gpt-5" },
      ]),
    ).toMatchObject({
      awake: 3,
      repairing: 1,
      toolUsers: 1,
      modelThinkers: 1,
      agentless: 1,
      value: "3 awake · 1 repair",
      tone: "warn",
    });
  });

  it("shows the idle state when no runs are live", () => {
    render(<FleetNowBar />);
    expect(screen.getByText(/fleet idle · listening/)).toBeTruthy();
  });

  it("summarizes a running agent (collapsed stack + ticker), not idle", () => {
    h.events = [
      { seq: 1, kind: "task.received", correlation_id: "c1", actor: "alice", payload: { intent: "deploy the app" } },
    ];
    render(<FleetNowBar />);
    // Collapsed: a compact "N running" summary (avatar stack), not a wall of text.
    expect(screen.getByText(/1 running/)).toBeTruthy();
    // The agent + intent still surface in the live event ticker.
    expect(screen.getAllByText(/alice/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/deploy the app/).length).toBeGreaterThan(0);
    expect(screen.getByLabelText("Now ledger").textContent).toContain("awake 1");
    expect(screen.queryByText(/fleet idle/)).toBeNull();
  });

  it("does not deep-link to a run id as if it were an agent (M980)", () => {
    // Ad-hoc run: actor is the run correlation id, no payload.agent. Clicking it
    // must go to the Overseer, NOT to #agent/agent-run-… (which 404s).
    h.events = [
      { seq: 1, kind: "task.received", correlation_id: "agent-run-XYZ", actor: "agent-run-XYZ", payload: { intent: "ad-hoc" } },
    ];
    location.hash = "";
    const onNavigate = vi.fn();
    render(<FleetNowBar onNavigate={onNavigate} />);
    fireEvent.click(screen.getByText(/1 running/)); // expand
    fireEvent.click(screen.getAllByText(/ad-hoc/)[0]); // click the card
    expect(onNavigate).toHaveBeenCalledWith("overseer");
    expect(location.hash).not.toContain("agent-run");
  });

  it("deep-links to the real agent slug from the payload (M980)", () => {
    h.events = [
      { seq: 1, kind: "task.received", correlation_id: "agent-run-XYZ", actor: "agent-run-XYZ", payload: { intent: "real", agent: "researcher" } },
    ];
    location.hash = "";
    render(<FleetNowBar />);
    fireEvent.click(screen.getByText(/1 running/));
    fireEvent.click(screen.getAllByText(/real/)[0]);
    expect(location.hash).toContain("agent/researcher");
  });

  it("opens a running agent's page directly from the collapsed avatar (M991)", () => {
    // The owner's ask: click a running agent at the top and go straight to its
    // page — no need to expand the slider first.
    h.events = [
      { seq: 1, kind: "task.received", correlation_id: "agent-run-XYZ", actor: "agent-run-XYZ", payload: { intent: "real", agent: "researcher" } },
    ];
    location.hash = "";
    render(<FleetNowBar />);
    fireEvent.click(screen.getByTitle("Open researcher's page"));
    expect(location.hash).toContain("agent/researcher");
  });

  it("expands into running-agent cards on click", () => {
    h.events = [
      { seq: 1, kind: "task.received", correlation_id: "c1", actor: "alice", payload: { intent: "deploy the app" } },
    ];
    render(<FleetNowBar />);
    fireEvent.click(screen.getByText(/1 running/));
    // Expanded slider shows the intent in a card and a Collapse control.
    expect(screen.getByText(/Collapse/)).toBeTruthy();
    expect(screen.getAllByText(/deploy the app/).length).toBeGreaterThan(0);
  });

  it("keeps the latest non-terminal phase on a running agent card", () => {
    h.events = [
      { seq: 3, kind: "tool.invoked", correlation_id: "c1", actor: "worker", ts_unix_ms: 3000, payload: { tool: "shell", iter: 1 } },
      { seq: 2, kind: "llm.response", correlation_id: "c1", actor: "worker", ts_unix_ms: 2000, payload: { iter: 1, model: "gpt-5" } },
      { seq: 1, kind: "task.received", correlation_id: "c1", actor: "worker", ts_unix_ms: 1000, payload: { intent: "repair disk", agent: "worker" } },
    ];
    expect(liveRunsFromEvents(h.events)).toMatchObject([
      {
        corr: "c1",
        agent: "worker",
        intent: "repair disk",
        phase: "using tool",
        tool: "shell",
        detail: "iter 1 · shell",
        lastTs: 3000,
      },
    ]);
    render(<FleetNowBar />);
    fireEvent.click(screen.getByText(/1 running/));
    expect(screen.getByLabelText("Now ledger").textContent).toContain("tools 1");
    expect(screen.getByText("using tool")).toBeTruthy();
    expect(screen.getByText("shell")).toBeTruthy();
    expect(screen.getByText("iter 1 · shell")).toBeTruthy();
  });

  it("shows standalone doctor repair work without a task.received prelude", () => {
    h.events = [
      {
        seq: 4,
        subject: "doctor.auto_repair",
        kind: "info",
        correlation_id: "repair-1",
        actor: "guardian-doctor",
        ts_unix_ms: 4000,
        payload: { agent: "builder", mode: "degraded", phase: "queued", reason: "provider timeout" },
      },
    ];
    expect(liveRunsFromEvents(h.events)).toMatchObject([
      {
        corr: "repair-1",
        agent: "builder",
        intent: "degraded repair builder",
        phase: "repair queued",
        detail: "degraded repair · agent builder · provider timeout",
      },
    ]);
    render(<FleetNowBar />);
    expect(screen.getByLabelText("Now ledger").textContent).toContain("repair 1");
    fireEvent.click(screen.getByText(/1 running/));
    expect(screen.getByText("repair queued")).toBeTruthy();
    expect(screen.getByText("degraded repair · agent builder · provider timeout")).toBeTruthy();
  });

  it("drops standalone doctor repair work after a terminal repair phase", () => {
    h.events = [
      {
        seq: 5,
        subject: "doctor.auto_repair",
        kind: "info",
        correlation_id: "repair-1",
        actor: "guardian-doctor",
        payload: { agent: "builder", mode: "degraded", phase: "completed" },
      },
      {
        seq: 4,
        subject: "doctor.auto_repair",
        kind: "info",
        correlation_id: "repair-1",
        actor: "guardian-doctor",
        payload: { agent: "builder", mode: "degraded", phase: "queued" },
      },
    ];
    expect(liveRunsFromEvents(h.events)).toEqual([]);
    render(<FleetNowBar />);
    expect(screen.getByText(/fleet idle · listening/)).toBeTruthy();
  });

  it("drops a run whose most recent lifecycle event is terminal", () => {
    // newest-first: completed is the latest event for c1 → not running.
    h.events = [
      { seq: 2, kind: "task.completed", correlation_id: "c1", actor: "alice" },
      { seq: 1, kind: "task.received", correlation_id: "c1", actor: "alice", payload: { intent: "old task" } },
    ];
    render(<FleetNowBar />);
    expect(screen.getByText(/fleet idle · listening/)).toBeTruthy();
    expect(screen.queryByText(/old task/)).toBeNull();
  });

  it("signals a dropped live stream", () => {
    h.connected = false;
    render(<FleetNowBar />);
    expect(screen.getByText(/reconnecting/)).toBeTruthy();
  });
});
