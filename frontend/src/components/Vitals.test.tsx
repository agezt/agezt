// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

const { getJSON, onNavigateSpy } = vi.hoisted(() => ({
  getJSON: vi.fn(),
  onNavigateSpy: vi.fn(),
}));

vi.mock("@/app/api", () => ({ getJSON: (...a: unknown[]) => getJSON(...a) }));

import { Vitals } from "./Vitals";

// The spend tile used to be `onClick={() => onNavigate("budget")}` for a view
// retired with no live equivalent. `budget` is not in VIEW_ALIASES, so the click
// wrote `#budget`, viewFromHash fell through its `|| "mission"`, and the
// operator landed on Mission Control from a button whose title said
// "Go to today". The number still mattered; the destination never existed.
describe("Vitals", () => {
  beforeEach(() => {
    getJSON.mockReset();
    // money() is `"$" + (mc / 1e9).toFixed(4)` — 1.5 USD is 1_500_000_000.
    getJSON.mockResolvedValue({
      running: 0,
      spent_mc: 1_500_000_000,
      schedules_enabled: 2,
      active_skills: 3,
      pending_approvals: 0,
    });
    onNavigateSpy.mockReset();
  });
  afterEach(cleanup);

  it("renders the spend tile as a readout, not a link to a retired view", async () => {
    render(<Vitals onNavigate={onNavigateSpy} />);
    const tile = await screen.findByText("today");
    const control = tile.closest("button");
    expect(control, "the spend tile must not be a button — there is nowhere to go").toBeNull();
    // And nothing in the bar advertises a trip to `budget`.
    expect(screen.queryByTitle(/Go to today/)).toBeNull();
  });

  it("still renders the spend number", async () => {
    render(<Vitals onNavigate={onNavigateSpy} />);
    await screen.findByText("today");
    expect(screen.getByText("$1.5000")).toBeTruthy();
  });

  // The other tiles are real destinations and must stay clickable, or the
  // change above would have quietly turned the whole bar inert.
  it("keeps the destinations that do exist clickable", async () => {
    render(<Vitals onNavigate={onNavigateSpy} />);
    const runs = await screen.findByTitle("Go to running");
    expect(runs.tagName).toBe("BUTTON");
    runs.click();
    expect(onNavigateSpy).toHaveBeenCalledWith("activity");

    const skills = screen.getByTitle("Go to skills");
    expect(skills.tagName).toBe("BUTTON");
  });
});