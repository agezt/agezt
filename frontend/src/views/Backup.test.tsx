// @vitest-environment jsdom
// Backup.tsx → lib/configbackup → lib/api touches `location` at import; jsdom provides it.
import { describe, it, expect } from "vitest";
import { configSummary } from "@/views/Backup";

describe("configSummary", () => {
  it("summarises a populated config", () => {
    expect(
      configSummary({ persona: "be terse", prompts: [{}, {}], chains: { chat: ["a"], code: ["b"] } }),
    ).toBe("persona set · 2 prompts · 2 routing chains");
  });

  it("reports the empty state and singular nouns", () => {
    expect(configSummary({ persona: "  ", prompts: [{}], chains: { chat: ["a"] } })).toBe(
      "no persona · 1 prompt · 1 routing chain",
    );
    expect(configSummary({ persona: "", prompts: [], chains: {} })).toBe("no persona · 0 prompts · 0 routing chains");
  });
});
