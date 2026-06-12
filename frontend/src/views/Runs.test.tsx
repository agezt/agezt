// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({ getJSON: (...a: unknown[]) => getJSON(...a) }));

import { Runs, runMatches, runBucket, runCounts } from "@/views/Runs";

afterEach(cleanup);
beforeEach(() => getJSON.mockReset());

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
    // No count chip until a query is entered.
    expect(screen.queryByText("2/5")).toBeNull();
    // "failed" surfaces the two failed runs.
    fireEvent.change(input, { target: { value: "failed" } });
    await waitFor(() => expect(screen.getByText("2/5")).toBeTruthy());
    expect(screen.getByText("deploy the API")).toBeTruthy();
    expect(screen.queryByText("write tests")).toBeNull();
    // A non-matching query shows the empty hint.
    fireEvent.change(input, { target: { value: "zzz" } });
    await waitFor(() => expect(screen.getByText(/no runs match/)).toBeTruthy());
  });

  it("does not show the filter when there are four or fewer runs", async () => {
    getJSON.mockResolvedValue({ runs: runs.slice(0, 3) });
    render(<Runs />);
    await waitFor(() => expect(screen.getByText("write tests")).toBeTruthy());
    expect(screen.queryByLabelText("Filter runs")).toBeNull();
  });
});

describe("Runs status chips (M923)", () => {
  const runs = [
    { correlation_id: "r1", status: "completed", intent: "summarize the inbox" },
    { correlation_id: "r2", status: "failed", intent: "deploy the API" },
    { correlation_id: "r3", status: "completed", intent: "write tests" },
    { correlation_id: "r4", status: "running", intent: "research pricing" },
    { correlation_id: "r5", status: "failed", intent: "build the frontend" },
  ];

  it("narrows the list to one outcome when a chip is clicked, and clears on re-click", async () => {
    getJSON.mockResolvedValue({ runs });
    render(<Runs />);
    const failedChip = await screen.findByRole("button", { name: /failed 2/ });
    fireEvent.click(failedChip);
    // Only the two failed runs remain visible.
    await waitFor(() => expect(screen.queryByText("write tests")).toBeNull());
    expect(screen.getByText("deploy the API")).toBeTruthy();
    expect(screen.getByText("build the frontend")).toBeTruthy();
    expect(failedChip.getAttribute("aria-pressed")).toBe("true");
    // Clicking the active chip again restores the full list.
    fireEvent.click(failedChip);
    await waitFor(() => expect(screen.getByText("write tests")).toBeTruthy());
  });

  it("disables a chip whose bucket has no runs", async () => {
    getJSON.mockResolvedValue({ runs });
    render(<Runs />);
    // None of these runs are "other".
    const otherChip = await screen.findByRole("button", { name: /other 0/ });
    expect(otherChip).toHaveProperty("disabled", true);
  });
});
