import { describe, it, expect } from "vitest";
import { sparkPaths } from "@/components/Sparkline";

describe("sparkPaths", () => {
  it("returns a centered baseline for an empty series", () => {
    const { line, area } = sparkPaths([], 100, 20);
    expect(line).toBe("M0 10 L100 10");
    expect(area).toBe("");
  });

  it("maps the min to the bottom and max to the top (y inverted)", () => {
    const { line } = sparkPaths([0, 10], 100, 20, 1);
    // two points: x at 0 and 100; min(0)→bottom (h-pad=19), max(10)→top (pad=1)
    expect(line).toBe("M0.00 19.00 L100.00 1.00");
  });

  it("centers a single point and closes the area to the baseline", () => {
    const { line, area } = sparkPaths([5], 60, 20);
    expect(line.startsWith("M30.00")).toBe(true); // x = w/2
    expect(area.endsWith("Z")).toBe(true);
  });

  it("handles a flat series without dividing by zero", () => {
    const { line } = sparkPaths([3, 3, 3], 100, 20, 1);
    // span defaults to 1; all y equal — no NaN
    expect(line.includes("NaN")).toBe(false);
  });
});
