import { describe, it, expect } from "vitest";
import { PROVIDER_PRESETS, familyNpm } from "@/lib/providerPresets";

describe("providerPresets", () => {
  it("every preset has the fields the connect flow needs", () => {
    for (const p of PROVIDER_PRESETS) {
      expect(p.id, p.name).toMatch(/^[a-z0-9-]+$/);
      expect(p.api, p.id).toMatch(/^https:\/\//);
      expect(p.keyEnv, p.id).toMatch(/^[A-Z][A-Z0-9_]*$/);
      expect(p.model.length, p.id).toBeGreaterThan(0);
      expect(p.signupUrl, p.id).toMatch(/^https:\/\//);
      expect(["openai-compatible", "anthropic"]).toContain(p.family);
      expect(["coding", "popular"]).toContain(p.category);
    }
  });

  it("ids are unique", () => {
    const ids = PROVIDER_PRESETS.map((p) => p.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("maps family to the catalog npm hint", () => {
    expect(familyNpm("openai-compatible")).toBe("@ai-sdk/openai-compatible");
    expect(familyNpm("anthropic")).toBe("@ai-sdk/anthropic");
  });

  it("ships the owner's named coding-plan providers", () => {
    const vendors = new Set(PROVIDER_PRESETS.map((p) => p.vendor));
    for (const v of ["zai", "minimax", "moonshot", "mimo", "deepseek", "opencode"]) {
      expect(vendors, v).toContain(v);
    }
  });
});
