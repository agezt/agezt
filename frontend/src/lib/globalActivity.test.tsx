// @vitest-environment jsdom
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { GlobalActivityProvider, useGlobalActivity } from "@/lib/globalActivity";
import type { AgentEvent } from "@/lib/events";

const mocks = vi.hoisted(() => ({
  getJSON: vi.fn(),
  listener: null as ((event: AgentEvent) => void) | null,
  connected: false,
}));

vi.mock("@/lib/api", () => ({
  getJSON: (...args: unknown[]) => mocks.getJSON(...args),
}));

vi.mock("@/lib/events", () => ({
  useEvents: () => ({
    connected: mocks.connected,
    subscribe: (listener: (event: AgentEvent) => void) => {
      mocks.listener = listener;
      return () => {
        if (mocks.listener === listener) mocks.listener = null;
      };
    },
  }),
}));

function Probe() {
  const { summary, seeding } = useGlobalActivity();
  return (
    <div>
      <span data-testid="running">{summary.running}</span>
      <span data-testid="seeding">{String(seeding)}</span>
    </div>
  );
}

describe("GlobalActivityProvider", () => {
  afterEach(cleanup);

  beforeEach(() => {
    mocks.connected = false;
    mocks.listener = null;
    mocks.getJSON.mockReset();
  });

  it("shows runs that started before the browser and folds their terminal event", async () => {
    mocks.getJSON.mockResolvedValue({
      runs: [
        {
          correlation_id: "cold-run",
          intent: "already working",
          status: "running",
          started_unix_ms: 10,
        },
      ],
    });
    render(
      <GlobalActivityProvider>
        <Probe />
      </GlobalActivityProvider>,
    );

    await waitFor(() => expect(screen.getByTestId("running").textContent).toBe("1"));
    expect(screen.getByTestId("seeding").textContent).toBe("false");

    act(() => {
      mocks.listener?.({
        kind: "task.completed",
        correlation_id: "cold-run",
        ts_unix_ms: 20,
        payload: {},
      });
    });
    expect(screen.getByTestId("running").textContent).toBe("0");
  });

  it("does not erase a live event that arrives while the snapshot is loading", async () => {
    let resolveSnapshot: ((value: unknown) => void) | undefined;
    mocks.getJSON.mockReturnValue(new Promise((resolve) => {
      resolveSnapshot = resolve;
    }));
    render(
      <GlobalActivityProvider>
        <Probe />
      </GlobalActivityProvider>,
    );

    act(() => {
      mocks.listener?.({
        kind: "task.received",
        correlation_id: "new-run",
        ts_unix_ms: 30,
        payload: { intent: "arrived live" },
      });
    });
    expect(screen.getByTestId("running").textContent).toBe("1");

    await act(async () => {
      resolveSnapshot?.({ runs: [] });
      await Promise.resolve();
    });
    expect(screen.getByTestId("running").textContent).toBe("1");
  });
});
