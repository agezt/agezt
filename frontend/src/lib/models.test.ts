import { describe, it, expect } from "vitest";
import { flattenModels, filterModels, groupByProvider, pinnedOptions, fmtContext, findModelContext, modelHealth, type ModelCatalog } from "@/lib/models";
import type {
  CatalogListResponseProvidersItem,
  CatalogListResponseProvidersItemModelsItem,
} from "@/app/api/types.generated";

// model builds a COMPLETE catalog model row — the daemon always sends every
// field of a model, so a fixture that leaves one out would be a fixture the
// daemon cannot produce.
function model(
  over: Partial<CatalogListResponseProvidersItemModelsItem> = {},
): CatalogListResponseProvidersItemModelsItem {
  return {
    id: "m",
    name: "m",
    family: "",
    context: 0,
    output: 0,
    reasoning: false,
    tool_call: false,
    strict_tool_args: false,
    grammar_constrained_decoding: false,
    schema_constrained_decoding: false,
    ...over,
  };
}

// provider builds a COMPLETE catalog provider row — same rule as model().
function provider(
  over: Partial<CatalogListResponseProvidersItem> = {},
): CatalogListResponseProvidersItem {
  return {
    id: "p",
    name: "p",
    api: "",
    doc: "",
    family: "",
    credentialed: false,
    model_count: 0,
    env: [],
    models: [],
    ...over,
  };
}

const cat: ModelCatalog = {
  provider_count: 2,
  providers: [
    provider({
      id: "openai",
      name: "OpenAI",
      credentialed: false,
      models: [
        model({ id: "gpt-4o", name: "GPT-4o", tool_call: true, reasoning: false, context: 128000, cost_input_usd_per_mtok: 5 }),
        model({ id: "o3", name: "o3", tool_call: true, reasoning: true, context: 200000 }),
      ],
    }),
    provider({
      id: "deepseek",
      name: "DeepSeek",
      credentialed: true,
      models: [model({ id: "deepseek-v4-pro", name: "DeepSeek V4 Pro", tool_call: true, context: 1000000 })],
    }),
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
    expect(flattenModels({ provider_count: 0, providers: null })).toEqual([]);
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
      provider_count: 2,
      providers: [
        provider({ id: "unkeyed", credentialed: false, models: [model({ id: "shared-model" })] }),
        provider({ id: "keyed", credentialed: true, models: [model({ id: "shared-model" }), model({ id: "keyed-only" })] }),
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
      provider_count: 2,
      providers: [
        provider({ id: "unkeyed", credentialed: false, models: [model({ id: "shared" })] }),
        provider({ id: "keyed", credentialed: true, models: [model({ id: "shared" })] }),
      ],
    };
    expect(modelHealth(multi, "shared")).toBe("ok");
  });
});
