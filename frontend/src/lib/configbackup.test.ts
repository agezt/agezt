import { describe, it, expect } from "vitest";
import { parseConfigBundle } from "@/lib/configbackup";

describe("parseConfigBundle", () => {
  it("reads a bare object with all three sections", () => {
    expect(
      parseConfigBundle('{"persona":"be terse","prompts":[{"title":"x","body":"y"}],"chains":{"chat":["a","b"]}}'),
    ).toEqual({
      persona: "be terse",
      prompts: [{ title: "x", body: "y" }],
      chains: { chat: ["a", "b"] },
    });
  });

  it("unwraps the {config:{…}} export wrapper", () => {
    expect(parseConfigBundle('{"version":1,"config":{"persona":"hi"}}')).toEqual({ persona: "hi" });
  });

  it("keeps an empty-string persona (a deliberate 'no persona' to restore)", () => {
    expect(parseConfigBundle('{"persona":""}')).toEqual({ persona: "" });
  });

  it("normalises chains: drops non-array values, blanks, and empty chains; trims", () => {
    expect(parseConfigBundle('{"chains":{" chat ":["  a ","",3,"b"],"plan":[],"x":"y"}}')).toEqual({
      chains: { chat: ["a", "b"] },
    });
  });

  it("ignores a non-array prompts and an array chains", () => {
    // prompts must be an array; chains must be an object — wrong-typed sections are dropped,
    // and with nothing valid left it throws.
    expect(() => parseConfigBundle('{"prompts":"nope","chains":["a"]}')).toThrow(/no valid config sections/);
  });

  it("throws on invalid JSON, non-object, or no usable section", () => {
    expect(() => parseConfigBundle("nope")).toThrow();
    expect(() => parseConfigBundle("[1,2]")).toThrow(/expected a config object/);
    expect(() => parseConfigBundle('{"junk":1}')).toThrow(/no valid config sections/);
  });
});
