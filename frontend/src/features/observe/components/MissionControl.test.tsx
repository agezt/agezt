// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";

const { getJSON, eventsRef } = vi.hoisted(() => ({
  getJSON: vi.fn(),
  eventsRef: { current: [] as { kind?: string; subject?: string; ts_unix_ms?: number }[] },
}));

vi.mock("@/app/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
}));

vi.mock("@/app/events", () => ({
  useEvents: () => ({ events: eventsRef.current, subscribe: () => () => {} }),
}));

import MissionControl from "./MissionControl";

const endpoint = (url: unknown): string => String(url);

function daemonOk(over: Record<string, unknown> = {}) {
  return (url: unknown) => {
    const u = endpoint(url as string);
    if (u === "/api/runs") return Promise.resolve(over.runs ?? { runs: [] });
    if (u === "/api/agents") return Promise.resolve(over.agents ?? { agents: [] });
    if (u === "/api/approvals") return Promise.resolve(over.approvals ?? { pending: 0 });
    if (u === "/api/status") return Promise.resolve(over.status ?? { version: "1.2.3" });
    if (u === "/api/spend/today") return Promise.resolve(over.spend ?? { total: 0 });
    if (u === "/api/attention") return Promise.resolve(over.attention ?? { items: [] });
    return Promise.resolve({});
  };
}

// Mission Control is the screen an operator opens to answer "is anything wrong",
// and it had no test. The last case is the one that matters most: every hook in
// this view swallows its fetch failure and leaves the previous value in place, so
// an unreachable daemon renders as a system with zeroes in it — which reads as a
// healthy system unless something says otherwise.
describe("MissionControl", () => {
  beforeEach(() => {
    getJSON.mockReset();
    eventsRef.current = [];
  });
  afterEach(cleanup);

  it("summarises the daemon's state across every endpoint it reads", async () => {
    getJSON.mockImplementation(
      daemonOk({
        runs: { runs: [{ status: "running" }, { status: "running" }, { status: "completed" }] },
        agents: { agents: [{}, {}, {}, {}] },
        approvals: { pending: 2 },
        status: { version: "9.9.9", revision: "abcdef1234567890" },
        spend: { total: 1.5 },
      }),
    );
    render(<MissionControl />);
    await waitFor(() => expect(screen.getByText("9.9.9")).toBeTruthy());
    // Scoped to each metric's own tile, and two levels up: the label span's
    // parent is the label row, whose SIBLING holds the value. A bare
    // getByText("2") would match whichever tile came first and prove nothing.
    const tile = (label: string) => screen.getByText(label).parentElement?.parentElement?.textContent;
    expect(tile("Runs in flight")).toContain("2");
    expect(tile("Agents")).toContain("4");
    expect(tile("Awaiting you")).toContain("2");
    expect(screen.getByText("abcdef12")).toBeTruthy(); // short revision
    expect(screen.getByText("$1.5000")).toBeTruthy();
  });

  it("omits the build row when the daemon reports no revision", async () => {
    getJSON.mockImplementation(daemonOk({ status: { version: "1.0.0" } }));
    render(<MissionControl />);
    await waitFor(() => expect(screen.getByText("1.0.0")).toBeTruthy());
    expect(screen.queryByText("Build")).toBeNull();
  });

  it("says the daemon is quiet when nothing has arrived in the window", async () => {
    getJSON.mockImplementation(daemonOk());
    render(<MissionControl />);
    expect(await screen.findByText(/No events in the last minute/i)).toBeTruthy();
  });

  it("says nothing needs eyes when the attention queue is empty", async () => {
    getJSON.mockImplementation(daemonOk());
    render(<MissionControl />);
    expect(await screen.findByText(/Nothing requires your eyes/i)).toBeTruthy();
  });

  it("lists what needs attention, and counts events per second", async () => {
    const t0 = Date.now();
    // Two events two seconds apart -> 1.00/sec, so the rate is not a stub zero.
    eventsRef.current = [
      { kind: "run.started", subject: "alpha", ts_unix_ms: t0 },
      { kind: "error.boom", subject: "beta", ts_unix_ms: t0 + 2000 },
    ];
    getJSON.mockImplementation(daemonOk({ attention: { items: [{ id: "a1", kind: "error", summary: "beta blew up", ts: t0 }] } }));
    render(<MissionControl />);
    expect(await screen.findByText("beta blew up")).toBeTruthy();
    await waitFor(() => expect(screen.getByText("1.00")).toBeTruthy());
    // The event tail shows the newest first, with its kind tone-coded.
    const errorBadge = screen.getByText("error.boom");
    expect(errorBadge.className).toContain("text-bad");
    expect(screen.getByText("run.started").className).toContain("text-accent");
  });

  it("drops events older than the 60s window", async () => {
    const t0 = Date.now();
    eventsRef.current = [
      { kind: "run.ancient", subject: "yesterday", ts_unix_ms: t0 - 10 * 60_000 },
      { kind: "run.fresh", subject: "now", ts_unix_ms: t0 },
    ];
    getJSON.mockImplementation(daemonOk());
    render(<MissionControl />);
    await waitFor(() => expect(screen.getByText("run.fresh")).toBeTruthy());
    expect(screen.queryByText("run.ancient")).toBeNull();
  });

  // The dangerous failure on a cockpit screen is not an error, it is a healthy-
  // looking zero. Every hook here catches and ignores, so an unreachable daemon
  // produced four zeroes and a sentence saying the system is running cleanly.
  it("does not report a clean system when the daemon cannot be reached", async () => {
    getJSON.mockRejectedValue(new Error("daemon unreachable"));
    render(<MissionControl />);
    expect(await screen.findByText(/could not be reached|unreachable/i)).toBeTruthy();
    expect(screen.queryByText(/Nothing requires your eyes/i)).toBeNull();
    expect(screen.queryByText(/running cleanly/i)).toBeNull();
  });

  it("surfaces a partial failure rather than silently showing zeros", async () => {
    getJSON.mockImplementation((url: unknown) => {
      if (endpoint(url) === "/api/runs") return Promise.reject(new Error("runs endpoint down"));
      return daemonOk()(url);
    });
    render(<MissionControl />);
    expect(await screen.findByText(/runs endpoint down/)).toBeTruthy();
  });
});