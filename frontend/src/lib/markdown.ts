// A deliberately tiny, dependency-free Markdown parser for rendering agent
// answers in the Chat view. It produces a block/inline AST that a React
// component renders as plain elements (text becomes React children, which React
// escapes) — no raw-HTML injection, so it is XSS- and CSP-safe by construction.
// It handles the subset LLM answers actually use (fenced code, inline code,
// bold/italic, bullet & numbered lists, headings, paragraphs); anything it
// doesn't recognise falls through as text.

export type Inline =
  | { t: "text"; v: string }
  | { t: "code"; v: string }
  | { t: "strong"; v: string }
  | { t: "em"; v: string }
  | { t: "del"; v: string }
  | { t: "link"; v: string; href: string }
  // File mention: a clickable POSIX path that opens the File Manager. Paths
  // need a slash OR a known code/text/markdown/json extension to qualify —
  // arbitrary words like "untitled" never match.
  | { t: "file"; v: string };

export type Block =
  | { t: "code"; lang: string; v: string }
  | { t: "json"; data: unknown } // a ```json / ```widget fence → rendered as a data widget
  | { t: "ul"; items: string[] }
  | { t: "ol"; items: string[] }
  | { t: "h"; level: number; v: string }
  | { t: "table"; header: string[]; rows: string[][] }
  | { t: "quote"; v: string }
  | { t: "p"; v: string };

// Earliest-match of `code`, [text](href) links, **strong**, ~~del~~, *em* (code
// first so markup inside code is literal; link before emphasis so `*` inside a
// URL/label doesn't split it). Non-greedy, single-line spans.
const INLINE_RE = /(`[^`]+`|\[[^\]]+\]\([^)\s]+\)|\*\*[^*]+\*\*|~~[^~]+~~|\*[^*]+\*)/;

// LINK_RE pulls the label and href out of a matched [text](href) span.
const LINK_RE = /^\[([^\]]+)\]\(([^)\s]+)\)$/;

// safeHref returns the href only when it's a navigable, non-script scheme
// (http/https/mailto) — so a `[x](javascript:…)` link can never execute. Anything
// else returns "" and the caller renders the label as plain text.
export function safeHref(href: string): string {
  return /^(https?:\/\/|mailto:)/i.test(href.trim()) ? href.trim() : "";
}

import { fileMentionRegex } from "@/lib/languages";

// Strip LaTeX math delimiters (\( … \), \[ … \]) so a model's math reads as the
// plain expression instead of literal backslash-brackets. Applied only to
// inline (paragraph/cell/heading/list) text — never to code blocks, which don't
// pass through here — so a backslash in code is untouched.
function stripMathDelimiters(s: string): string {
  return s
    .replace(/\\\[\s*([\s\S]*?)\s*\\\]/g, (_, x) => x)
    .replace(/\\\(\s*([\s\S]*?)\s*\\\)/g, (_, x) => x);
}

export function parseInline(input: string): Inline[] {
  const out: Inline[] = [];
  let rest = stripMathDelimiters(input);
  // Local non-global copies of the mention regex so we can call .exec() each
  // iteration without stateful lastIndex drift. fileMentionRegex() returns a
  // /g regex by design (for callers that want all matches); we keep one
  // matched group explicit here so the link grammar never sees "notes/x.md"
  // as a malformed "[x](.md)" link.
  while (rest.length > 0) {
    const fm = new RegExp(fileMentionRegex().source).exec(rest);
    const im = rest.match(INLINE_RE);
    // Pick whichever match lands earlier in `rest`. Default to whichever
    // exists when the other doesn't.
    let picked: "file" | "inline" | null = null;
    let pos = -1;
    if (fm && fm.index != null && (!im || im.index == null || fm.index <= im.index)) {
      picked = "file";
      pos = fm.index;
    } else if (im && im.index != null) {
      picked = "inline";
      pos = im.index;
    }
    if (!picked) {
      out.push({ t: "text", v: rest });
      break;
    }
    if (pos > 0) out.push({ t: "text", v: rest.slice(0, pos) });
    if (picked === "file") {
      out.push({ t: "file", v: fm![0] });
      rest = rest.slice(pos + fm![0].length);
      continue;
    }
    const tok = im![0];
    if (tok.startsWith("`")) {
      out.push({ t: "code", v: tok.slice(1, -1) });
    } else if (tok.startsWith("[")) {
      const lm = tok.match(LINK_RE);
      const href = lm ? safeHref(lm[2]) : "";
      if (lm && href) out.push({ t: "link", v: lm[1], href });
      else out.push({ t: "text", v: tok }); // unsafe/odd href → render literally
    } else if (tok.startsWith("**")) {
      out.push({ t: "strong", v: tok.slice(2, -2) });
    } else if (tok.startsWith("~~")) {
      out.push({ t: "del", v: tok.slice(2, -2) });
    } else {
      out.push({ t: "em", v: tok.slice(1, -1) });
    }
    rest = rest.slice(pos + tok.length);
  }
  return out;
}

