// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({ getJSON: (...a: unknown[]) => getJSON(...a) }));
const downloadText = vi.fn();
vi.mock("@/lib/export", () => ({ downloadText: (...a: unknown[]) => downloadText(...a) }));

import { CausationTrace, JournalIntegrity, JournalExport, journalExportBundle } from "@/views/Search";

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  downloadText.mockReset();
});

describe("JournalIntegrity (M759)", () => {
  it("reports an intact chain after a successful verify", async () => {
    getJSON.mockResolvedValue({ ok: true });
    render(<JournalIntegrity />);
    expect(screen.getByRole("button", { name: /verify integrity/ })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /verify integrity/ }));
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/journal/verify"));
    await waitFor(() => expect(screen.getByText("chain intact")).toBeTruthy());
  });

  it("reports a broken chain (and surfaces the error in the title) on failure", async () => {
    getJSON.mockRejectedValueOnce(new Error("hash mismatch at seq 42"));
    render(<JournalIntegrity />);
    fireEvent.click(screen.getByRole("button", { name: /verify integrity/ }));
    await waitFor(() => expect(screen.getByText("chain broken")).toBeTruthy());
    expect(screen.getByRole("button").getAttribute("title")).toBe("hash mismatch at seq 42");
  });
});

describe("CausationTrace (M755)", () => {
  it("loads /api/why on demand and renders the causal chain root→this", async () => {
    getJSON.mockResolvedValue({
      correlation: "run-1",
      causation_chain: [
        { id: "c0", kind: "pulse.tick", subject: "heartbeat", ts_unix_ms: 1 },
        { id: "c1", kind: "initiative.raised", subject: "noticed a failure", ts_unix_ms: 2 },
        { id: "c2", kind: "tool.result", subject: "ran the fix", ts_unix_ms: 3 },
      ],
    });
    render(<CausationTrace eventId="c2" />);
    // Collapsed initially — no chain shown.
    expect(screen.queryByText("pulse.tick")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /trace cause/ }));
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/why", { event_id: "c2" }));
    await waitFor(() => expect(screen.getByText("pulse.tick")).toBeTruthy());
    expect(screen.getByText("initiative.raised")).toBeTruthy();
    expect(screen.getByText("tool.result")).toBeTruthy();
    // The root marker and the "this" marker on the last link.
    expect(screen.getByText("root")).toBeTruthy();
    expect(screen.getByText("this")).toBeTruthy();
  });

  it("reports a root cause when there is no upstream chain", async () => {
    getJSON.mockResolvedValue({ correlation: "run-1", causation_chain: [] });
    render(<CausationTrace eventId="e1" />);
    fireEvent.click(screen.getByRole("button", { name: /trace cause/ }));
    await waitFor(() => expect(screen.getByText(/root cause/)).toBeTruthy());
  });

  it("surfaces a sub-agent's parent run", async () => {
    getJSON.mockResolvedValue({
      causation_chain: [{ id: "x", kind: "run.started", subject: "child", ts_unix_ms: 1 }],
      parent_correlation: "abcdef1234567890",
    });
    render(<CausationTrace eventId="x" />);
    fireEvent.click(screen.getByRole("button", { name: /trace cause/ }));
    await waitFor(() => expect(screen.getByText(/parent/)).toBeTruthy());
    expect(screen.getByText("34567890")).toBeTruthy(); // last 8 of the parent id
  });

  it("toggles the trace closed again without refetching", async () => {
    getJSON.mockResolvedValue({ causation_chain: [{ id: "a", kind: "k", subject: "s", ts_unix_ms: 1 }] });
    render(<CausationTrace eventId="a" />);
    fireEvent.click(screen.getByRole("button", { name: /trace cause/ }));
    await waitFor(() => expect(screen.getByText("k")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /hide cause/ }));
    await waitFor(() => expect(screen.queryByText("k")).toBeNull());
    // Re-open: no second fetch (cached).
    fireEvent.click(screen.getByRole("button", { name: /trace cause/ }));
    await waitFor(() => expect(screen.getByText("k")).toBeTruthy());
    expect(getJSON).toHaveBeenCalledTimes(1);
  });

  it("surfaces a fetch error", async () => {
    getJSON.mockRejectedValueOnce(new Error("event not found"));
    render(<CausationTrace eventId="nope" />);
    fireEvent.click(screen.getByRole("button", { name: /trace cause/ }));
    await waitFor(() => expect(screen.getByText("event not found")).toBeTruthy());
  });
});

describe("JournalExport (M772)", () => {
  it("wraps the export payload into a self-describing, offline-verifiable bundle", () => {
    const bundle = JSON.parse(
      journalExportBundle({ events: [{ seq: 1 }, { seq: 2 }], count: 2, head_seq: 2, head_hash: "abc", first_seq: 1, last_seq: 2 }),
    );
    expect(bundle.version).toBe(1);
    expect(bundle.kind).toBe("agezt-journal-export");
    expect(bundle.head_hash).toBe("abc");
    expect(bundle.count).toBe(2);
    expect(bundle.events).toHaveLength(2);
  });

  it("downloads the journal bundle on click and reports the event count", async () => {
    getJSON.mockResolvedValue({ events: [{ seq: 1 }], count: 1, head_seq: 1, head_hash: "h" });
    render(<JournalExport />);
    fireEvent.click(screen.getByRole("button", { name: /export journal/ }));
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/journal/export"));
    await waitFor(() => expect(downloadText).toHaveBeenCalled());
    expect(downloadText.mock.calls[0][0]).toBe("agezt-journal.json");
    expect(downloadText.mock.calls[0][2]).toBe("application/json");
    await waitFor(() => expect(screen.getByRole("button").getAttribute("title")).toBe("1 events"));
  });

  it("surfaces an export error in the button title", async () => {
    getJSON.mockRejectedValueOnce(new Error("journal locked"));
    render(<JournalExport />);
    fireEvent.click(screen.getByRole("button", { name: /export journal/ }));
    await waitFor(() => expect(screen.getByRole("button").getAttribute("title")).toBe("journal locked"));
  });
});
