import { describe, expect, it } from "vitest";
import { agentHue, initials } from "@/lib/agent";

describe("agent identity helpers", () => {
  it("maps a slug to a stable hue range", () => {
    const a = agentHue("planner");
    expect(a).toBe(agentHue("planner"));
    expect(a).toBeGreaterThanOrEqual(0);
    expect(a).toBeLessThan(360);
  });

  it("derives initials from names or slug fallback", () => {
    expect(initials("Ada Lovelace", "agent-1")).toBe("AL");
    expect(initials("Scout", "agent-1")).toBe("SC");
    expect(initials(undefined, "rx-worker")).toBe("RX");
  });
});
