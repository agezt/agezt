// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({ getJSON: (...a: unknown[]) => getJSON(...a) }));

import { Board, awaitingReply } from "@/views/Board";

afterEach(cleanup);
beforeEach(() => getJSON.mockReset());

describe("awaitingReply", () => {
  it("flags addressed messages with no reply; replies clear them; broadcasts never flag", () => {
    const w = awaitingReply([
      { topic: "dm", id: "q1", from: "planner", to: "researcher", text: "?" },
      { topic: "dm", id: "q2", from: "planner", to: "ops", text: "?" },
      { topic: "dm", id: "r1", from: "ops", to: "planner", reply_to: "q2", text: "!" },
      { topic: "general", id: "b1", from: "x", text: "broadcast" },
    ]);
    expect(w.has("q1")).toBe(true); // unanswered DM
    expect(w.has("q2")).toBe(false); // answered
    expect(w.has("r1")).toBe(false); // a reply is an answer, never awaiting one
    expect(w.has("b1")).toBe(false); // broadcast
  });
});

describe("Board", () => {
  it("renders DM addressing, reply linkage, and the awaiting-reply badge", async () => {
    getJSON.mockResolvedValue({
      count: 3,
      topics: { dm: 3 },
      messages: [
        { topic: "dm", id: "q1", from: "planner", to: "researcher", text: "deploy target?", ts_unix_ms: 3 },
        { topic: "dm", id: "q2", from: "planner", to: "ops", text: "disk ok?", ts_unix_ms: 2 },
        { topic: "dm", id: "r1", from: "ops", to: "planner", reply_to: "q2", text: "yes", ts_unix_ms: 1 },
      ],
    });
    render(<Board />);
    await waitFor(() => expect(screen.getByText("deploy target?")).toBeTruthy());
    // Recipients render as → chips.
    expect(screen.getByText("researcher")).toBeTruthy();
    // The reply is marked and the answered question carries no badge; the
    // unanswered one does (exactly one badge on the board).
    expect(screen.getByText("reply")).toBeTruthy();
    expect(screen.getAllByText("awaiting reply")).toHaveLength(1);
  });

  it("renders post text as markdown (M820): bold + list, not raw asterisks", async () => {
    getJSON.mockResolvedValue({
      count: 1,
      topics: { general: 1 },
      messages: [{ topic: "general", from: "planner", text: "**done**\n\n- step one\n- step two", ts_unix_ms: 1 }],
    });
    render(<Board />);
    // "done" renders inside a <strong>, not as literal "**done**".
    const strong = await waitFor(() => screen.getByText("done"));
    expect(strong.tagName).toBe("STRONG");
    expect(screen.queryByText("**done**")).toBeNull();
    // The bullets render as list items.
    expect(screen.getByText("step one").closest("li")).toBeTruthy();
    expect(screen.getByText("step two").closest("li")).toBeTruthy();
  });
});
