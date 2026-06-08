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
  | { t: "em"; v: string };

export type Block =
  | { t: "code"; lang: string; v: string }
  | { t: "json"; data: unknown } // a ```json / ```widget fence → rendered as a data widget
  | { t: "ul"; items: string[] }
  | { t: "ol"; items: string[] }
  | { t: "h"; level: number; v: string }
  | { t: "table"; header: string[]; rows: string[][] }
  | { t: "quote"; v: string }
  | { t: "p"; v: string };

// Earliest-match of `code`, **strong**, *em* (code first so `*` inside code is
// literal). Non-greedy, single-line spans.
const INLINE_RE = /(`[^`]+`|\*\*[^*]+\*\*|\*[^*]+\*)/;

export function parseInline(s: string): Inline[] {
  const out: Inline[] = [];
  let rest = s;
  while (rest.length > 0) {
    const m = rest.match(INLINE_RE);
    if (!m || m.index == null) {
      out.push({ t: "text", v: rest });
      break;
    }
    if (m.index > 0) out.push({ t: "text", v: rest.slice(0, m.index) });
    const tok = m[0];
    if (tok.startsWith("`")) out.push({ t: "code", v: tok.slice(1, -1) });
    else if (tok.startsWith("**")) out.push({ t: "strong", v: tok.slice(2, -2) });
    else out.push({ t: "em", v: tok.slice(1, -1) });
    rest = rest.slice(m.index + tok.length);
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
