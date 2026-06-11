import { describe, it, expect } from "vitest";
import { parseInline, parseMarkdown, safeHref } from "@/lib/markdown";

describe("parseInline", () => {
  it("splits inline code, bold and italic out of plain text", () => {
    expect(parseInline("run `npm test` now")).toEqual([
      { t: "text", v: "run " },
      { t: "code", v: "npm test" },
      { t: "text", v: " now" },
    ]);
    expect(parseInline("**bold** and *soft*")).toEqual([
      { t: "strong", v: "bold" },
      { t: "text", v: " and " },
      { t: "em", v: "soft" },
    ]);
  });

  it("treats * inside inline code as literal (code wins)", () => {
    expect(parseInline("`a*b`")).toEqual([{ t: "code", v: "a*b" }]);
  });

  it("returns a single text token when there's no markup", () => {
    expect(parseInline("just words")).toEqual([{ t: "text", v: "just words" }]);
  });

  it("parses [text](href) links and strikethrough (M825)", () => {
    expect(parseInline("see [the docs](https://agezt.dev/x) here")).toEqual([
      { t: "text", v: "see " },
      { t: "link", v: "the docs", href: "https://agezt.dev/x" },
      { t: "text", v: " here" },
    ]);
    expect(parseInline("~~old~~ new")).toEqual([
      { t: "del", v: "old" },
      { t: "text", v: " new" },
    ]);
  });

  it("renders an unsafe-scheme link as literal text, never executes it", () => {
    // javascript: href must not become a <link>; falls through as plain text.
    expect(parseInline("[x](javascript:alert)")).toEqual([
      { t: "text", v: "[x](javascript:alert)" },
    ]);
    expect(safeHref("javascript:alert(1)")).toBe("");
    expect(safeHref("https://ok.com")).toBe("https://ok.com");
    expect(safeHref("mailto:a@b.com")).toBe("mailto:a@b.com");
  });

  it("strips LaTeX math delimiters, keeping the expression", () => {
    expect(parseInline("so \\( 2b = 0.10 \\) gives the answer")).toEqual([
      { t: "text", v: "so 2b = 0.10 gives the answer" },
    ]);
    expect(parseInline("\\[ E = mc^2 \\]")).toEqual([{ t: "text", v: "E = mc^2" }]);
  });
});

describe("parseMarkdown", () => {
  it("captures a fenced code block with its language", () => {
    const blocks = parseMarkdown("before\n\n```go\nfmt.Println(1)\n```\nafter");
    expect(blocks).toEqual([
      { t: "p", v: "before" },
      { t: "code", lang: "go", v: "fmt.Println(1)" },
      { t: "p", v: "after" },
    ]);
  });

  it("turns a ```json fence of valid JSON into a data widget block", () => {
    const md = '```json\n[{"a":1},{"a":2}]\n```';
    expect(parseMarkdown(md)).toEqual([{ t: "json", data: [{ a: 1 }, { a: 2 }] }]);
  });

  it("treats a ```widget fence the same way", () => {
    expect(parseMarkdown('```widget\n{"k":"v"}\n```')).toEqual([{ t: "json", data: { k: "v" } }]);
  });

  it("falls back to a code block when a ```json fence isn't valid JSON", () => {
    expect(parseMarkdown("```json\nnot json\n```")).toEqual([{ t: "code", lang: "json", v: "not json" }]);
  });

  it("keeps an unterminated fence as a code block (doesn't crash)", () => {
    const blocks = parseMarkdown("```\nstill open");
    expect(blocks).toEqual([{ t: "code", lang: "", v: "still open" }]);
  });

  it("groups consecutive bullet and numbered list items", () => {
    const blocks = parseMarkdown("- one\n- two\n\n1. a\n2. b");
    expect(blocks).toEqual([
      { t: "ul", items: ["one", "two"] },
      { t: "ol", items: ["a", "b"] },
    ]);
  });

  it("parses headings by level", () => {
    expect(parseMarkdown("# Title\n## Sub")).toEqual([
      { t: "h", level: 1, v: "Title" },
      { t: "h", level: 2, v: "Sub" },
    ]);
  });

  it("parses a GFM table into header + rows (with alignment colons)", () => {
    const md = "| Step | Result |\n|------|:------:|\n| 1 | ok |\n| 2 | done |";
    expect(parseMarkdown(md)).toEqual([
      { t: "table", header: ["Step", "Result"], rows: [["1", "ok"], ["2", "done"]] },
    ]);
  });

  it("does not treat a lone pipe line (no separator) as a table", () => {
    const blocks = parseMarkdown("a | b is not a table");
    expect(blocks).toEqual([{ t: "p", v: "a | b is not a table" }]);
  });

  it("parses a blockquote, stripping the markers", () => {
    expect(parseMarkdown("> note one\n> note two")).toEqual([{ t: "quote", v: "note one\nnote two" }]);
  });

  it("joins wrapped lines into one paragraph and splits on blank lines", () => {
    const blocks = parseMarkdown("line one\nline two\n\nnext para");
    expect(blocks).toEqual([
      { t: "p", v: "line one\nline two" },
      { t: "p", v: "next para" },
    ]);
  });
});
