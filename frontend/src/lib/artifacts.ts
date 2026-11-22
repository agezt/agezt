import { authHeaders } from "@/app/api";

// The artifact domain: what a stored artifact IS, how to classify it, and how to
// get its bytes. The daemon indexes every agent output and inbound channel file
// (M822); metadata comes from /api/artifacts, bytes from the binary
// /api/artifact/raw route. Artifacts are content-addressed — read + delete only.
//
// This lived inside views/Files.tsx until the 2026-09 IA pass, which made four
// unrelated modules (the Artifacts gallery, Inbox, ChannelSessions and the file
// manager workspace) import domain logic *from a view*. A view is a screen, not
// a module other screens depend on; the dependency is why the Files/Artifacts
// duplicate pair could not be merged without breaking the inbox.
//
// It lived inside the artifacts FEATURE until W5.3, which made four other
// modules (the inbox, the channels view, the file manager and the shared
// layer) import it from a feature. The same argument applies: what several
// screens share belongs in the shared layer, not in one feature's pocket.

export type ArtifactEntry = {
  id: string;
  ref: string;
  name?: string;
  mime?: string;
  kind?: string;
  source?: string;
  sender?: string;
  corr?: string;
  size?: number;
  created_ms?: number;
  caption?: string;
};

export type ArtifactList = {
  count: number;
  entries: ArtifactEntry[];
};

// isImage decides whether an entry renders as a gallery thumbnail.
export function isImage(e: ArtifactEntry): boolean {
  return e.kind === "image" || (e.mime ?? "").toLowerCase().startsWith("image/");
}

// isRunInternal flags an entry that is a RUN BYPRODUCT, not something a human or
// agent deliberately produced or uploaded: the agent loop offloads any large tool
// output (code-exec / introspect / shell / skill stdout > the artifact threshold)
// to the blob store and auto-indexes it as kind="tool-output", source="run". These
// pile up fast and drown the real files/artifacts. The galleries hide them by
// default (a toggle reveals them); the bytes stay retrievable by raw_ref from the
// run, so nothing is lost — they just don't clutter the human-facing view.
export function isRunInternal(e: ArtifactEntry): boolean {
  return e.kind === "tool-output" || e.source === "run";
}

// isPdf / textKind classify an entry for inline preview (M842). textKind returns
// "markdown" | "json" | "code" | "text" for text-like artifacts, or "" otherwise —
// driving how the preview pane renders the fetched bytes.
export function isPdf(e: ArtifactEntry): boolean {
  return (e.mime ?? "").toLowerCase().includes("pdf") || (e.name ?? "").toLowerCase().endsWith(".pdf");
}

export function textKind(e: ArtifactEntry): "markdown" | "json" | "code" | "text" | "" {
  const mime = (e.mime ?? "").toLowerCase();
  const name = (e.name ?? "").toLowerCase();
  const ext = name.includes(".") ? name.slice(name.lastIndexOf(".") + 1) : "";
  if (mime === "text/markdown" || ext === "md" || ext === "markdown") return "markdown";
  if (mime.includes("json") || ext === "json") return "json";
  if (
    ["js", "ts", "tsx", "jsx", "go", "py", "rs", "java", "c", "cpp", "h", "sh", "yaml", "yml", "toml", "css", "html", "xml", "sql"].includes(ext) ||
    mime.includes("javascript") ||
    mime.includes("yaml") ||
    mime.includes("xml")
  )
    return "code";
  if (mime.startsWith("text/") || ["txt", "log", "csv", "tsv", "ini", "env", "conf"].includes(ext)) return "text";
  // Unknown mime with a known-text-ish nothing → not previewable as text.
  return "";
}

// categoryOf buckets an entry for the Artifacts gallery (M931): the file types
// agents actually produce, each with its own preview treatment. Checked before
// textKind so html/svg get their dedicated buckets rather than "code"/"image".
export type ArtifactCategory = "image" | "svg" | "html" | "pdf" | "markdown" | "json" | "code" | "text" | "other";

export function categoryOf(e: ArtifactEntry): ArtifactCategory {
  const mime = (e.mime ?? "").toLowerCase();
  const name = (e.name ?? "").toLowerCase();
  const ext = name.includes(".") ? name.slice(name.lastIndexOf(".") + 1) : "";
  if (mime.includes("svg") || ext === "svg") return "svg";
  if (isImage(e)) return "image";
  if (mime === "text/html" || ext === "html" || ext === "htm") return "html";
  if (isPdf(e)) return "pdf";
  const t = textKind(e);
  return t === "" ? "other" : t;
}

// previewMaxBytes caps inline text fetches so a giant artifact can't hang the UI.
export const previewMaxBytes = 2 * 1024 * 1024;

// rawURL builds the binary URL for an entry. Fetch callers attach bearer auth;
// browser navigations must use a Blob URL produced by fetchRawBlob instead.
export function rawURL(e: ArtifactEntry, download = false): string {
  const params = new URLSearchParams({ ref: e.ref });
  if (e.mime) params.set("mime", e.mime);
  if (download) {
    params.set("download", "1");
    params.set("name", e.name || `${e.kind || "artifact"}-${e.id}`);
  }
  return `/api/artifact/raw?${params.toString()}`;
}

export async function fetchRawBlob(e: ArtifactEntry, download = false): Promise<Blob> {
  const res = await fetch(rawURL(e, download), { headers: authHeaders() });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.blob();
}

export async function downloadArtifact(e: ArtifactEntry): Promise<void> {
  const blob = await fetchRawBlob(e, true);
  const href = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = href;
  a.download = e.name || `${e.kind || "artifact"}-${e.id}`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(href);
}

// humanSize is the artifact domain's name for the shared byte formatter; the
// implementation lives in lib/format so every page agrees on it.
export { bytes as humanSize } from "@/app/format";
