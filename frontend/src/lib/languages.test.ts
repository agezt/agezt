// @vitest-environment node
import { describe, expect, it } from "vitest";
import { extOf, fileMentionRegex, MENTION_EXTS } from "./language";

describe("language.extOf", () => {
  it("returns lowercase extension, or empty when none", () => {
    expect(extOf("notes/x.md")).toBe("md");
    expect(extOf("FOO.TSX")).toBe("tsx");
    expect(extOf("")).toBe("");
    expect(extOf("x.")).toBe("");
    expect(extOf("a/b/c")).toBe("");
    expect(extOf(".gitignore")).toBe("");
  });
});

describe("languages.MENTION_EXTS", () => {
  it("contains the obvious code/text extensions", () => {
    expect(MENTION_EXTS.has("md")).toBe(true);
    expect(MENTION_EXTS.has("ts")).toBe(true);
    expect(MENTION_EXTS.has("tsx")).toBe(true);
    expect(MENTION_EXTS.has("go")).toBe(true);
    expect(MENTION_EXTS.has("py")).toBe(true);
    expect(MENTION_EXTS.has("json")).toBe(true);
    expect(MENTION_EXTS.has("yaml")).toBe(true);
    expect(MENTION_EXTS.has("toml")).toBe(true);
    expect(MENTION_EXTS.has("sh")).toBe(true);
  });

  it("does not contain image / binary types", () => {
    expect(MENTION_EXTS.has("png")).toBe(false);
    expect(MENTION_EXTS.has("jpg")).toBe(false);
    expect(MENTION_EXTS.has("pdf")).toBe(false);
    expect(MENTION_EXTS.has("zip")).toBe(false);
    expect(MENTION_EXTS.has("exe")).toBe(false);
  });
});

describe("languages.fileMentionRegex", () => {
  // The regex is global; tests instantiate a fresh one to keep state clean.
  const all = (s: string) => {
    const re = new RegExp(fileMentionRegex().source, "g");
    return [...s.matchAll(re)].map((m) => m[0].trim());
  };

  it("matches paths with a directory", () => {
    expect(all("see notes/x.md for details")).toEqual(["notes/x.md"]);
    expect(all("from kernel/agent/agent.go")).toEqual(["kernel/agent/agent.go"]);
    expect(all("src/index.tsx handles it")).toEqual(["src/index.tsx"]);
  });

  it("matches a bare filename with a known extension", () => {
    expect(all("the README.md file")).toEqual(["README.md"]);
    expect(all("see agent.go")).toEqual(["agent.go"]);
    expect(all("package.json")).toEqual(["package.json"]);
  });

  it("rejects URLs that look like paths", () => {
    // URLs must stay links — the URL parser in INLINE_RE handles them.
    expect(all("https://example.com/x.md")).toEqual([]);
    expect(all("see http://foo.com/notes/x.md")).toEqual([]);
  });

  it("rejects absolute paths", () => {
    expect(all("/etc/passwd")).toEqual([]);
    expect(all("/usr/local/bin/foo")).toEqual([]);
  });

  it("rejects unknown extensions and bare names", () => {
    expect(all("photo.jpg")).toEqual([]);
    expect(all("hello world")).toEqual([]);
    expect(all("untitled")).toEqual([]);
  });

  it("honours surrounding punctuation but does not include it", () => {
    expect(all("(notes/x.md)")).toEqual(["notes/x.md"]);
    expect(all("use src/index.tsx, please")).toEqual(["src/index.tsx"]);
    expect(all("\"kernel/agent/agent.go\"")).toEqual(["kernel/agent/agent.go"]);
  });
});
