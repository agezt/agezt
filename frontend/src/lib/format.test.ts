import { describe, it, expect } from "vitest";
import { money, pct, byDescValue } from "@/lib/format";

describe("money", () => {
  it("renders microcents (1e-9 USD) as a 4-dp dollar string", () => {
    expect(money(0)).toBe("$0.0000");
    expect(money(1_000_000)).toBe("$0.0010"); // 1e6 microcents = $0.001
    expect(money(5_000_000_000)).toBe("$5.0000");
    expect(money(undefined)).toBe("$0.0000");
  });
});

describe("pct", () => {
  it("formats a 0..1 rate as an integer percent", () => {
    expect(pct(0.5)).toBe("50%");
    expect(pct(0.333)).toBe("33%");
    expect(pct(undefined)).toBe("0%");
  });
  it("returns an em dash when the denominator is zero", () => {
    expect(pct(0.5, 0)).toBe("—");
    expect(pct(0.5, 3)).toBe("50%");
  });
});

describe("byDescValue", () => {
  it("sorts keys by descending numeric value", () => {
    expect(byDescValue({ a: 1, b: 9, c: 5 })).toEqual(["b", "c", "a"]);
  });
  it("handles an empty object", () => {
    expect(byDescValue({})).toEqual([]);
  });
});
