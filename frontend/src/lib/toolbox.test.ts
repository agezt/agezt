// @vitest-environment jsdom
// toolbox.ts pulls in api.ts (streamInstall → withToken), which reads location at
// module load — jsdom provides it. The pure filter/census logic under test is
// environment-independent (mirrors chat.test.ts).
import { describe, it, expect } from "vitest";
import { filterTools, census, categoriesPresent, CATEGORY_LABELS, type ToolStatus } from "@/lib/toolbox";

const TOOLS: ToolStatus[] = [
  { name: "jq", category: "data", description: "JSON processor", installed: true, version: "jq-1.7", installable: true, manager: "winget", command: "winget install jq" },
  { name: "rg", category: "search", description: "ripgrep fast search", installed: false, installable: true, manager: "winget", command: "winget install ripgrep" },
  { name: "ffmpeg", category: "media", description: "video tool", installed: false, installable: false },
  { name: "git", category: "vcs", description: "version control", installed: true, version: "2.54", installable: true },
];

describe("filterTools", () => {
  it("category filter keeps only that category", () => {
    expect(filterTools(TOOLS, "search", "").map((t) => t.name)).toEqual(["rg"]);
  });
  it("installed/missing status filters", () => {
    expect(filterTools(TOOLS, "installed", "").map((t) => t.name).sort()).toEqual(["git", "jq"]);
    expect(filterTools(TOOLS, "missing", "").map((t) => t.name).sort()).toEqual(["ffmpeg", "rg"]);
  });
  it("all + search over name/description/manager", () => {
    expect(filterTools(TOOLS, "all", "ripgrep").map((t) => t.name)).toEqual(["rg"]);
    expect(filterTools(TOOLS, "all", "json").map((t) => t.name)).toEqual(["jq"]);
    expect(filterTools(TOOLS, "all", "winget").map((t) => t.name).sort()).toEqual(["jq", "rg"]);
  });
  it("blank filter+query returns all", () => {
    expect(filterTools(TOOLS, "all", "").length).toBe(4);
  });
});

describe("census", () => {
  it("counts installed/missing/outdated/installable-missing", () => {
    const c = census(TOOLS, new Set(["jq"]));
    expect(c).toEqual({ total: 4, installed: 2, missing: 2, outdated: 1, installableMissing: 1 });
  });
  it("outdated only counts installed tools", () => {
    // "rg" is missing; even if flagged outdated it must not count.
    expect(census(TOOLS, new Set(["rg"])).outdated).toBe(0);
  });
});

describe("categoriesPresent", () => {
  it("returns present categories in canonical order, no empties", () => {
    expect(categoriesPresent(TOOLS)).toEqual(["vcs", "search", "data", "media"]);
  });
  it("every present category has a label", () => {
    for (const c of categoriesPresent(TOOLS)) {
      expect(CATEGORY_LABELS[c]).toBeTruthy();
    }
  });
});
