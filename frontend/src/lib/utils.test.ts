import { describe, it, expect } from "vitest";
import { clip, prettyJSON, fmtTime } from "@/lib/utils";

describe("clip", () => {
  it("truncates with an ellipsis past the limit", () => {
    expect(clip("hello world", 5)).toBe("hell…");
    expect(clip("hi", 5)).toBe("hi");
  });
  it("stringifies non-strings and guards nullish", () => {
    expect(clip(12345, 3)).toBe("12…");
    expect(clip(null, 5)).toBe("");
    expect(clip(undefined, 5)).toBe("");
  });
});

describe("prettyJSON", () => {
  it("indents valid JSON", () => {
    expect(prettyJSON('{"a":1}')).toBe('{\n  "a": 1\n}');
  });
  it("returns the input unchanged when it is not JSON", () => {
    expect(prettyJSON("not json")).toBe("not json");
    expect(prettyJSON("")).toBe("");
  });
});

describe("fmtTime", () => {
  it("returns empty for a falsy timestamp", () => {
    expect(fmtTime(0)).toBe("");
    expect(fmtTime(undefined)).toBe("");
  });
  it("formats a real epoch-ms to a non-empty string", () => {
    expect(fmtTime(1_700_000_000_000).length).toBeGreaterThan(0);
  });
});
