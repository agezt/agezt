// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, waitFor, fireEvent } from "@testing-library/react";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

import { Board, awaitingReply, boardAgentFilterFromHash, boardAgentMailboxSummary, boardAgentWakeIssue, boardAgentWakePlan, boardCounts, boardHashForAgentFilter, boardMessageFilterCounts, boardMessageInvolvesAgent, filterBoardMessages, filterBoardMessagesByAgent, messageAckedBy } from "@/views/Board";

afterEach(cleanup);
beforeEach(() => {
  location.hash = "";
  getJSON.mockReset();
  postJSON.mockReset();
  postJSON.mockResolvedValue({});
  postAction.mockReset();
  postAction.mockResolvedValue({});
});

describe("awaitingReply", () => {
  it("flags addressed messages with no reply; replies clear them; broadcasts never flag", () => {
    const w = awaitingReply([
      { topic: "dm", id: "q1", from: "planner", to: "researcher", text: "?" },
      { topic: "dm", id: "q2", from: "planner", to: "ops", text: "?" },
      { topic: "dm", id: "q3", from: "planner", to: "writer", text: "seen?", acked_by: ["writer"] },
      { topic: "dm", id: "r1", from: "ops", to: "planner", reply_to: "q2", text: "!" },
      { topic: "general", id: "b1", from: "x", to: "*", text: "broadcast" },
    ]);
    expect(w.has("q1")).toBe(true); // unanswered DM
    expect(w.has("q2")).toBe(false); // answered
    expect(w.has("q3")).toBe(false); // acknowledged
    expect(w.has("r1")).toBe(false); // a reply is an answer, never awaiting one
    expect(w.has("b1")).toBe(false); // broadcast
  });
});

describe("messageAckedBy", () => {
  it("formats mailbox acknowledgement provenance", () => {
    expect(messageAckedBy({ acked_by: ["researcher", "", "ops"] })).toBe("researcher, ops");
    expect(messageAckedBy({})).toBe("");
  });
});

describe("filterBoardMessages", () => {
  it("turns the shared board into operational mailbox queues", () => {
    const messages = [
      { topic: "dm", id: "q1", from: "planner", to: "researcher", text: "?" },
      { topic: "dm", id: "q2", from: "planner", to: "ops", text: "seen?", acked_by: ["ops"] },
      { topic: "general", id: "b1", from: "ops", to: "*", text: "broadcast" },
      { topic: "help", id: "h1", from: "builder", help: true, text: "blocked" },
    ];
    const waiting = awaitingReply(messages);
    expect(filterBoardMessages(messages, "awaiting", waiting).map((m) => m.id)).toEqual(["q1"]);
    expect(filterBoardMessages(messages, "dm", waiting).map((m) => m.id)).toEqual(["q1", "q2"]);
    expect(filterBoardMessages(messages, "broadcast", waiting).map((m) => m.id)).toEqual(["b1"]);
    expect(filterBoardMessages(messages, "acked", waiting).map((m) => m.id)).toEqual(["q2"]);
    expect(filterBoardMessages(messages, "help", waiting).map((m) => m.id)).toEqual(["h1"]);
    expect(boardMessageFilterCounts(messages, waiting)).toEqual({ all: 4, awaiting: 1, dm: 2, broadcast: 1, acked: 1, help: 1 });
  });

  it("filters the shared board down to one agent mailbox", () => {
    const messages = [
      { topic: "dm", id: "q1", from: "planner", to: "researcher", text: "?" },
      { topic: "dm", id: "q2", from: "ops", to: "planner", text: "done", acked_by: [" Planner "] },
      { topic: "general", id: "b1", from: "ops", to: "*", text: "broadcast" },
      { topic: "dm", id: "q3", from: "writer", to: "editor", text: "draft" },
    ];
    expect(boardMessageInvolvesAgent(messages[0], "researcher")).toBe(true);
    expect(boardMessageInvolvesAgent(messages[1], "planner")).toBe(true);
    expect(boardMessageInvolvesAgent(messages[1], "PLANNER")).toBe(true);
    expect(boardMessageInvolvesAgent(messages[2], "researcher")).toBe(true);
    expect(filterBoardMessagesByAgent(messages, "researcher").map((m) => m.id)).toEqual(["q1", "b1"]);
    expect(filterBoardMessagesByAgent(messages, "planner").map((m) => m.id)).toEqual(["q1", "q2", "b1"]);
    expect(filterBoardMessagesByAgent(messages, "").map((m) => m.id)).toEqual(["q1", "q2", "b1", "q3"]);
  });
});

