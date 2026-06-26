// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

import type { AgentEvent } from "@/lib/events";

const getJSON = vi.fn();
let liveEvents: AgentEvent[] = [];
vi.mock("@/lib/api", () => ({ getJSON: (...a: unknown[]) => getJSON(...a) }));
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ events: liveEvents, connected: true, subscribe: () => () => {} }),
}));

import { Runs, runMatches, runBucket, runCounts } from "@/views/Runs";

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  liveEvents = [];
});

describe("runMatches (M775)", () => {
  it("matches on intent, status, or correlation id (case-insensitive)", () => {
    const r = { intent: "Deploy the API", status: "failed", correlation_id: "abc123" };
    expect(runMatches(r, "deploy")).toBe(true);
    expect(runMatches(r, "failed")).toBe(true);
    expect(runMatches(r, "abc123")).toBe(true);
    expect(runMatches(r, "running")).toBe(false);
    expect(runMatches(r, "")).toBe(true);
  });
});

describe("runBucket / runCounts (M923)", () => {
  it("buckets statuses, counting abandoned as failed and unknown as other", () => {
    expect(runBucket({ status: "running" })).toBe("running");
    expect(runBucket({ status: "Completed" })).toBe("completed"); // case-insensitive
    expect(runBucket({ status: "failed" })).toBe("failed");
    expect(runBucket({ status: "abandoned" })).toBe("failed");
    expect(runBucket({ status: "queued" })).toBe("other");
    expect(runBucket({})).toBe("other");
  });

  it("tallies a distribution whose buckets sum to the total", () => {
    const c = runCounts([
      { status: "completed" },
      { status: "completed" },
      { status: "failed" },
      { status: "abandoned" },
      { status: "running" },
      { status: "queued" },
    ]);
    expect(c).toEqual({ total: 6, running: 1, completed: 2, failed: 2, other: 1 });
    expect(c.running + c.completed + c.failed + c.other).toBe(c.total);
  });
});

describe("Runs filter (M775)", () => {
  const runs = [
    { correlation_id: "r1", status: "completed", intent: "summarize the inbox" },
    { correlation_id: "r2", status: "failed", intent: "deploy the API" },
    { correlation_id: "r3", status: "completed", intent: "write tests" },
    { correlation_id: "r4", status: "running", intent: "research pricing" },
    { correlation_id: "r5", status: "failed", intent: "build the frontend" },
  ];

  it("filters runs and shows a match count, only once the list grows past four", async () => {
    getJSON.mockResolvedValue({ runs });
    render(<Runs />);
    const input = await screen.findByLabelText("Filter runs");
    // The count badge (a span beside the input) only appears once a query is entered, and now
    // shows just the live match count — the redesign dropped the "/total" suffix (Runs.tsx RunList).
    const badge = () => input.parentElement!.querySelector("span.absolute.right-2");
    expect(badge()).toBeNull();
    // "failed" surfaces the two failed runs.
    fireEvent.change(input, { target: { value: "failed" } });
    await waitFor(() => expect(badge()?.textContent).toBe("2"));
    expect(screen.getByText("deploy the API")).toBeTruthy();
    expect(screen.queryByText("write tests")).toBeNull();
    // A non-matching query shows the "no runs match" empty state (filter stays mounted).
    fireEvent.change(input, { target: { value: "zzz" } });
    await waitFor(() => expect(screen.getByText(/No runs match/i)).toBeTruthy());
  });

  it("does not show the filter when there are four or fewer runs", async () => {
    getJSON.mockResolvedValue({ runs: runs.slice(0, 3) });
    render(<Runs />);
    await waitFor(() => expect(screen.getByText("write tests")).toBeTruthy());
    expect(screen.queryByLabelText("Filter runs")).toBeNull();
  });

  it("shows a live indicator wired to the event stream (M-live-runs)", async () => {
    getJSON.mockResolvedValue({ runs });
    render(<Runs />);
    // The events mock reports connected:true, so the header pill reads "live".
    expect(await screen.findByText("live")).toBeTruthy();
  });

  it("surfaces a running run's live phase + tool from the event stream (M-live-phase)", async () => {
    const corr = "run-live-1";
    // Stream is newest-first; the fold reverses it, so the latest event (tool.invoked) wins the phase.
    liveEvents = [
      { kind: "tool.invoked", correlation_id: corr, actor: "research-agent", payload: { tool: "web_search" } },
      { kind: "task.received", correlation_id: corr, actor: "research-agent", payload: { intent: "research" } },
    ] as AgentEvent[];
    getJSON.mockResolvedValue({ runs: [{ correlation_id: corr, status: "running", intent: "research pricing" }] });
    render(<Runs />);
    expect(await screen.findByText("using tool · web_search")).toBeTruthy();
  });
});

// The status-chip row (M923) was replaced by an outcome TabNav (role="tab", count badge in
// the accessible name). Tabs don't toggle off — the "All" tab restores the full list — and an
// empty bucket renders a zero-count tab with an empty panel rather than a disabled chip.
// Radix Tabs activate on pointer-down, not a bare click, under jsdom.
const selectTab = (tab: HTMLElement) => {
  fireEvent.pointerDown(tab, { button: 0, ctrlKey: false });
  fireEvent.mouseDown(tab, { button: 0 });
  fireEvent.click(tab);
};

describe("Runs status tabs (M923 → TabNav)", () => {
  const runs = [
    { correlation_id: "r1", status: "completed", intent: "summarize the inbox" },
    { correlation_id: "r2", status: "failed", intent: "deploy the API" },
    { correlation_id: "r3", status: "completed", intent: "write tests" },
    { correlation_id: "r4", status: "running", intent: "research pricing" },
    { correlation_id: "r5", status: "failed", intent: "build the frontend" },
  ];

  it("narrows the list to one outcome when a tab is selected, and All restores it", async () => {
    getJSON.mockResolvedValue({ runs });
    render(<Runs />);
    const failedTab = await screen.findByRole("tab", { name: /failed\s*2/i });
    selectTab(failedTab);
    // Only the two failed runs remain visible (inactive panels unmount).
    await waitFor(() => expect(screen.queryByText("write tests")).toBeNull());
    expect(screen.getByText("deploy the API")).toBeTruthy();
    expect(screen.getByText("build the frontend")).toBeTruthy();
    expect(failedTab.getAttribute("aria-selected")).toBe("true");
    // The All tab restores the full list.
    selectTab(screen.getByRole("tab", { name: /all\s*5/i }));
    await waitFor(() => expect(screen.getByText("write tests")).toBeTruthy());
  });

  it("shows a zero-count tab with an empty panel for a bucket with no runs", async () => {
    // No running runs → the Running tab carries a 0 count and selects into an empty state.
    getJSON.mockResolvedValue({ runs: runs.filter((r) => r.status !== "running") });
    render(<Runs />);
    const runningTab = await screen.findByRole("tab", { name: /running\s*0/i });
    selectTab(runningTab);
    await waitFor(() => expect(screen.getByText(/No runs yet/i)).toBeTruthy());
  });
});
