import { describe, it, expect, vi, beforeEach } from "vitest";

const getJSON = vi.fn();
const postJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
}));

import { parseConfigBundle, fetchConfigBundle, applyConfigBundle } from "@/lib/configbackup";

beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postJSON.mockResolvedValue({ saved: true });
});

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

  it("keeps an empty-string persona field (a deliberate 'no default identity' to restore)", () => {
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

describe("fetchConfigBundle", () => {
  it("gathers default identity + prompt templates + routing into a wrapped, versioned bundle", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/persona") return Promise.resolve({ system: "be terse" });
      if (path === "/api/prompts") return Promise.resolve({ prompts: [{ title: "t", text: "x" }] });
      if (path === "/api/routing") return Promise.resolve({ chains: { chat: ["a"] } });
      return Promise.resolve({});
    });
    const b = await fetchConfigBundle();
    expect(b).toEqual({
      version: 1,
      config: { persona: "be terse", prompts: [{ title: "t", text: "x" }], chains: { chat: ["a"] } },
    });
  });

  it("defaults missing sections to empties", async () => {
    getJSON.mockResolvedValue({});
    const b = await fetchConfigBundle();
    expect(b.config).toEqual({ persona: "", prompts: [], chains: {} });
  });
});

describe("applyConfigBundle", () => {
  it("posts each present section and reports what it applied", async () => {
    const applied = await applyConfigBundle({ persona: "hi", prompts: [{ title: "t", text: "x" }], chains: { chat: ["a"] } });
    expect(applied).toEqual(["default identity", "prompt templates", "routing"]);
    expect(postJSON).toHaveBeenCalledWith("/api/persona/set", { system: "hi" });
    expect(postJSON).toHaveBeenCalledWith("/api/prompts/set", { prompts: [{ title: "t", text: "x" }] });
    expect(postJSON).toHaveBeenCalledWith("/api/routing/set", { chains: { chat: ["a"] } });
  });

  it("only posts the sections the bundle carries", async () => {
    const applied = await applyConfigBundle({ persona: "" }); // empty persona is still applied
    expect(applied).toEqual(["default identity"]);
    expect(postJSON).toHaveBeenCalledTimes(1);
    expect(postJSON).toHaveBeenCalledWith("/api/persona/set", { system: "" });
  });
});