describe("boardAgentFilterFromHash", () => {
  it("reads agent context from board deep links", () => {
    expect(boardAgentFilterFromHash("#board?agent=researcher")).toBe("researcher");
    expect(boardAgentFilterFromHash("#/board?agent=ops%2Flead")).toBe("ops/lead");
    expect(boardAgentFilterFromHash("#board")).toBe("");
  });

  it("builds bookmarkable agent mailbox links", () => {
    expect(boardHashForAgentFilter("researcher")).toBe("#board?agent=researcher");
    expect(boardHashForAgentFilter(" ops/lead ")).toBe("#board?agent=ops%2Flead");
    expect(boardHashForAgentFilter("")).toBe("#board");
  });
});

describe("boardCounts", () => {
  it("summarizes the board as an inter-agent mailbox cockpit", () => {
    const data = {
      count: 4,
      topics: { dm: 3, help: 1 },
      messages: [
        { topic: "dm", id: "q1", from: "planner", to: "researcher", text: "?" },
        { topic: "dm", id: "q2", from: "planner", to: "ops", text: "?" },
        { topic: "dm", id: "r1", from: "ops", to: "planner", reply_to: "q2", text: "!" },
      ],
    };
    expect(boardCounts(data, [{ topic: "help", id: "h1", from: "builder", text: "blocked" }])).toEqual({
      messages: 4,
      topics: 2,
      awaiting: 1,
      help: 1,
    });
  });
});

describe("boardAgentWakeIssue", () => {
  it("keeps mailbox wake aligned with roster lifecycle and hierarchy", () => {
    expect(boardAgentWakeIssue({ slug: "ops" })).toBe("");
    expect(boardAgentWakeIssue({ slug: "ops", enabled: false })).toBe("resume this agent before waking it");
    expect(boardAgentWakeIssue({ slug: "ops", retired: true })).toBe("revive this agent before waking it");
    expect(boardAgentWakeIssue({ slug: "worker", direct_callable: false, parent_agent: "lead" })).toBe(
      "managed sub-agent; wake lead instead",
    );
    expect(boardAgentWakeIssue({ slug: "planner-child", kind: "subagent", parent_agent: "lead" })).toBe(
      "managed sub-agent; wake lead instead",
    );
  });
});

describe("boardAgentWakePlan", () => {
  it("wakes a managed recipient's parent while keeping the message addressed to the child", () => {
    expect(boardAgentWakePlan({ slug: "researcher" }, [])).toEqual({
      ref: "researcher",
      label: "Wake recipient",
      issue: "",
    });
    expect(boardAgentWakePlan({ slug: "worker", direct_callable: false, parent_agent: "lead" }, [])).toEqual({
      ref: "lead",
      label: "Wake lead",
      issue: "",
    });
    expect(boardAgentWakePlan({ slug: "planner-child", kind: "subagent", parent_agent: "lead" }, [])).toEqual({
      ref: "lead",
      label: "Wake lead",
      issue: "",
    });
    expect(
      boardAgentWakePlan(
        { slug: "worker", direct_callable: false, parent_agent: "lead" },
        [{ slug: "lead", enabled: false }],
      ),
    ).toEqual({
      ref: "",
      label: "Wake lead",
      issue: "manager lead: resume this agent before waking it",
    });
  });
});

