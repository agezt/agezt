// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { projectedDailySpend } from "@/views/Budget";

const dayMs = 24 * 60 * 60 * 1000;
// A UTC midnight timestamp to anchor "fraction of day" math.
const midnight = Math.floor(Date.now() / dayMs) * dayMs;

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
