import { extOf } from "@/lib/languages";

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
