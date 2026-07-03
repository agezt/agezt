// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { looksLikePath } from "./Artifacts";

// looksLikePath drives the Viewer's "Open in File Manager" affordance: it must
// only return a path when the artifact's `ref` truly looks like one, so a
// content-addressed blob hash (e.g. "blake3:abcdef…") never hops into the
// file manager UI.

describe("looksLikePath", () => {
  it("accepts refs with a POSIX path separator", () => {
    expect(looksLikePath("notes/README.md")).toBe("notes/README.md");
    expect(looksLikePath("a/b/c.ts")).toBe("a/b/c.ts");
  });

  it("accepts a bare filename with a known extension", () => {
    expect(looksLikePath("README.md")).toBe("README.md");
    expect(looksLikePath("agent.go")).toBe("agent.go");
    expect(looksLikePath("config.yaml")).toBe("config.yaml");
  });

  it("rejects absolute, drive-prefixed, and NUL-containing refs", () => {
    expect(looksLikePath("/etc/passwd")).toBeNull();
    expect(looksLikePath("C:\\Windows")).toBeNull();
    expect(looksLikePath("notes/bad\0")).toBeNull();
  });

  it("rejects content-addressed hashes and channel refs", () => {
    expect(looksLikePath("blake3:abcdef1234567890")).toBeNull();
    expect(looksLikePath("telegram:42")).toBeNull();
    expect(looksLikePath("")).toBeNull();
    expect(looksLikePath(undefined)).toBeNull();
  });

  it("rejects bare names without a separator or known extension", () => {
    expect(looksLikePath("README")).toBeNull();
    expect(looksLikePath("untitled")).toBeNull();
  });
});
