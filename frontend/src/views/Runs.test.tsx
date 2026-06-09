// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({ getJSON: (...a: unknown[]) => getJSON(...a) }));

import { Runs, runMatches } from "@/views/Runs";

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

describe("Runs filter (M775)", () => {
  const runs = [
    { correlation_id: "r1", status: "done", intent: "summarize the inbox" },
    { correlation_id: "r2", status: "failed", intent: "deploy the API" },
    { correlation_id: "r3", status: "done", intent: "write tests" },
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
