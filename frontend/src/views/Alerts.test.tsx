// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({ getJSON: (...a: unknown[]) => getJSON(...a) }));
// Empty live SSE buffer — the point of M777 is that history comes from the journal, not
// the live stream.
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ events: [], connected: true, subscribe: () => () => {} }),
}));
const focusRun = vi.fn();
vi.mock("@/lib/runfocus", () => ({ focusRun: (...a: unknown[]) => focusRun(...a) }));

import { Alerts, mergeAlerts } from "@/views/Alerts";

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  focusRun.mockReset();
});

describe("mergeAlerts (M777)", () => {
  const row = (id: string, tsMs: number) => ({ id, tsMs, level: "info" as const, title: id, detail: "", source: "x", kind: "k" });
  it("dedupes by id, sorts newest-first, and caps the list", () => {
    const merged = mergeAlerts([row("a", 1), row("b", 3)], [row("b", 3), row("c", 2)]);
    expect(merged.map((r) => r.id)).toEqual(["b", "c", "a"]); // ts 3,2,1; b not doubled
  });
});

describe("Alerts journal backfill (M777)", () => {
  it("surfaces historical alerts from the journal even when the live buffer is empty", async () => {
    getJSON.mockResolvedValue({
      events: [
        { id: "e1", kind: "task.failed", ts_unix_ms: 10, payload: { reason: "provider unavailable" } },
        { id: "e2", kind: "budget.exceeded", ts_unix_ms: 20, payload: {} },
        { id: "e3", kind: "tool.result", ts_unix_ms: 30, payload: {} }, // not an alert → ignored
      ],
    });
    render(<Alerts />);
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/journal", { limit: "500" }));
    // The two alert-worthy events surface; the non-alert event does not.
    await waitFor(() => expect(screen.getByText("run failed")).toBeTruthy());
    expect(screen.getByText("budget ceiling exceeded")).toBeTruthy();
    expect(screen.getByText(/provider unavailable/)).toBeTruthy();
  });

  it("shows the all-quiet empty state when the journal has no alert-worthy events", async () => {
    getJSON.mockResolvedValue({ events: [{ id: "e1", kind: "tool.result", ts_unix_ms: 1, payload: {} }] });
    render(<Alerts />);
    await waitFor(() => expect(getJSON).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByText(/no alerts — all quiet/)).toBeTruthy());
  });
});

describe("Alerts → open run (M781)", () => {
  it("a run-associated alert links to its run (focusRun + navigate)", async () => {
    getJSON.mockResolvedValue({
      events: [{ id: "e1", kind: "task.failed", ts_unix_ms: 10, correlation_id: "run-abc", payload: { reason: "boom" } }],
    });
    render(<Alerts />);
    const btn = await screen.findByRole("button", { name: /open run/ });
    fireEvent.click(btn);
    expect(focusRun).toHaveBeenCalledWith("run-abc");
    expect(location.hash).toBe("#runs");
  });

  it("an alert with no correlation does not show an open-run link", async () => {
    getJSON.mockResolvedValue({ events: [{ id: "e1", kind: "budget.exceeded", ts_unix_ms: 5, payload: {} }] });
    render(<Alerts />);
    await waitFor(() => expect(screen.getByText("budget ceiling exceeded")).toBeTruthy());
    expect(screen.queryByRole("button", { name: /open run/ })).toBeNull();
  });
});
