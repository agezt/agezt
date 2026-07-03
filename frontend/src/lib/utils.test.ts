import { describe, it, expect, beforeEach, vi } from "vitest";
import { clip, prettyJSON, fmtTime, __resetPrettyJSONCacheForTest } from "@/lib/utils";

beforeEach(() => {
  // The cache lives at module scope; reset between tests so the cache
  // assertions below are deterministic and don't leak into one another.
  __resetPrettyJSONCacheForTest();
});

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

  // S2.4: cache layer — hits on the second call skip the parse +
  // re-format pass, which is the whole point of this cache. We assert
  // by spying on JSON.parse the second call must not invoke it again.
  it("memoises pretty-printed output (no second JSON.parse on cache hit)", () => {
    const input = '{"a":1,"b":[2,3,4],"c":null}';
    // Reset the spy counter so the second call's hit is unambiguous.
    const parseSpy = vi.spyOn(JSON, "parse");
    parseSpy.mockClear();
    prettyJSON(input);
    expect(parseSpy).toHaveBeenCalledTimes(1);
    parseSpy.mockClear();
    const cached = prettyJSON(input);
    expect(cached).toBe(prettyJSON(input));
    expect(parseSpy).not.toHaveBeenCalled();
    parseSpy.mockRestore();
  });

  it("caches non-JSON inputs too (the 'return original' path)", () => {
    // A non-JSON input would otherwise re-string-pass on every call.
    const input = "definitely not json!";
    const stringifySpy = vi.spyOn(JSON, "stringify");
    stringifySpy.mockClear();
    prettyJSON(input);
    expect(stringifySpy).not.toHaveBeenCalled();
    // Second call must not stringify either (cache hit on the fallback path).
    stringifySpy.mockClear();
    prettyJSON(input);
    expect(stringifySpy).not.toHaveBeenCalled();
    stringifySpy.mockRestore();
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
