// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, fireEvent } from "@testing-library/react";

// Mutable handle the mocked useEvents reads from, so each test can vary the
// live buffer / connection state (vi.hoisted survives vi.mock hoisting).
const h = vi.hoisted(() => ({ events: [] as any[], connected: true }));
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ events: h.events, connected: h.connected, subscribe: () => () => {} }),
}));

import { FleetNowBar } from "@/components/FleetNowBar";

afterEach(cleanup);
beforeEach(() => {
  h.events = [];
  h.connected = true;
});

describe("FleetNowBar", () => {
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
