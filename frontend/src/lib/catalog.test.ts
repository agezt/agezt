import { describe, it, expect } from "vitest";
import { joinCatalog, levelTone } from "@/lib/catalog";

describe("joinCatalog", () => {
  const tools = [
    { name: "shell", description: "run a command", capability: "shell" },
    { name: "web_search", description: "search the web", capability: "web.search" },
    { name: "memory", description: "remember", capability: "memory" },
  ];
  const levels = { shell: "L2", "web.search": "L2", memory: "L4" };
  const byTool = { shell: { calls: 10, errors: 2 }, web_search: { calls: 3 } };

  it("joins capability → level and usage by tool name", () => {
    const rows = joinCatalog(tools, levels, byTool);
    const shell = rows.find((r) => r.name === "shell")!;
    expect(shell.level).toBe("L2");
    expect(shell.calls).toBe(10);
    expect(shell.errors).toBe(2);
    const mem = rows.find((r) => r.name === "memory")!;
    expect(mem.level).toBe("L4");
    expect(mem.calls).toBe(0); // no usage row → zero
  });

  it("returns rows name-sorted", () => {
    const rows = joinCatalog(tools, levels, byTool);
    expect(rows.map((r) => r.name)).toEqual(["memory", "shell", "web_search"]);
  });

  it("leaves level empty when the capability isn't in the policy map", () => {
    const rows = joinCatalog([{ name: "x", capability: "unknown.cap" }], levels, {});
    expect(rows[0].level).toBe("");
  });

  it("is safe with missing levels/usage", () => {
    const rows = joinCatalog(tools, undefined, undefined);
    expect(rows).toHaveLength(3);
    expect(rows[0].calls).toBe(0);
    expect(rows[0].level).toBe("");
  });
});

describe("levelTone", () => {
  it("greens L4, reds L0, warns L1, neutral otherwise", () => {
    expect(levelTone("L4")).toContain("good");
    expect(levelTone("L0")).toContain("bad");
    expect(levelTone("L1")).toContain("warn");
    expect(levelTone("L2")).toContain("muted");
    expect(levelTone("")).toContain("muted");
  });
});
