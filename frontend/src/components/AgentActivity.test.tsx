// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
}));
vi.mock("@/lib/events", () => ({
  useEvents: () => ({
    events: [],
    subscribe: () => () => {},
  }),
}));

import { AgentActivity } from "@/components/AgentActivity";
import { UIProvider } from "@/components/ui/feedback";

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  getJSON.mockImplementation((path: string) => {
    if (path === "/api/agents/activity") {
      return Promise.resolve({
        activity: [
          {
            seq: 1,
            kind: "task.received",
            ts_unix_ms: 10,
            correlation_id: "corr-1",
            summary: "started a run: check disks",
          },
        ],
      });
    }
    if (path === "/api/runs") {
      return Promise.resolve({
        runs: [
          {
            correlation_id: "corr-1",
            agent: "ops",
            status: "running",
            intent: "check disks",
            started_unix_ms: 10,
          },
        ],
      });
    }
    if (path === "/api/journal") {
      return Promise.resolve({
        events: [
          {
            seq: 1,
            kind: "task.received",
            ts_unix_ms: 10,
            correlation_id: "corr-1",
            payload: { intent: "check disks" },
          },
        ],
      });
    }
    return Promise.resolve({});
  });
});

describe("AgentActivity", () => {
  it("keeps a focused run in one top panel instead of duplicating inline detail", async () => {
    render(
      <UIProvider>
        <AgentActivity slug="ops" initialOpenRun="corr-1" />
      </UIProvider>,
    );

    await waitFor(() => expect(screen.getByText(/focused run/)).toBeTruthy());
    expect(screen.getByText("awake")).toBeTruthy();
    expect(screen.getByText("1 live run")).toBeTruthy();
    expect(screen.getByText("focused")).toBeTruthy();
    expect(screen.getAllByText(/corr-1/).length).toBeGreaterThan(0);
    await waitFor(() => expect(screen.getAllByText(/phase timeline/)).toHaveLength(1));

    fireEvent.click(screen.getByText(/started a run: check disks/));
    await waitFor(() => expect(screen.queryByText(/focused run/)).toBeNull());
    expect(screen.queryAllByText(/phase timeline/)).toHaveLength(0);

    fireEvent.click(screen.getByText(/started a run: check disks/));
    await waitFor(() => expect(screen.getAllByText(/phase timeline/)).toHaveLength(1));
  });
});
