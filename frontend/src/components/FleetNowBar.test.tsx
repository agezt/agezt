// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

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

  it("shows a breathing chip for a running agent with its intent", () => {
    h.events = [
      { seq: 1, kind: "task.received", correlation_id: "c1", actor: "alice", payload: { intent: "deploy the app" } },
    ];
    render(<FleetNowBar />);
    // "alice" / the intent appear in both the chip and the event ticker.
    expect(screen.getAllByText("alice").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/deploy the app/).length).toBeGreaterThan(0);
    expect(screen.queryByText(/fleet idle/)).toBeNull();
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
