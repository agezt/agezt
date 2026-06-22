import { describe, expect, it } from "vitest";
import { acpCensus, acpUsageHint, type ACPInventory } from "@/lib/acp";

describe("acp helpers", () => {
  it("counts installed and missing agents", () => {
    const inv: ACPInventory = {
      os: "linux",
      agents: [
        { slug: "codex", name: "Codex", bin: "codex", command: "codex", description: "", installed: true, active: true },
        { slug: "gemini", name: "Gemini", bin: "gemini", command: "gemini", description: "", installed: false, active: false },
      ],
      installed_count: 1,
      missing_count: 1,
    };
    expect(acpCensus(inv)).toEqual({ total: 2, installed: 1, missing: 1 });
    expect(acpCensus(null)).toEqual({ total: 0, installed: 0, missing: 0 });
  });

  it("explains active, installed, and missing usage", () => {
    expect(acpUsageHint({ slug: "codex", name: "Codex", bin: "codex", command: "codex", description: "", installed: true, active: true })).toContain("Default ACP agent");
    expect(acpUsageHint({ slug: "gemini", name: "Gemini", bin: "gemini", command: "gemini", description: "", installed: true, active: false })).toContain('agent="gemini"');
    expect(acpUsageHint({ slug: "claude", name: "Claude", bin: "claude", command: "claude", description: "", installed: false, active: false, install: "npm i -g claude" })).toBe("Install: npm i -g claude");
  });
});
