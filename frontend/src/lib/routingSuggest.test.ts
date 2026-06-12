import { describe, expect, it } from "vitest";
import { suggestChains, SUGGEST_CHAIN_MAX } from "./routingSuggest";
import { type ModelCatalog } from "./models";

// A small keyed-vs-unkeyed catalog: two credentialed providers (one strong,
// one cheap), one provider alias serving the same model id, one unkeyed
// provider, and non-chat models (embeddings) that must never be suggested.
const cat: ModelCatalog = {
  providers: [
    {
      id: "strong",
      name: "Strong",
      credentialed: true,
      models: [
        { id: "strong-big", tool_call: true, reasoning: false, context: 200000, cost_output_usd_per_mtok: 20 },
        { id: "strong-mini", tool_call: true, reasoning: false, context: 64000, cost_output_usd_per_mtok: 1 },
        { id: "strong-think", tool_call: true, reasoning: true, context: 200000, cost_output_usd_per_mtok: 10 },
        { id: "strong-embed-1", tool_call: false, context: 8192, cost_output_usd_per_mtok: 0.1 },
      ],
    },
    {
      id: "cheap",
      name: "Cheap",
      credentialed: true,
      models: [{ id: "cheap-turbo", tool_call: true, reasoning: false, context: 32000, cost_output_usd_per_mtok: 0.5 }],
    },
    {
      id: "strong-alias",
      name: "Strong (alias endpoint)",
      credentialed: true,
      models: [{ id: "strong-big", tool_call: true, reasoning: false, context: 200000, cost_output_usd_per_mtok: 20 }],
    },
    {
      id: "unkeyed",
      name: "Unkeyed",
      credentialed: false,
      models: [{ id: "unkeyed-best", tool_call: true, context: 1000000, cost_output_usd_per_mtok: 99 }],
    },
  ],
};

describe("suggestChains", () => {
  it("builds one chain per task from credentialed providers only", () => {
    const out = suggestChains(cat, ["chat", "summarize"]);
    expect(Object.keys(out).sort()).toEqual(["chat", "summarize"]);
    for (const chain of Object.values(out)) {
      expect(chain).not.toContain("unkeyed-best");
    }
  });

  it("orders heavy tasks strongest-first and light tasks cheapest-first", () => {
    const out = suggestChains(cat, ["chat", "summarize"]);
    expect(out.chat[0]).toBe("strong-big"); // highest output price = strength proxy
    expect(out.summarize[0]).toBe("cheap-turbo"); // cheapest per-provider pick wins
  });

  it("prefers reasoning models for plan", () => {
    const out = suggestChains(cat, ["plan"]);
    expect(out.plan[0]).toBe("strong-think");
  });

  it("dedupes a model served by provider aliases and never suggests non-chat models", () => {
    const out = suggestChains(cat, ["chat"]);
    expect(out.chat.filter((m) => m === "strong-big")).toHaveLength(1);
    expect(out.chat.join(" ")).not.toMatch(/embed/);
    expect(out.chat.length).toBeLessThanOrEqual(SUGGEST_CHAIN_MAX);
  });

  it("returns an empty object when nothing is keyed", () => {
    const none: ModelCatalog = { providers: [{ id: "x", credentialed: false, models: [{ id: "m" }] }] };
    expect(suggestChains(none, ["chat"])).toEqual({});
  });
});
