// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor, act } from "@testing-library/react";

// Mock the network seams while keeping the rest of the store real (M907).
const postAction = vi.fn();
const getJSON = vi.fn();
vi.mock("@/lib/api", async (orig) => {
  const actual = await orig<typeof import("@/lib/api")>();
  return {
    ...actual,
    postAction: (...a: unknown[]) => postAction(...a),
    getJSON: (...a: unknown[]) => getJSON(...a),
  };
});

// streamRun is replaced with a controllable fake: it hands us the onFrame
// callback and resolves/rejects only when the abort signal fires.
let frameCb: ((f: { kind: string; correlation_id?: string }) => void) | null = null;
const streamRun = vi.fn(
  (_body: unknown, onFrame: (f: { kind: string; correlation_id?: string }) => void, signal: AbortSignal) => {
    frameCb = onFrame;
    return new Promise<void>((_resolve, reject) => {
      signal.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
    });
  },
);
vi.mock("@/lib/chat", async (orig) => {
  const actual = await orig<typeof import("@/lib/chat")>();
  return { ...actual, streamRun: (...a: unknown[]) => (streamRun as (...x: unknown[]) => unknown)(...a) };
});

// The store subscribes to the global firehose; a no-op subscription is enough.
vi.mock("@/lib/events", () => ({ useEvents: () => ({ subscribe: () => () => {} }) }));

import { ChatProvider, useChat } from "@/lib/chatStore";

function Harness() {
  const { send, stop, busy } = useChat();
  return (
    <div>
      <button onClick={() => send("hi")}>send</button>
      <button onClick={() => stop()}>stop</button>
      <span data-testid="busy">{busy ? "busy" : "idle"}</span>
    </div>
  );
}

beforeEach(() => {
  postAction.mockReset();
  getJSON.mockReset();
  streamRun.mockClear();
  frameCb = null;
  postAction.mockResolvedValue({});
  getJSON.mockResolvedValue({ model: "x" });
  localStorage.clear();
});
afterEach(cleanup);

describe("chat Stop cancels the server-side run (M907)", () => {
  it("cancels by the correlation id seen in the stream", async () => {
    render(
      <ChatProvider>
        <Harness />
      </ChatProvider>,
    );
    fireEvent.click(screen.getByText("send"));
    await waitFor(() => expect(streamRun).toHaveBeenCalledTimes(1));
    expect(screen.getByTestId("busy").textContent).toBe("busy");

    // A frame carrying the run's correlation id arrives.
    act(() => frameCb?.({ kind: "open", correlation_id: "run-42" }));

    fireEvent.click(screen.getByText("stop"));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/cancel_run", { correlation: "run-42" }),
    );
  });

  it("does not call cancel when no correlation id was seen yet", async () => {
    render(
      <ChatProvider>
        <Harness />
      </ChatProvider>,
    );
    fireEvent.click(screen.getByText("send"));
    await waitFor(() => expect(streamRun).toHaveBeenCalledTimes(1));

    // Stop before any frame with a correlation id → nothing to cancel server-side.
    fireEvent.click(screen.getByText("stop"));
    await waitFor(() => expect(screen.getByTestId("busy").textContent).toBe("idle"));
    expect(postAction).not.toHaveBeenCalledWith("/api/cancel_run", expect.anything());
  });
});
