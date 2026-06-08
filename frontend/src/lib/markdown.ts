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
  | { t: "ul"; items: string[] }
  | { t: "ol"; items: string[] }
  | { t: "h"; level: number; v: string }
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
      blocks.push({ t: "code", lang, v: body.join("\n") });
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
