// @vitest-environment node
import { describe, expect, it } from "vitest";
import { basename, isPathSafe, joinPath, parentPath } from "./files";

describe("files path helpers", () => {
  it("isPathSafe rejects absolute, drive-prefixed, and NUL paths", () => {
    expect(isPathSafe("notes/x.md")).toBe(true);
    expect(isPathSafe("")).toBe(true);
    expect(isPathSafe("/etc/passwd")).toBe(false);
    expect(isPathSafe("C:\\Windows")).toBe(false);
    expect(isPathSafe("notes/x\0y")).toBe(false);
  });

  it("isPathSafe rejects '..' segments", () => {
    expect(isPathSafe("../foo")).toBe(false);
    expect(isPathSafe("a/../b")).toBe(false);
    expect(isPathSafe("a/..hidden")).toBe(true); // '..' as a prefix of a name is fine
  });

  it("joinPath concatenates and normalises", () => {
    expect(joinPath("a", "b/c")).toBe("a/b/c");
    expect(joinPath("a/", "/b")).toBe("a/b");
    expect(joinPath("", "b")).toBe("b");
    expect(joinPath("a", "")).toBe("a");
    expect(joinPath("a", "./b")).toBe("a/b");
  });

  it("parentPath returns '' for root and prefix for everything else", () => {
    expect(parentPath("")).toBe("");
    expect(parentPath("a")).toBe("");
    expect(parentPath("a/b")).toBe("a");
    expect(parentPath("a/b/c")).toBe("a/b");
  });

  it("basename returns the leaf only", () => {
    expect(basename("")).toBe("");
    expect(basename("a")).toBe("a");
    expect(basename("a/b/c.md")).toBe("c.md");
  });
});
