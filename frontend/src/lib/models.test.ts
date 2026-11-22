import { describe, it, expect } from "vitest";
import { flattenModels, filterModels, groupByProvider, pinnedOptions, fmtContext, findModelContext, modelHealth, type ModelCatalog } from "@/lib/models";

const cat: ModelCatalog = {
  providers: [
    {
      id: "openai",
      name: "OpenAI",
      credentialed: false,
      models: [
        { id: "gpt-4o", name: "GPT-4o", tool_call: true, reasoning: false, context: 128000, cost_input_usd_per_mtok: 5 },
        { id: "o3", name: "o3", tool_call: true, reasoning: true, context: 200000 },
      ],
    },
    {
      id: "deepseek",
      name: "DeepSeek",
      credentialed: true,
      models: [{ id: "deepseek-v4-pro", name: "DeepSeek V4 Pro", tool_call: true, context: 1000000 }],
    },
  ],
};

describe("flattenModels", () => {
  it("flattens every provider's models with capability + pricing", () => {
    const opts = flattenModels(cat);
    expect(opts).toHaveLength(3);
    const gpt = opts.find((o) => o.id === "gpt-4o")!;
    expect(gpt.providerName).toBe("OpenAI");
    expect(gpt.toolCall).toBe(true);
    expect(gpt.context).toBe(128000);
    expect(gpt.costInput).toBe(5);
  });

  it("returns empty for a null/empty catalog", () => {
    expect(flattenModels(null)).toEqual([]);
    expect(flattenModels({})).toEqual([]);
  });
});

describe("filterModels", () => {
  it("matches id/name/provider, case-insensitive, AND across terms", () => {
    const opts = flattenModels(cat);
    expect(filterModels(opts, "deepseek").map((o) => o.id)).toEqual(["deepseek-v4-pro"]);
    expect(filterModels(opts, "openai o3").map((o) => o.id)).toEqual(["o3"]);
    expect(filterModels(opts, "")).toHaveLength(3);
  });
});

describe("groupByProvider", () => {
  it("groups by provider and floats credentialed providers first", () => {
    const groups = groupByProvider(flattenModels(cat));
    expect(groups.map((g) => g.providerId)).toEqual(["deepseek", "openai"]);
    expect(groups[0].credentialed).toBe(true);
    expect(groups[1].options).toHaveLength(2);
  });
});

describe("fmtContext", () => {
  it("renders context windows compactly", () => {
    expect(fmtContext(128000)).toBe("128K");
    expect(fmtContext(1000000)).toBe("1M");
    expect(fmtContext(0)).toBe("");
  });
});

describe("findModelContext", () => {
  it("finds a model's window by id across providers", () => {
    expect(findModelContext(cat, "gpt-4o")).toBe(128000);
    expect(findModelContext(cat, "deepseek-v4-pro")).toBe(1000000);
  });
  it("returns 0 for unknown models, blank ids, and a missing catalog", () => {
    expect(findModelContext(cat, "nope")).toBe(0);
    expect(findModelContext(cat, "")).toBe(0);
    expect(findModelContext(null, "gpt-4o")).toBe(0);
  });
});

describe("pinnedOptions", () => {
  it("resolves ids in order, preferring a credentialed provider", () => {
    const multi: ModelCatalog = {
      providers: [
        { id: "unkeyed", credentialed: false, models: [{ id: "shared-model" }] },
        { id: "keyed", credentialed: true, models: [{ id: "shared-model" }, { id: "keyed-only" }] },
      ],
    };
    const out = pinnedOptions(flattenModels(multi), ["keyed-only", "shared-model"]);
    expect(out.map((o) => o.id)).toEqual(["keyed-only", "shared-model"]);
    expect(out[1].providerId).toBe("keyed"); // credentialed match wins
  });

  it("skips unknown ids and dedupes repeats", () => {
    const out = pinnedOptions(flattenModels(cat), ["gpt-4o", "not-in-catalog", "gpt-4o"]);
    expect(out.map((o) => o.id)).toEqual(["gpt-4o"]);
  });

  it("returns empty for an empty id list", () => {
    expect(pinnedOptions(flattenModels(cat), [])).toEqual([]);
  });
});

describe("modelHealth", () => {
  it("ok when a credentialed provider serves the model", () => {
    expect(modelHealth(cat, "deepseek-v4-pro")).toBe("ok");
  });
  it("nokey when only an uncredentialed provider has it", () => {
    expect(modelHealth(cat, "gpt-4o")).toBe("nokey");
  });
  it("unknown when no provider lists the id", () => {
    expect(modelHealth(cat, "made-up-model")).toBe("unknown");
    expect(modelHealth(cat, "")).toBe("unknown");
    expect(modelHealth(null, "gpt-4o")).toBe("unknown");
  });
  it("prefers ok if any serving provider is credentialed", () => {
    const multi: ModelCatalog = {
      providers: [
        { id: "unkeyed", credentialed: false, models: [{ id: "shared" }] },
        { id: "keyed", credentialed: true, models: [{ id: "shared" }] },
      ],
    };
    expect(modelHealth(multi, "shared")).toBe("ok");
  });
});
