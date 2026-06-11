// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
  // withToken is used to build <img>/download URLs; a simple stub is enough.
  withToken: (path: string, extra?: Record<string, string>) =>
    `${path}?${new URLSearchParams(extra || {}).toString()}`,
}));

const confirm = vi.fn();
const toast = vi.fn();
vi.mock("@/components/ui/feedback", () => ({
  useUI: () => ({ confirm: (...a: unknown[]) => confirm(...a), toast }),
}));

import { Files, isImage, rawURL } from "@/views/Files";

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postAction.mockReset();
  confirm.mockReset();
  toast.mockReset();
  postAction.mockResolvedValue({});
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
});

describe("Files view", () => {
  const list = {
    count: 2,
    entries: [
      { id: "art-1", ref: "aaa", kind: "image", source: "telegram", mime: "image/png", created_ms: 2, caption: "a cat" },
      { id: "art-2", ref: "bbb", kind: "tool-output", name: "out.txt", mime: "text/plain", size: 1200, created_ms: 1 },
    ],
  };

  it("renders an image gallery and a files list, and deletes after confirm", async () => {
    getJSON.mockResolvedValue(list);
    confirm.mockResolvedValue(true);
    render(<Files />);

    // The image renders as an <img> with the raw URL; the file shows its name.
    await waitFor(() => expect(screen.getByText("out.txt")).toBeTruthy());
    const img = document.querySelector("img") as HTMLImageElement;
    expect(img).toBeTruthy();
    expect(img.getAttribute("src")).toContain("/api/artifact/raw?ref=aaa");

    // Delete the file row → confirm → postAction with the id.
    fireEvent.click(screen.getByTitle("Delete"));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/artifact/delete", { id: "art-2" }));
  });

  it("shows an empty state when nothing is stored", async () => {
    getJSON.mockResolvedValue({ count: 0, entries: [] });
    render(<Files />);
    await waitFor(() => expect(screen.getByText("No stored files yet")).toBeTruthy());
  });
});
