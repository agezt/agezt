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

import { Status } from "@/views/Status";

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  liveEvents = [];
});

describe("Status", () => {
  it("shows live schedule firings instead of only enabled/total counts", async () => {
    getJSON.mockResolvedValue({
      daemon: "test",
      protocol: 1,
      model: "mock",
      halted: false,
      uptime_seconds: 2,
      active_runs: 0,
      pending_approvals: 0,
      journal_head: 7,
      tools: 3,
      schedules: { total: 5, enabled: 4, running: 1, resident: true },
      delegation: { enabled: true, max_depth: 2 },
    });

    render(<Status />);

    await waitFor(() => expect(screen.getByText("1 live")).toBeTruthy());
    expect(screen.queryByText("4/5")).toBeNull();
  });

  it("shows offline when enabled schedules have no cadence resident", async () => {
    getJSON.mockResolvedValue({
      daemon: "test",
      protocol: 1,
      model: "mock",
      halted: false,
      uptime_seconds: 2,
      active_runs: 0,
      pending_approvals: 0,
      journal_head: 7,
      tools: 3,
      schedules: { total: 3, enabled: 2, running: 0, resident: false },
      delegation: { enabled: true, max_depth: 2 },
    });

    render(<Status />);

    await waitFor(() => expect(screen.getByText("offline")).toBeTruthy());
    expect(screen.queryByText("2/3")).toBeNull();
  });
});
