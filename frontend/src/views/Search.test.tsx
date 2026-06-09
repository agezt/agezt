// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({ getJSON: (...a: unknown[]) => getJSON(...a) }));

import { CausationTrace } from "@/views/Search";

afterEach(cleanup);
beforeEach(() => getJSON.mockReset());

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
