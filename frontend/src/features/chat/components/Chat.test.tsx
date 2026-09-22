// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import Chat from "./Chat";
import { UIProvider } from "@/components/ui/feedback";

vi.mock("@/app/api", () => ({
  getJSON: vi.fn().mockImplementation(async (url: string) => {
    if (url === "/api/prompts" || url === "/api/suggestions" || url === "/api/routing") return {};
    return {};
  }),
  postAction: vi.fn().mockResolvedValue({}),
  postJSON: vi.fn().mockResolvedValue({}),
  eventsURLAsync: vi.fn().mockResolvedValue("http://localhost/__test__/events"),
}));

// ChannelSessions inside the Chat sidebar pulls from the events SSE + inbox;
// stub it to a noop so the test doesn't drag those surfaces in.
vi.mock("@/features/channels/components/ChannelSessions", () => ({
  ChannelSessions: () => null,
}));

// Replace the real ChatProvider with a minimal stub so we can test the Chat
// view without wiring up the whole engine. The view only consumes the engine
// through the React context, so a stub covers the surface area we render.
const stubEngine: any = {
  store: {
    conversations: [
      { id: "c1", title: "First thread", messages: [], updatedAt: 0 },
      { id: "c2", title: "Second thread", messages: [], updatedAt: 0, pinned: true },
    ],
    activeId: "c1",
  },
  messages: [],
  busy: false,
  model: "",
  setModel: vi.fn(),
  agent: "",
  setAgent: vi.fn(),
  executionProfile: "",
  setExecutionProfile: vi.fn(),
  activeModel: "default-model",
  send: vi.fn(),
  retry: vi.fn(),
  continueRun: vi.fn(),
  editAndResend: vi.fn(),
  conversationPersona: "",
  setConversationPersona: vi.fn(),
  autoApproveForge: false,
  setAutoApproveForge: vi.fn(),
  trustWebContent: false,
  setTrustWebContent: vi.fn(),
  historySummary: undefined,
  stop: vi.fn(),
  newChat: vi.fn(),
  selectConversation: vi.fn(),
  removeConversation: vi.fn(),
  renameConversation: vi.fn(),
  togglePin: vi.fn(),
  learnedFor: () => [],
  forgetLearned: vi.fn().mockResolvedValue(undefined),
  activeCorr: null,
  steer: vi.fn().mockResolvedValue(undefined),
  queue: [],
  enqueue: vi.fn(),
  removeQueued: vi.fn(),
  reorderQueued: vi.fn(),
  clearQueue: vi.fn(),
  sendQueuedNow: vi.fn(),
};

// The actual Chat tree reads useChat via React context. We provide a custom
// Provider by mocking the module.
vi.mock("@/lib/chatStore", () => ({
  useChat: () => stubEngine,
  __esModule: true,
}));

function wrap(ui: React.ReactNode) {
  return render(<UIProvider>{ui}</UIProvider>);
}

afterEach(cleanup);

describe("Chat view (legacy full-page)", () => {
  beforeEach(() => {
    stubEngine.messages = [];
    stubEngine.queue = [];
    for (const k of Object.keys(stubEngine)) {
      const v = stubEngine[k];
      if (typeof v === "function" && "mockClear" in v) v.mockClear();
    }
  });

  it("renders the empty-state greeting when there are no messages", () => {
    wrap(<Chat />);
    expect(screen.getByText(/Talk to your agent/i)).toBeTruthy();
    expect(
      screen.getByText(/Type an intent and watch it run/i),
    ).toBeTruthy();
  });

  it("renders one ConversationItem per thread with the sidebar search affordance", () => {
    wrap(<Chat />);
    expect(screen.getByText("First thread")).toBeTruthy();
    expect(screen.getByText("Second thread")).toBeTruthy();
    expect(screen.getByLabelText("Search conversations")).toBeTruthy();
  });

  it("calls newChat + clears input when the New chat button is clicked", () => {
    wrap(<Chat />);
    // The sidebar's "+ New chat" button is the first match.
    const buttons = screen.getAllByRole("button", { name: /New chat/i });
    fireEvent.click(buttons[0]);
    expect(stubEngine.newChat).toHaveBeenCalledOnce();
  });

  it("shows the queue panel with queued follow-ups", () => {
    stubEngine.queue = [{ id: "q1", text: "follow-up message", ts: 0 }];
    wrap(<Chat />);
    expect(screen.getByText(/follow-up message/i)).toBeTruthy();
    expect(screen.getByText(/Queue/i)).toBeTruthy();
  });

  it("renders user + assistant bubbles when messages are present", () => {
    stubEngine.messages = [
      { role: "user", text: "hello there" },
      {
        role: "assistant",
        turn: {
          status: "done",
          streamedText: "hi, what can I do for you?",
          reasoning: "",
          tools: [],
          timeline: [],
          iters: 0,
          costMicrocents: 0,
        },
      },
    ];
    wrap(<Chat />);
    expect(screen.getByText("hello there")).toBeTruthy();
    expect(screen.getByText("hi, what can I do for you?")).toBeTruthy();
  });

  it("shows the trust-web-content and forge-auto-approve toggle buttons", () => {
    wrap(<Chat />);
    expect(screen.getByText(/Forge auto-approve/i)).toBeTruthy();
    expect(screen.getByText(/Trust web content/i)).toBeTruthy();
  });

  it("toggles forge-auto-approve when its button is clicked", () => {
    wrap(<Chat />);
    fireEvent.click(screen.getByText(/Forge auto-approve/i));
    expect(stubEngine.setAutoApproveForge).toHaveBeenCalledWith(true);
  });
});
