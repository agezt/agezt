// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { fmtBytes, pctOf, Storage } from "./Storage";
import { UIProvider } from "@/components/ui/feedback";

vi.mock("@/lib/usePanel", () => ({
  usePanel: () => ({
    data: {
      base_dir: "/home/u/.agezt",
      total_bytes: 1024 * 1024,
      total_files: 42,
      disk_available: true,
      disk_free_bytes: 25 * 1024 ** 3,
      disk_total_bytes: 100 * 1024 ** 3,
      disk_free_pct: 25,
      dirs: [
        { name: "journal", bytes: 768 * 1024, files: 30, label: "Append-only event log" },
        { name: "artifacts", bytes: 256 * 1024, files: 12, label: "Blob store" },
      ],
    },
    error: null,
    loading: false,
    reload: () => {},
  }),
}));

afterEach(cleanup);

describe("fmtBytes", () => {
  it("scales through the units", () => {
    expect(fmtBytes(0)).toBe("0 B");
    expect(fmtBytes(512)).toBe("512 B");
    expect(fmtBytes(2048)).toBe("2.0 KB");
    expect(fmtBytes(3 * 1024 * 1024)).toBe("3.0 MB");
    expect(fmtBytes(2 * 1024 ** 3)).toBe("2.00 GB");
  });
});

describe("pctOf", () => {
  it("returns the share and survives a zero total", () => {
    expect(pctOf(25, 100)).toBe(25);
    expect(pctOf(10, 0)).toBe(0);
  });
});

describe("Storage view", () => {
  it("renders the breakdown with labels and the collectors", () => {
    render(
      <UIProvider>
        <Storage />
      </UIProvider>,
    );
    // "journal" appears both in the breakdown row and the "Largest" summary card.
    expect(screen.getAllByText("journal").length).toBeGreaterThan(0);
    expect(screen.getByText("Append-only event log")).toBeTruthy();
    expect(screen.getByText("artifacts")).toBeTruthy();
    // Collector cards are present.
    expect(screen.getByText("Artifact collect")).toBeTruthy();
    expect(screen.getByText("Memory prune")).toBeTruthy();
    expect(screen.getByText("Memory consolidate")).toBeTruthy();
    expect(screen.getByText("Reaper scan")).toBeTruthy();
    // Disk free summary.
    expect(screen.getByText("25%")).toBeTruthy();
  });
});
