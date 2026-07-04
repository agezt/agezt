// ─────────────────── extension / mention classification ───────────────────
// Single source of truth for the language / extension lists the UI uses to
// classify a text-shaped artifact, pick a syntax-highlight grammar, or
// recognise an inline file mention. Replaced three near-identical copies
// (Files.textKind / Artifacts.looksLikePath / Markdown INLINE_RE) that had
// drifted apart already. (Previously lib/languages.ts; merged into lib/language
// so the ext↔lang mapping lives as one concern — C2.)

export const CODE_EXTS = new Set([
  "js", "ts", "tsx", "jsx", "go", "py", "rs", "java", "c", "cpp", "h", "sh",
  "yaml", "yml", "toml", "css", "html", "xml", "sql",
]);

export const TEXT_EXTS = new Set([
  "txt", "log", "csv", "tsv", "ini", "env", "conf",
]);

// MENTION_EXTS is the subset that the inline file-mention grammar accepts
// without a path separator (e.g. a lone "README.md" in chat prose). It is
// the union of the file extensions any of our surfaces consider text/code,
// minus the binary / image ones — we never want to treat "photo.jpg" or
// "blob.bin" as a clickable file.
export const MENTION_EXTS = new Set<string>([...CODE_EXTS, ...TEXT_EXTS, "md", "markdown", "json"]);

// escapeRegex escapes a string for safe inclusion as a literal inside a
// RegExp source. The standard `[\.\-\*]` set covers what `replace` will see.
export function escapeRegex(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// extOf returns the lowercase extension for a path-like string, or "" when
// there isn't one. A leading-dotfile like ".gitignore" has no extension.
export function extOf(nameOrPath: string): string {
  const s = (nameOrPath || "").toLowerCase();
  const lastDot = s.lastIndexOf(".");
  if (lastDot < 0) return "";
  if (lastDot === s.length - 1) return ""; // "x."
  if (lastDot === 0) return ""; // ".gitignore"
  // The last slash must come BEFORE the last dot (otherwise "foo.dir/bar"
  // would look like ".dir/bar" with a "dir" "extension" on the dirname).
  if (s.lastIndexOf("/") > lastDot) return "";
  return s.slice(lastDot + 1);
}

// FileMentionRE matches an inline file reference in chat prose. The captured
// text (the match itself) is the path; leading/trailing punctuation is NOT
// included — the lookarounds assert the match sits between boundaries but
// don't extend the match.
//
//   notes/x.md                         → "notes/x.md"
//   kernel/agent/agent.go              → "kernel/agent/agent.go"
//   README.md                          → "README.md"        (has a known ext)
//   src/index.tsx                      → "src/index.tsx"
//   https://example.com/x.md           → NO MATCH (URL stays a link)
//   `notes/x.md`                       → NO MATCH (already inside a code span)
//   **notes/x.md**                     → NO MATCH (consumed by the strong grammar)
//   foo/bar?q=1                        → NO MATCH (query string, not a path)
//   /etc/passwd                        → NO MATCH (absolute; the server refuses it)
//
// Built lazily so the cached regex can include the full ext list without
// breaking tsc's import resolution.
let cachedRE: RegExp | null = null;
export function fileMentionRegex(): RegExp {
  if (cachedRE) return cachedRE;
  const exts = [...MENTION_EXTS].sort((a, b) => b.length - a.length); // longest first
  const alt = exts.map(escapeRegex).join("|");
  // Either "dir/file.ext" (one or more "name/" prefixes) or a bare
  // "file.ext" with no directory. Lookbehind = line start / whitespace /
  // opening punctuation; lookahead = line end / whitespace / closing
  // punctuation. The whole match is the path itself.
  cachedRE = new RegExp(
    `(?<=^|\\s|[\\(\\[\"\\'])(?:(?:[A-Za-z0-9._\\-]+/)+)?[A-Za-z0-9._\\-]+\\.(${alt})(?=$|\\s|[\\)\\]"'. ,;:!?])`,
    "g",
  );
  return cachedRE;
}

// ─────────────────── Monaco language mapping ───────────────────
// Language mapping (M1017). Maps a file path / basename to a Monaco language
// id. Falls back to "plaintext" when we have no idea — Monaco will still
// render the buffer, just without highlighting.

const EXT_TO_LANG: Record<string, string> = {
  ts: "typescript",
  tsx: "typescript",
  js: "javascript",
  jsx: "javascript",
  mjs: "javascript",
  cjs: "javascript",
  go: "go",
  py: "python",
  rs: "rust",
  java: "java",
  c: "c",
  cpp: "cpp",
  cc: "cpp",
  cxx: "cpp",
  h: "cpp",
  hpp: "cpp",
  sh: "shell",
  bash: "shell",
  zsh: "shell",
  yml: "yaml",
  yaml: "yaml",
  toml: "ini",
  ini: "ini",
  css: "css",
  scss: "scss",
  less: "less",
  html: "html",
  htm: "html",
  xml: "xml",
  svg: "xml",
  sql: "sql",
  md: "markdown",
  markdown: "markdown",
  json: "json",
  jsonc: "json",
  txt: "plaintext",
  log: "plaintext",
  csv: "plaintext",
  tsv: "plaintext",
  conf: "plaintext",
  env: "plaintext",
};

// languageFor picks the best Monaco language id for a path. Filename matters
// (e.g. "Dockerfile" → "dockerfile") and extension is the fallback.
export function languageFor(pathOrName: string): string {
  if (!pathOrName) return "plaintext";
  const base = pathOrName.slice(pathOrName.lastIndexOf("/") + 1).toLowerCase();
  // Filename shortcuts that don't depend on the extension.
  if (base === "dockerfile") return "dockerfile";
  if (base === "makefile") return "makefile";
  if (base === ".bashrc" || base === ".zshrc") return "shell";

  const ext = extOf(pathOrName);
  return EXT_TO_LANG[ext] || "plaintext";
}
