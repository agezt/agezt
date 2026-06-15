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
