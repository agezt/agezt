// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { categoryOf, type ArtifactEntry } from "./Files";
import { groupByCategory, matchesQuery, Artifacts } from "./Artifacts";
import { UIProvider } from "@/components/ui/feedback";

const entry = (over: Partial<ArtifactEntry>): ArtifactEntry => ({ id: "a1", ref: "r1", ...over });

const ENTRIES: ArtifactEntry[] = [
  entry({ id: "i1", ref: "ref-i1", name: "photo.png", mime: "image/png", kind: "image" }),
  entry({ id: "s1", ref: "ref-s1", name: "chart.svg", mime: "image/svg+xml" }),
  entry({ id: "h1", ref: "ref-h1", name: "report.html", mime: "text/html" }),
  entry({ id: "m1", ref: "ref-m1", name: "notes.md" }),
  entry({ id: "j1", ref: "ref-j1", name: "data.json" }),
  entry({ id: "b1", ref: "ref-b1", name: "blob.bin", mime: "application/octet-stream" }),
];

// A run byproduct (offloaded tool output) — hidden from the gallery by default.
const RUN_ENTRY = entry({ id: "run1", ref: "ref-run1", name: "shell-output.txt", mime: "text/plain", kind: "tool-output", source: "run" });

vi.mock("@/lib/usePanel", () => ({
  usePanel: () => ({ data: { count: 7, entries: [...ENTRIES, RUN_ENTRY] }, error: null, loading: false, reload: () => {} }),
}));

afterEach(cleanup);

describe("categoryOf", () => {
  it("buckets by what the artifact is, html/svg before code/image", () => {
    expect(categoryOf(entry({ name: "x.svg", mime: "image/svg+xml" }))).toBe("svg");
    expect(categoryOf(entry({ name: "x.png", mime: "image/png" }))).toBe("image");
    expect(categoryOf(entry({ name: "x.html" }))).toBe("html");
    expect(categoryOf(entry({ mime: "text/html" }))).toBe("html");
    expect(categoryOf(entry({ name: "x.pdf" }))).toBe("pdf");
    expect(categoryOf(entry({ name: "x.md" }))).toBe("markdown");
    expect(categoryOf(entry({ name: "x.json" }))).toBe("json");
    expect(categoryOf(entry({ name: "x.go" }))).toBe("code");
    expect(categoryOf(entry({ name: "x.log" }))).toBe("text");
    expect(categoryOf(entry({ name: "x.bin", mime: "application/octet-stream" }))).toBe("other");
  });
});

describe("groupByCategory", () => {
  it("groups in display order and drops empty buckets", () => {
    const groups = groupByCategory(ENTRIES);
    expect(groups.map((g) => g.key)).toEqual(["image", "svg", "html", "markdown", "json", "other"]);
    expect(groups[0].entries[0].id).toBe("i1");
  });
});

describe("matchesQuery", () => {
  it("matches name, caption, source, sender case-insensitively", () => {
    const e = entry({ name: "Report.html", caption: "weekly summary", source: "telegram", sender: "ersin" });
    expect(matchesQuery(e, "")).toBe(true);
    expect(matchesQuery(e, "report")).toBe(true);
    expect(matchesQuery(e, "WEEKLY")).toBe(true);
    expect(matchesQuery(e, "telegram")).toBe(true);
    expect(matchesQuery(e, "nope")).toBe(false);
  });
});

describe("Artifacts view", () => {
  it("renders category sections with counts and chips", () => {
    render(
      <UIProvider>
        <Artifacts />
      </UIProvider>,
    );
    expect(screen.getByText("Images (1)")).toBeTruthy();
    expect(screen.getByText("HTML (1)")).toBeTruthy();
    expect(screen.getByText("Markdown (1)")).toBeTruthy();
    expect(screen.getByText("6 produced")).toBeTruthy();
  });

  it("opens the viewer with a fullscreen toggle when a card is clicked", () => {
    render(
      <UIProvider>
        <Artifacts />
      </UIProvider>,
    );
    fireEvent.click(screen.getByTitle("report.html"));
    expect(screen.getByLabelText("Fullscreen")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("Fullscreen"));
    expect(screen.getByLabelText("Exit fullscreen")).toBeTruthy();
  });

  it("hides run outputs by default and reveals them via the toggle", () => {
    render(
      <UIProvider>
        <Artifacts />
      </UIProvider>,
    );
    // The offloaded run output is not in the gallery initially…
    expect(screen.queryByText("shell-output.txt")).toBeNull();
    // …until the "Show run outputs (1)" toggle is clicked.
    fireEvent.click(screen.getByText(/Show run outputs/));
    expect(screen.getByText("shell-output.txt")).toBeTruthy();
  });
});
