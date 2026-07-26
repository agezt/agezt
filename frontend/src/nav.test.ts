import { describe, expect, it } from "vitest";
import { NAV, NAV_GROUPS, groupForView, sectionForView } from "@/nav";

describe("operator-job navigation", () => {
  it("uses the eight cockpit jobs in a stable order", () => {
    expect(NAV_GROUPS.map((group) => group.label)).toEqual([
      "Talk",
      "Observe",
      "Automate",
      "Govern",
      "Knowledge",
      "Connect",
      "Build",
      "Admin",
    ]);
  });

  it("assigns every view exactly once and keeps groups scannable", () => {
    const ids = NAV_GROUPS.flatMap((group) => group.items.map((item) => item.id));
    expect(new Set(ids).size).toBe(ids.length);
    expect(ids).toEqual(NAV.map((item) => item.id));
    expect(Math.max(...NAV_GROUPS.map((group) => group.items.length))).toBeLessThanOrEqual(10);
  });

  it("maps representative tasks to operator language", () => {
    expect(groupForView.chat).toBe("talk");
    expect(groupForView.runs).toBe("observe");
    expect(groupForView.schedules).toBe("automate");
    expect(groupForView.policy).toBe("govern");
    expect(groupForView.memory).toBe("knowledge");
    expect(groupForView.providers).toBe("connect");
    expect(groupForView.toolforge).toBe("build");
    expect(groupForView.configcenter).toBe("admin");

    expect(sectionForView.policy).toBe("Govern");
    expect(sectionForView.providers).toBe("Connect");
  });
});
