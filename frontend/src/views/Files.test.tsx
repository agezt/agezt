// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
  authHeaders: () => new Headers({ Authorization: "Bearer test-token" }),
}));

const confirm = vi.fn();
const toast = vi.fn();
vi.mock("@/components/ui/feedback", () => ({
  useUI: () => ({ confirm: (...a: unknown[]) => confirm(...a), toast }),
}));

import { Files, isImage, rawURL, isPdf, textKind, isRunInternal, BlobArtifact } from "@/views/Files";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
beforeEach(() => {
  getJSON.mockReset();
  postAction.mockReset();
  confirm.mockReset();
  toast.mockReset();
  postAction.mockResolvedValue({});
  vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:test-artifact");
  vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
  vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(new Blob(["img"], { type: "image/png" })));
});

describe("pure helpers", () => {
  it("isImage by kind or mime", () => {
    expect(isImage({ id: "1", ref: "r", kind: "image" })).toBe(true);
    expect(isImage({ id: "1", ref: "r", mime: "image/png" })).toBe(true);
    expect(isImage({ id: "1", ref: "r", kind: "tool-output", mime: "text/plain" })).toBe(false);
  });
  it("rawURL carries ref + mime, and download params when asked", () => {
    expect(rawURL({ id: "1", ref: "abc", mime: "image/png" })).toBe("/api/artifact/raw?ref=abc&mime=image%2Fpng");
    const dl = rawURL({ id: "1", ref: "abc", mime: "image/png", name: "x.png" }, true);
    expect(dl).toContain("download=1");
    expect(dl).toContain("name=x.png");
  });
  it("isPdf by mime or extension", () => {
    expect(isPdf({ id: "1", ref: "r", mime: "application/pdf" })).toBe(true);
    expect(isPdf({ id: "1", ref: "r", name: "report.PDF" })).toBe(true);
    expect(isPdf({ id: "1", ref: "r", mime: "text/plain" })).toBe(false);
  });
  it("textKind classifies markdown/json/code/text and binary", () => {
    expect(textKind({ id: "1", ref: "r", mime: "text/markdown" })).toBe("markdown");
    expect(textKind({ id: "1", ref: "r", name: "notes.md" })).toBe("markdown");
    expect(textKind({ id: "1", ref: "r", mime: "application/json" })).toBe("json");
    expect(textKind({ id: "1", ref: "r", name: "main.go" })).toBe("code");
    expect(textKind({ id: "1", ref: "r", name: "data.csv", mime: "text/csv" })).toBe("text");
    expect(textKind({ id: "1", ref: "r", mime: "application/octet-stream" })).toBe("");
    expect(textKind({ id: "1", ref: "r", mime: "image/png" })).toBe("");
  });
  it("isRunInternal flags offloaded tool outputs but not real files/uploads", () => {
    expect(isRunInternal({ id: "1", ref: "r", kind: "tool-output", source: "run" })).toBe(true);
    expect(isRunInternal({ id: "1", ref: "r", source: "run" })).toBe(true);
    expect(isRunInternal({ id: "1", ref: "r", kind: "image", source: "telegram" })).toBe(false);
    expect(isRunInternal({ id: "1", ref: "r", kind: "download", source: "fetch" })).toBe(false);
    expect(isRunInternal({ id: "1", ref: "r", kind: "file", name: "report.md" })).toBe(false);
  });
});

describe("Files view", () => {
  const list = {
    count: 3,
    entries: [
      { id: "art-1", ref: "aaa", kind: "image", source: "telegram", mime: "image/png", created_ms: 3, caption: "a cat" },
      { id: "art-2", ref: "bbb", kind: "tool-output", source: "run", name: "out.txt", mime: "text/plain", size: 1200, created_ms: 2 },
      { id: "art-3", ref: "ccc", kind: "file", name: "report.md", mime: "text/markdown", size: 800, created_ms: 1 },
    ],
  };

  it("renders an image gallery and a real files list, and deletes after confirm", async () => {
    getJSON.mockResolvedValue(list);
    confirm.mockResolvedValue(true);
    render(<Files />);

    // The deliberate file (report.md) shows; the run-internal out.txt is hidden by default.
    await waitFor(() => expect(screen.getByText("report.md")).toBeTruthy());
    expect(screen.queryByText("out.txt")).toBeNull();
    await waitFor(() => expect(document.querySelector("img")).toBeTruthy());
    const img = document.querySelector("img") as HTMLImageElement;
    expect(img.getAttribute("src")).toBe("blob:test-artifact");
    expect(fetch).toHaveBeenCalledWith("/api/artifact/raw?ref=aaa&mime=image%2Fpng", { headers: expect.any(Headers) });

    // Delete the file row → confirm → postAction with the id.
    fireEvent.click(screen.getByTitle("Delete"));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/artifact/delete", { id: "art-3" }));
  });

  it("hides run outputs by default and reveals them via the toggle", async () => {
    getJSON.mockResolvedValue(list);
    render(<Files />);
    await waitFor(() => expect(screen.getByText("report.md")).toBeTruthy());
    expect(screen.queryByText("out.txt")).toBeNull();
    // Toggle reveals the offloaded run output.
    fireEvent.click(screen.getByText(/Show run outputs/));
    await waitFor(() => expect(screen.getByText("out.txt")).toBeTruthy());
  });

  it("shows an empty state when nothing is stored", async () => {
    getJSON.mockResolvedValue({ count: 0, entries: [] });
    render(<Files />);
    await waitFor(() => expect(screen.getByText("No stored files yet")).toBeTruthy());
  });
});

// CLI-004: the PDF preview frame renders agent-authored bytes from a blob: URL.
// Its sibling in Artifacts.tsx has always carried sandbox="" (empty value = every
// permission withheld: no scripts, no forms, no top-level navigation) plus
// referrerPolicy="no-referrer". This frame omitted both. Three outer controls
// currently keep hostile content out of it — categoryOf orders svg/html ahead of
// pdf, safeContentType allowlists the served type, and the console CSP has no
// frame-src so default-src 'none' blocks the frame entirely — but the frame's own
// attributes are the layer that survives any of those changing.
describe("BlobArtifact pdf frame is sandboxed (CLI-004)", () => {
  it("renders the pdf preview with sandbox and no-referrer", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(new Blob(["%PDF-1.7"], { type: "application/pdf" })),
    );
    render(<BlobArtifact entry={{ id: "1", ref: "abc", mime: "application/pdf", name: "doc.pdf" }} kind="pdf" />);

    const frame = (await waitFor(() => {
      const f = document.querySelector("iframe");
      expect(f).toBeTruthy();
      return f;
    })) as HTMLIFrameElement;

    // Present AND empty — sandbox="allow-scripts" would defeat the point.
    expect(frame.hasAttribute("sandbox")).toBe(true);
    expect(frame.getAttribute("sandbox")).toBe("");
    expect(frame.getAttribute("referrerpolicy")).toBe("no-referrer");
  });
});