const HEADING_RE = /^(#{1,6})\s+(.*)$/;
const UL_RE = /^\s*[-*]\s+(.*)$/;
const OL_RE = /^\s*\d+\.\s+(.*)$/;
const QUOTE_RE = /^\s*>\s?(.*)$/;
// A GFM table separator row: pipe-delimited cells of dashes with optional
// alignment colons, e.g. |---|:--:|---:|  (at least one dash per cell).
const TABLE_SEP_RE = /^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$/;

// splitRow splits a GFM table row into trimmed cells, dropping the optional
// outer pipes.
function splitRow(line: string): string[] {
  let s = line.trim();
  if (s.startsWith("|")) s = s.slice(1);
  if (s.endsWith("|")) s = s.slice(0, -1);
  return s.split("|").map((c) => c.trim());
}

export function parseMarkdown(src: string): Block[] {
  const lines = (src || "").replace(/\r\n/g, "\n").split("\n");
  const blocks: Block[] = [];
  let para: string[] = [];

  const flushPara = () => {
    if (para.length > 0) {
      blocks.push({ t: "p", v: para.join("\n") });
      para = [];
    }
  };

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];

    // Fenced code block: ```lang … ```
    const fence = line.match(/^```(.*)$/);
    if (fence) {
      flushPara();
      const lang = fence[1].trim();
      const body: string[] = [];
      i++;
      while (i < lines.length && !/^```/.test(lines[i])) {
        body.push(lines[i]);
        i++;
      }
      // i now sits on the closing fence (or past end if unterminated).
      // A ```json / ```widget fence holding valid JSON becomes a data widget
      // (table / key-value / list, chosen by shape); anything else stays code.
      const bodyStr = body.join("\n");
      if ((lang === "json" || lang === "widget") && bodyStr.trim() !== "") {
        try {
          blocks.push({ t: "json", data: JSON.parse(bodyStr) });
          continue;
        } catch {
          /* not valid JSON — fall through to a normal code block */
        }
      }
      blocks.push({ t: "code", lang, v: bodyStr });
      continue;
    }

    if (line.trim() === "") {
      flushPara();
      continue;
    }

    const h = line.match(HEADING_RE);
    if (h) {
      flushPara();
      blocks.push({ t: "h", level: h[1].length, v: h[2].trim() });
      continue;
    }

    // GFM table: a header row of cells followed by a |---|---| separator.
    if (line.includes("|") && i + 1 < lines.length && TABLE_SEP_RE.test(lines[i + 1])) {
      flushPara();
      const header = splitRow(line);
      i += 2; // consume header + separator
      const rows: string[][] = [];
      while (i < lines.length && lines[i].includes("|") && lines[i].trim() !== "") {
        rows.push(splitRow(lines[i]));
        i++;
      }
      i--; // step back; the for-loop advances
      blocks.push({ t: "table", header, rows });
      continue;
    }

    if (QUOTE_RE.test(line)) {
      flushPara();
      const quoted: string[] = [];
      while (i < lines.length && QUOTE_RE.test(lines[i])) {
        quoted.push(lines[i].match(QUOTE_RE)![1]);
        i++;
      }
      i--;
      blocks.push({ t: "quote", v: quoted.join("\n") });
      continue;
    }

    if (UL_RE.test(line)) {
      flushPara();
      const items: string[] = [];
      while (i < lines.length && UL_RE.test(lines[i])) {
        items.push(lines[i].match(UL_RE)![1]);
        i++;
      }
      i--; // step back; the for-loop will advance
      blocks.push({ t: "ul", items });
      continue;
    }

    if (OL_RE.test(line)) {
      flushPara();
      const items: string[] = [];
      while (i < lines.length && OL_RE.test(lines[i])) {
        items.push(lines[i].match(OL_RE)![1]);
        i++;
      }
      i--;
      blocks.push({ t: "ol", items });
      continue;
    }

    para.push(line);
  }
  flushPara();
  return blocks;
}
