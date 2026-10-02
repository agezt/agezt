// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { UIProvider } from "@/components/ui/feedback";

const { getJSON, postAction, listenerRef } = vi.hoisted(() => ({
  getJSON: vi.fn(),
  postAction: vi.fn(),
  listenerRef: { current: null as null | ((e: { kind?: string }) => void) },
}));

vi.mock("@/app/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

// Approvals subscribes to the firehose so a decision made elsewhere lands in the
// queue without a poll. The listener is captured here so a test can fire one.
vi.mock("@/app/events", () => ({
  useEvents: () => ({
    subscribe: (fn: (e: { kind?: string }) => void) => {
      listenerRef.current = fn;
      return () => {
        listenerRef.current = null;
      };
    },
  }),
}));

import Approvals from "./Approvals";

// The human-in-the-loop queue is the one screen where an operator's click grants
// or denies a real action, and it had no test at all — the audit measured 35 of
// 38 views covered, and this was one of the three that were not.
describe("Approvals", () => {
  beforeEach(() => {
    getJSON.mockReset();
    postAction.mockReset();
    listenerRef.current = null;
    postAction.mockResolvedValue({});
    getJSON.mockResolvedValue({ pending: [], history: [] });
  });
  afterEach(cleanup);

  const renderIt = () =>
    render(
      <UIProvider>
        <Approvals />
      </UIProvider>,
    );

  it("shows the pending count and each queued capability", async () => {
    getJSON.mockResolvedValue({
      pending: [
        { id: "a1", capability: "shell.exec", reason: "install deps" },
        { id: "a2", capability: "net.fetch" },
      ],
      history: [],
    });
    renderIt();
    await waitFor(() => expect(screen.getByText("shell.exec — install deps")).toBeTruthy());
    expect(screen.getByText("net.fetch")).toBeTruthy();
    // The badge counts what is actually waiting.
    expect(screen.getByText("2")).toBeTruthy();
  });

  it("says the policy engine is auto-deciding when nothing is queued", async () => {
    renderIt();
    await waitFor(() => expect(screen.getByText(/No agent is waiting on your eyes/i)).toBeTruthy());
  });

  // The `tool:` line only earns its space when it says something the capability
  // name does not already say.
  it("shows the tool name only when it differs from the capability", async () => {
    getJSON.mockResolvedValue({
      pending: [
        { id: "a1", capability: "shell.exec", tool_name: "bash" },
        { id: "a2", capability: "shell.exec", tool_name: "shell.exec" },
      ],
      history: [],
    });
    renderIt();
    await waitFor(() => expect(screen.getByText(/bash/)).toBeTruthy());
    expect(screen.getAllByText(/^tool:$/)).toHaveLength(1);
  });

  it("grants: posts the decision, toasts, and reloads", async () => {
    getJSON.mockResolvedValue({ pending: [{ id: "a1", capability: "shell.exec" }], history: [] });
    renderIt();
    await waitFor(() => expect(screen.getByText("shell.exec")).toBeTruthy());
    const callsBefore = getJSON.mock.calls.length;

    fireEvent.click(screen.getByRole("button", { name: "Grant" }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/decide", { id: "a1", decision: "grant" }));
    expect(await screen.findByText("Granted")).toBeTruthy();
    // The queue is re-read so a granted item leaves the list without a poll.
    await waitFor(() => expect(getJSON.mock.calls.length).toBeGreaterThan(callsBefore));
  });

  it("denies: posts the decision and toasts", async () => {
    getJSON.mockResolvedValue({ pending: [{ id: "a1", capability: "shell.exec" }], history: [] });
    renderIt();
    await waitFor(() => expect(screen.getByText("shell.exec")).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: "Deny" }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/decide", { id: "a1", decision: "deny" }));
    expect(await screen.findByText("Denied")).toBeTruthy();
  });

  it("reports a failed decision instead of pretending it worked", async () => {
    postAction.mockRejectedValue(new Error("policy service unavailable"));
    getJSON.mockResolvedValue({ pending: [{ id: "a1", capability: "shell.exec" }], history: [] });
    renderIt();
    await waitFor(() => expect(screen.getByText("shell.exec")).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: "Grant" }));
    expect(await screen.findByText("policy service unavailable")).toBeTruthy();
    expect(screen.queryByText("Granted")).toBeNull();
    // And the buttons come back: a failure must not leave the row stuck busy.
    // (Plain .disabled, not toBeDisabled — this project does not load
    // @testing-library/jest-dom.)
    await waitFor(() =>
      expect((screen.getByRole("button", { name: "Grant" }) as HTMLButtonElement).disabled).toBe(false),
    );
  });

  it("surfaces a load failure as a banner rather than an empty queue", async () => {
    // The dangerous failure here is the quiet one: a fetch that throws would
    // otherwise render as "nothing is waiting", which reads as good news.
    getJSON.mockRejectedValue(new Error("daemon unreachable"));
    renderIt();
    expect(await screen.findByText("daemon unreachable")).toBeTruthy();
    expect(screen.queryByText(/No agent is waiting on your eyes/i)).toBeNull();
  });

  it("filters history by decision, and says why pending shows nothing", async () => {
    getJSON.mockResolvedValue({
      pending: [],
      history: [
        { id: "h1", decision: "granted", summary: "install deps", ts: 2 },
        { id: "h2", decision: "denied", summary: "curl evil.test", ts: 1 },
      ],
    });
    renderIt();
    await waitFor(() => expect(screen.getByText(/Switch to/i)).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: "granted" }));
    expect(await screen.findByText("install deps")).toBeTruthy();
    expect(screen.queryByText("curl evil.test")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "denied" }));
    expect(await screen.findByText("curl evil.test")).toBeTruthy();
    expect(screen.queryByText("install deps")).toBeNull();
  });

  it("says so when a filter has no records, instead of showing a blank panel", async () => {
    getJSON.mockResolvedValue({ pending: [], history: [{ id: "h1", decision: "granted", ts: 1 }] });
    renderIt();
    await waitFor(() => expect(screen.getByText(/Switch to/i)).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "denied" }));
    expect(await screen.findByText(/No denied decisions on record yet/i)).toBeTruthy();
  });

  it("reloads when an approval event arrives, and ignores unrelated ones", async () => {
    getJSON.mockResolvedValue({ pending: [], history: [] });
    renderIt();
    await waitFor(() => expect(listenerRef.current).toBeTruthy());
    const callsBefore = getJSON.mock.calls.length;

    await act(async () => {
      listenerRef.current?.({ kind: "run.started" });
    });
    expect(getJSON.mock.calls.length).toBe(callsBefore);

    await act(async () => {
      listenerRef.current?.({ kind: "approval.granted" });
    });
    await waitFor(() => expect(getJSON.mock.calls.length).toBeGreaterThan(callsBefore));
  });

  it("reloads on demand from the header button", async () => {
    renderIt();
    await waitFor(() => expect(screen.getByRole("button", { name: "Reload" })).toBeTruthy());
    const callsBefore = getJSON.mock.calls.length;
    fireEvent.click(screen.getByRole("button", { name: "Reload" }));
    await waitFor(() => expect(getJSON.mock.calls.length).toBeGreaterThan(callsBefore));
  });
});