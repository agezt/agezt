// @vitest-environment jsdom
// Backup.tsx → lib/configbackup → lib/api touches `location` at import; jsdom provides it.
import { describe, it, expect } from "vitest";
import { configSummary } from "@/views/Backup";

describe("configSummary", () => {
  it("summarises a populated config", () => {
    expect(
      configSummary({ persona: "be terse", prompts: [{}, {}], chains: { chat: ["a"], code: ["b"] } }),
    ).toBe("default identity set · 2 prompt templates · 2 routing chains");
  });

  it("reports the empty state and singular nouns", () => {
    expect(configSummary({ persona: "  ", prompts: [{}], chains: { chat: ["a"] } })).toBe(
      "no default identity · 1 prompt template · 1 routing chain",
    );
    expect(configSummary({ persona: "", prompts: [], chains: {} })).toBe("no default identity · 0 prompt templates · 0 routing chains");
  });
});
