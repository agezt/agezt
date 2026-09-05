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

  it("shows provenance and phase badges for doctor-family alerts", async () => {
    getJSON.mockResolvedValue({
      events: [
        {
          id: "e1",
          kind: "info",
          subject: "doctor.auto_repair",
          ts_unix_ms: 10,
          payload: {
            agent: "builder",
            mode: "degraded",
            phase: "failed",
            error: "provider timeout",
          },
        },
        {
          id: "e2",
          kind: "info",
          subject: "doctor.auto_repair",
          ts_unix_ms: 20,
          payload: {
            agent: "builder",
            phase: "delegation_failed",
            delegate_to: "infra-lead",
            reason: "target agent infra-lead is paused",
          },
        },
      ],
    });
    render(<Alerts />);
    await waitFor(() =>
      expect(screen.getByText("doctor run failed")).toBeTruthy(),
    );
    expect(screen.getAllByText("doctor").length).toBeGreaterThan(0);
    expect(screen.getByText("failed")).toBeTruthy();
    expect(screen.getByText("delegate failed")).toBeTruthy();
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

// ────────── Row identity for unidentifiable alerts ──────────
//
// rowOf() used to mint `${kind}-${seq ?? Math.random()}`, so an event carrying
// neither id nor seq got a BRAND NEW identity on every call. That id is the
// mergeAlerts dedup key, the React key and the localStorage dismissal key at once
// — so dismissing such an alert persisted an id that could never match again and
// the alert resurfaced on the next mount, forever.

describe("Alerts dismissal — events with no id and no seq", () => {
  const orphan = (n: number) => ({
    kind: "task.failed",
    ts_unix_ms: n,
    subject: `agent:r${n}`,
    correlation_id: `corr-${n}`,
    payload: { reason: "boom" },
  });

  // Dismissals live in localStorage under a shared key; clear so each case
  // starts from an unacknowledged feed.
  beforeEach(() => localStorage.clear());

  it("keeps an id-less, seq-less alert dismissed across a remount", async () => {
    getJSON.mockResolvedValue({ events: [orphan(1)] });
    const ui = render(<Alerts />);
    await screen.findByText("run failed");
    fireEvent.click(screen.getByTitle(/Dismiss — acknowledged/));
    await screen.findByText(/no alerts — all quiet/);

    const stored = JSON.parse(localStorage.getItem("agezt.alerts.dismissed.v1") || "[]");
    expect(stored).toHaveLength(1);
    // A float here is Math.random() having crept back in.
    expect(stored[0]).toMatch(/^task\.failed-anon-[0-9a-z]+$/);

    // Remount: dismissed set is re-read from storage and the identity is
    // re-derived. The second feed ALSO carries a new orphan, which proves the
    // ordinal is pure content and not a positional index (the seed list, each
    // SSE append and the journal backfill are merged and re-sorted, so any
    // position-derived id would drift and silently un-dismiss the row).
    getJSON.mockResolvedValue({ events: [orphan(1), orphan(2)] });
    ui.unmount();
    render(<Alerts />);
    await waitFor(() => expect(getJSON).toHaveBeenCalledTimes(2));
    // Exactly one row survives: orphan(1) is still dismissed (its identity
    // re-derived to the SAME anon id) while the brand-new orphan(2) is shown.
    // A position-derived ordinal would have un-dismissed orphan(1) here.
    expect(screen.getAllByText("run failed")).toHaveLength(1);
    expect(localStorage.getItem("agezt.alerts.dismissed.v1")).toBe(JSON.stringify(stored));
  });

  it("collapses one id-less event delivered by two paths into a single row", async () => {
    getJSON.mockResolvedValue({ events: [orphan(3), orphan(3)] });
    render(<Alerts />);
    await screen.findByText("run failed");
    await new Promise((r) => setTimeout(r, 0));
    expect(screen.getAllByText("run failed")).toHaveLength(1);
  });

  it("keeps two DISTINCT id-less events as two separate dismissable rows", async () => {
    getJSON.mockResolvedValue({ events: [orphan(4), orphan(5)] });
    render(<Alerts />);
    await waitFor(() => expect(screen.getAllByText("run failed")).toHaveLength(2));
    fireEvent.click(screen.getAllByTitle(/Dismiss — acknowledged/)[0]);
    await waitFor(() => expect(screen.getAllByText("run failed")).toHaveLength(1));
  });
});
