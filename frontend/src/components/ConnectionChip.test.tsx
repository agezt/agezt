// @vitest-environment jsdom
//
// Smoke test for the S4.1 ConnectionChip. We mount the EventsProvider with a
// mocked context value to drive each of the three connection states through
// the chip's rendering path. The pure classification logic is covered by
// events.test.ts; here we just verify the chip renders the right role +
// data-connection-state attribute.

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, cleanup, act } from "@testing-library/react";

vi.mock("@/lib/events", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/events")>();
  return {
    ...actual,
    useEvents: vi.fn(),
  };
});

import { ConnectionChip } from "@/components/ConnectionChip";
import { useEvents } from "@/lib/events";

const mockedUseEvents = vi.mocked(useEvents);

beforeEach(() => {
  vi.useFakeTimers({ now: 1_700_000_000_000 });
  mockedUseEvents.mockReset();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("ConnectionChip", () => {
  it("renders the live state when connected and a recent event exists", () => {
    mockedUseEvents.mockReturnValue({
      connected: true,
      lastEventAt: 1_700_000_000_000,
      events: [],
      subscribe: () => () => {},
    });
    render(<ConnectionChip />);
    const chip = screen.getByRole("status");
    expect(chip.getAttribute("data-connection-state")).toBe("live");
    expect(chip.title).toMatch(/live/);
  });

  it("renders the stale state when connected but no event within STALE_MS", () => {
    mockedUseEvents.mockReturnValue({
      connected: true,
      lastEventAt: 1_700_000_000_000 - 30_000, // 30 s ago
      events: [],
      subscribe: () => () => {},
    });
    render(<ConnectionChip />);
    expect(screen.getByRole("status").getAttribute("data-connection-state")).toBe("stale");
  });

  it("renders the stale state when connected but no event has arrived yet", () => {
    mockedUseEvents.mockReturnValue({
      connected: true,
      lastEventAt: null,
      events: [],
      subscribe: () => () => {},
    });
    render(<ConnectionChip />);
    expect(screen.getByRole("status").getAttribute("data-connection-state")).toBe("stale");
  });

  it("renders the disconnected state when the socket is closed", () => {
    mockedUseEvents.mockReturnValue({
      connected: false,
      lastEventAt: 1_700_000_000_000 - 4_000,
      events: [],
      subscribe: () => () => {},
    });
    render(<ConnectionChip />);
    expect(screen.getByRole("status").getAttribute("data-connection-state")).toBe("disconnected");
  });

  it("recomputes state on a tick so the staleness flips without a provider update", () => {
    // Start 'live', advance the clock past STALE_MS, tick — should flip stale.
    mockedUseEvents.mockReturnValue({
      connected: true,
      lastEventAt: 1_700_000_000_000,
      events: [],
      subscribe: () => () => {},
    });
    render(<ConnectionChip />);
    expect(screen.getByRole("status").getAttribute("data-connection-state")).toBe("live");
    // 20 s pass in the fake clock. The chip re-reads connectionState on
    // its 1 s tick (the first happens after 1 s of fake time).
    act(() => {
      vi.advanceTimersByTime(20_000);
    });
    expect(screen.getByRole("status").getAttribute("data-connection-state")).toBe("stale");
  });
});
