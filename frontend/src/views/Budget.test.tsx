// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const postAction = vi.fn();
const reload = vi.fn();
let panelData: any = null;

vi.mock("@/lib/api", () => ({
  postAction: (...a: unknown[]) => postAction(...a),
}));

vi.mock("@/lib/usePanel", () => ({
  usePanel: () => ({ data: panelData, error: null, loading: false, reload }),
}));

import { Budget, projectedDailySpend } from "@/views/Budget";

const dayMs = 24 * 60 * 60 * 1000;
// A UTC midnight timestamp to anchor "fraction of day" math.
const midnight = Math.floor(Date.now() / dayMs) * dayMs;

afterEach(cleanup);
beforeEach(() => {
  postAction.mockReset();
  postAction.mockResolvedValue({});
  reload.mockReset();
  panelData = {
    utc_date: "2026-06-28",
    spent_mc: 1_000_000_000,
    ceiling_mc: 5_000_000_000,
    strict_pricing: true,
    per_task: [],
  };
});

describe("projectedDailySpend (M920)", () => {
  it("extrapolates spend to end-of-day from the fraction elapsed", () => {
    // Quarter of the day in, $1 spent → projects ~$4.
    const q = midnight + dayMs / 4;
    expect(projectedDailySpend(1_000_000_000, q)).toBe(4_000_000_000);
    // Half the day in, $2 → ~$4.
    expect(projectedDailySpend(2_000_000_000, midnight + dayMs / 2)).toBe(4_000_000_000);
  });

  it("returns null too early in the day (noise) and for the first hour", () => {
    expect(projectedDailySpend(500_000_000, midnight + 10 * 60_000)).toBeNull(); // 10 min in
    expect(projectedDailySpend(500_000_000, midnight + 30 * 60_000)).toBeNull(); // 30 min in
  });

  it("projects from ~1h in once there's signal", () => {
    const oneHour = midnight + 60 * 60_000 + 60_000; // just past the 0.04 threshold
    expect(projectedDailySpend(1_000_000_000, oneHour)).toBeGreaterThan(1_000_000_000);
  });
});

describe("Budget view", () => {
  it("keeps the daily ceiling editor in a modal and posts the runtime cap", async () => {
    render(<Budget />);

    expect(screen.queryByLabelText("Daily ceiling dollars")).toBeNull();
    fireEvent.click(screen.getAllByRole("button", { name: /Adjust/ })[0]);
    expect(screen.getByRole("dialog", { name: "Adjust daily ceiling" })).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Daily ceiling dollars"), { target: { value: "12.5" } });
    fireEvent.click(screen.getByRole("button", { name: /Set ceiling/ }));

    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/budget_set", { ceiling_mc: "12500000000" }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Adjust daily ceiling" })).toBeNull());
  });
});
