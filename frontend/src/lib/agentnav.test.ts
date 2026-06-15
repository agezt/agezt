// @vitest-environment jsdom
import { describe, it, expect, beforeEach } from "vitest";
import { openAgent, agentSlugFromHash, AGENT_HASH_PREFIX } from "@/lib/agentnav";

describe("agentnav", () => {
  beforeEach(() => {
    location.hash = "";
  });

  it("agentSlugFromHash extracts a slug from an agent route", () => {
    expect(agentSlugFromHash("#agent/researcher")).toBe("researcher");
    expect(agentSlugFromHash("#/agent/researcher")).toBe("researcher");
    expect(agentSlugFromHash("agent/researcher")).toBe("researcher");
  });

  it("returns null for ordinary nav views and blank selections", () => {
    expect(agentSlugFromHash("#agents")).toBeNull();
    expect(agentSlugFromHash("#chat")).toBeNull();
    expect(agentSlugFromHash("#agent/")).toBeNull();
    expect(agentSlugFromHash("")).toBeNull();
  });

  it("round-trips a slug needing url-encoding", () => {
    const slug = "team.lead-1";
    openAgent(slug);
    expect(location.hash).toContain(AGENT_HASH_PREFIX);
    expect(agentSlugFromHash(location.hash)).toBe(slug);
  });

  it("openAgent ignores a blank slug", () => {
    location.hash = "agents";
    openAgent("");
    expect(location.hash).toBe("#agents");
  });

  it("tolerates a malformed escape without throwing", () => {
    expect(agentSlugFromHash("#agent/%E0%A4%A")).toBeTruthy();
  });
});