describe("boardAgentMailboxSummary", () => {
  it("summarizes one agent mailbox with waiting, ack, and wake routing", () => {
    const messages = [
      { topic: "dm", id: "q1", from: "planner", to: "worker", text: "deploy target?" },
      { topic: "dm", id: "q2", from: "worker", to: "planner", text: "done", acked_by: ["planner"] },
      { topic: "general", id: "b1", from: "ops", to: "*", text: "all hands", acked_by: ["worker"] },
      { topic: "dm", id: "r1", from: "lead", to: "planner", reply_to: "q2", text: "checked" },
    ];
    expect(
      boardAgentMailboxSummary(messages, "worker", [
        { slug: "worker", direct_callable: false, parent_agent: "lead" },
        { slug: "lead" },
      ]),
    ).toMatchObject({
      value: "worker mailbox: 1 waiting",
      detail: "2 received · 1 sent · 1 acked · wake lead -> lead",
      tone: "warn",
      waiting: 1,
      received: 2,
      sent: 1,
      acked: 1,
      wake: "wake lead -> lead",
    });
  });
});

describe("Board", () => {
  it("renders DM addressing, reply linkage, and the awaiting-reply badge", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/board/help") return Promise.resolve({ open_help: [{ topic: "help", id: "h1", from: "builder", text: "blocked" }] });
      return Promise.resolve({
        count: 3,
        topics: { dm: 3 },
        messages: [
          { topic: "dm", id: "q1", from: "planner", to: "researcher", text: "deploy target?", ts_unix_ms: 3 },
          { topic: "dm", id: "q2", from: "planner", to: "ops", text: "disk ok?", ts_unix_ms: 2 },
          { topic: "dm", id: "r1", from: "ops", to: "planner", reply_to: "q2", text: "yes", ts_unix_ms: 1 },
          { topic: "dm", id: "q3", from: "planner", to: "writer", text: "confirm?", acked_by: ["writer"], ts_unix_ms: 4 },
        ],
      });
    });
    render(<Board />);
    await waitFor(() => expect(screen.getByText("deploy target?")).toBeTruthy());
    // Recipients render as → chips.
    expect(screen.getByText("researcher")).toBeTruthy();
    // The reply is marked and the answered question carries no badge; the
    // unanswered one does (exactly one badge on the board).
    expect(screen.getByText("reply")).toBeTruthy();
    expect(screen.getByText("seen by writer")).toBeTruthy();
    expect(screen.getAllByText("awaiting reply")).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: /Awaiting1/ }));
    expect(screen.getByText("deploy target?")).toBeTruthy();
    expect(screen.queryByText("confirm?")).toBeNull();
    expect(screen.getByText("awaiting")).toBeTruthy();
    expect(screen.getAllByText("help").length).toBeGreaterThan(0);
    expect(screen.getAllByText("1").length).toBeGreaterThanOrEqual(2);
  });

  it("filters the board as a selected agent mailbox", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/board/help") return Promise.resolve({ open_help: [] });
      if (path === "/api/agents") return Promise.resolve({ profiles: [{ slug: "researcher" }, { slug: "ops" }] });
      return Promise.resolve({
        count: 4,
        topics: { dm: 3, general: 1 },
        messages: [
          { topic: "dm", id: "q1", from: "planner", to: "researcher", text: "deploy target?", ts_unix_ms: 4 },
          { topic: "dm", id: "q2", from: "ops", to: "planner", text: "done", ts_unix_ms: 3 },
          { topic: "general", id: "b1", from: "ops", to: "*", text: "all hands", ts_unix_ms: 2 },
          { topic: "dm", id: "q3", from: "writer", to: "editor", text: "draft", ts_unix_ms: 1 },
        ],
      });
    });
    render(<Board />);
    await waitFor(() => expect(screen.getByText("deploy target?")).toBeTruthy());

    fireEvent.change(screen.getByLabelText("Filter by agent"), { target: { value: "researcher" } });
    expect(location.hash).toBe("#board?agent=researcher");
    expect(screen.getByText("researcher mailbox: 1 waiting")).toBeTruthy();
    expect(screen.getByText("2 received · 0 sent · 0 acked · wake recipient -> researcher")).toBeTruthy();
    expect(screen.getByText("deploy target?")).toBeTruthy();
    expect(screen.getByText("all hands")).toBeTruthy();
    expect(screen.queryByText("done")).toBeNull();
    expect(screen.queryByText("draft")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /DM1/ }));
    expect(screen.getByText("deploy target?")).toBeTruthy();
    expect(screen.queryByText("all hands")).toBeNull();
  });

  it("opens with the agent filter from the board hash", async () => {
    location.hash = "#board?agent=researcher";
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/board/help") return Promise.resolve({ open_help: [] });
      if (path === "/api/agents") return Promise.resolve({ profiles: [{ slug: "researcher" }, { slug: "ops" }] });
      return Promise.resolve({
        count: 2,
        topics: { dm: 2 },
        messages: [
          { topic: "dm", id: "q1", from: "planner", to: "researcher", text: "deploy target?", ts_unix_ms: 2 },
          { topic: "dm", id: "q2", from: "ops", to: "planner", text: "not yours", ts_unix_ms: 1 },
        ],
      });
    });
    render(<Board />);
    await waitFor(() => expect(screen.getByText("deploy target?")).toBeTruthy());

    expect((screen.getByLabelText("Filter by agent") as HTMLSelectElement).value).toBe("researcher");
    expect(screen.queryByText("not yours")).toBeNull();
  });

  it("acknowledges a visible mailbox message as the selected agent", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/board/help") return Promise.resolve({ open_help: [] });
      if (path === "/api/agents") return Promise.resolve({ profiles: [{ slug: "researcher" }] });
      return Promise.resolve({
        count: 1,
        topics: { dm: 1 },
        messages: [
          { topic: "dm", id: "q1", from: "planner", to: "researcher", text: "deploy target?", ts_unix_ms: 4 },
        ],
      });
    });
    render(<Board />);
    await waitFor(() => expect(screen.getByText("deploy target?")).toBeTruthy());

    fireEvent.change(screen.getByLabelText("Filter by agent"), { target: { value: "researcher" } });
    fireEvent.click(screen.getByRole("button", { name: "Ack as researcher" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/board/ack", {
        id: "q1",
        by: "researcher",
      }),
    );
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/board", { limit: "200" }));
  });

  it("replies to a visible mailbox message as the selected agent", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/board/help") return Promise.resolve({ open_help: [] });
      if (path === "/api/agents") return Promise.resolve({ profiles: [{ slug: "researcher" }] });
      return Promise.resolve({
        count: 1,
        topics: { dm: 1 },
        messages: [
          { topic: "dm", id: "q1", from: "planner", to: "researcher", text: "deploy target?", ts_unix_ms: 4 },
        ],
      });
    });
    render(<Board />);
    await waitFor(() => expect(screen.getByText("deploy target?")).toBeTruthy());

    fireEvent.change(screen.getByLabelText("Filter by agent"), { target: { value: "researcher" } });
    fireEvent.click(screen.getByRole("button", { name: "Reply as researcher" }));
    fireEvent.change(screen.getByLabelText("Reply to q1"), { target: { value: "ship us-east" } });
    fireEvent.click(screen.getByRole("button", { name: "Send reply" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/board/send", {
        from: "researcher",
        reply_to: "q1",
        text: "ship us-east",
      }),
    );
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/board", { limit: "200" }));
  });

  it("sends an operator DM into the shared mailbox", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/board/help") return Promise.resolve({ open_help: [] });
      return Promise.resolve({ count: 0, topics: {}, messages: [] });
    });
    render(<Board />);
    await waitFor(() => expect(screen.getByText("New message")).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: "New message" }));
    fireEvent.change(screen.getByLabelText("To"), { target: { value: "researcher" } });
    fireEvent.change(screen.getByLabelText("Topic"), { target: { value: "dm" } });
    fireEvent.change(screen.getByLabelText("Message text"), { target: { value: "check the inbox" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/board/send", {
        from: "operator",
        to: "researcher",
        topic: "dm",
        text: "check the inbox",
      }),
    );
  });

  it("uses roster agents as mailbox recipients when available", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/board/help") return Promise.resolve({ open_help: [] });
      if (path === "/api/agents") return Promise.resolve({ profiles: [{ slug: "researcher", name: "Researcher" }, { slug: "ops", enabled: false }] });
      return Promise.resolve({ count: 0, topics: {}, messages: [] });
    });
    render(<Board />);
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/agents"));

    fireEvent.click(screen.getByRole("button", { name: "New message" }));
    const to = (await screen.findByLabelText("To")) as HTMLSelectElement;
    expect(to.value).toBe("researcher");
    fireEvent.change(to, { target: { value: "ops" } });
    fireEvent.change(screen.getByLabelText("Message text"), { target: { value: "wake when ready" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/board/send", {
        from: "operator",
        to: "ops",
        topic: "dm",
        text: "wake when ready",
      }),
    );
  });

  it("can wake the recipient after dropping a mailbox message", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/board/help") return Promise.resolve({ open_help: [] });
      if (path === "/api/agents") return Promise.resolve({ profiles: [{ slug: "researcher", name: "Researcher" }] });
      return Promise.resolve({ count: 0, topics: {}, messages: [] });
    });
    render(<Board />);
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/agents"));

    fireEvent.click(screen.getByRole("button", { name: "New message" }));
    fireEvent.change(screen.getByLabelText("Message text"), { target: { value: "please handle your inbox" } });
    fireEvent.click(screen.getByLabelText("Wake recipient"));
    fireEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/agents/wake", {
        ref: "researcher",
        reason: "mailbox message",
      }),
    );
  });

  it("does not offer direct wake for paused mailbox recipients", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/board/help") return Promise.resolve({ open_help: [] });
      if (path === "/api/agents") return Promise.resolve({ profiles: [{ slug: "ops", enabled: false }] });
      return Promise.resolve({ count: 0, topics: {}, messages: [] });
    });
    render(<Board />);
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/agents"));

    fireEvent.click(screen.getByRole("button", { name: "New message" }));
    const wake = screen.getByLabelText("Wake recipient") as HTMLInputElement;
    expect(wake.disabled).toBe(true);
    expect(screen.getByText("resume this agent before waking it")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Message text"), { target: { value: "message only" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/board/send", expect.objectContaining({ to: "ops" })));
    expect(postAction).not.toHaveBeenCalled();
  });

  it("wakes a managed recipient's parent after leaving the mailbox message on the child", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/board/help") return Promise.resolve({ open_help: [] });
      if (path === "/api/agents")
        return Promise.resolve({
          profiles: [
            { slug: "worker", name: "Worker", direct_callable: false, parent_agent: "lead" },
            { slug: "lead", name: "Lead" },
          ],
        });
      return Promise.resolve({ count: 0, topics: {}, messages: [] });
    });
    render(<Board />);
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/agents"));

    fireEvent.click(screen.getByRole("button", { name: "New message" }));
    fireEvent.change(screen.getByLabelText("Message text"), { target: { value: "child inbox item" } });
    expect(screen.getByText("Wake lead")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("Wake recipient"));
    fireEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/board/send", expect.objectContaining({ to: "worker" })),
    );
    expect(postAction).toHaveBeenCalledWith("/api/agents/wake", {
      ref: "lead",
      reason: "mailbox message",
    });
  });

  it("caps the topic chips and filters them when there are many (M829)", async () => {
    const topics: Record<string, number> = {};
    for (let i = 0; i < 30; i++) topics[`topic-${i}`] = 30 - i; // 30 topics, sorted by count
    getJSON.mockResolvedValue({ count: 1, topics, messages: [{ topic: "topic-0", from: "a", text: "hi", ts_unix_ms: 1 }] });
    render(<Board />);

    // The search box appears (>12 topics), and only the first 24 chips show + a "+6 more".
    const search = await screen.findByLabelText("Filter topics");
    expect(screen.getByText("+6 more")).toBeTruthy();
    expect(screen.queryByText("topic-23")).toBeTruthy(); // 24th by count, within the cap
    expect(screen.queryByText("topic-29")).toBeNull(); // tail, capped out

    // Filtering narrows to the match (a tail topic becomes reachable).
    fireEvent.change(search, { target: { value: "topic-29" } });
    await waitFor(() => expect(screen.getByText("topic-29")).toBeTruthy());
    expect(screen.queryByText("topic-23")).toBeNull();
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
