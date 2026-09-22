// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import Jarvis from "./Jarvis";
import { UIProvider } from "@/components/ui/feedback";

vi.mock("@/app/api", () => ({
  getJSON: vi.fn(),
  postAction: vi.fn().mockResolvedValue({}),
  postJSON: vi.fn().mockResolvedValue({}),
}));

const stubEngine: any = {
  send: vi.fn(),
  newChat: vi.fn(),
};
vi.mock("@/lib/chatStore", () => ({
  useChat: () => stubEngine,
  __esModule: true,
}));

// Day 28+1 navigation regression: previously Jarvis "Open chat", "Open Voice",
// and QuickPrompt buttons all *toasted* instead of navigating — operators had
// to know to manually click Talk › Chat. Now they call goToView("chat"|"voice")
// from @/lib/nav. We assert that import-and-call, not the hash itself, so the
// test stays decoupled from the implementation of the hash router.
const mockedGoToView = vi.fn();
vi.mock("@/lib/nav", () => ({
  goToView: (...args: unknown[]) => mockedGoToView(...args),
}));

const mockedGetJSON = vi.mocked(await import("@/app/api")).getJSON;

afterEach(() => {
  cleanup();
  // reset hash between tests so an earlier goToView's #chat doesn't leak into
  // a fresh render that expects a different default
  if (typeof window !== "undefined") window.location.hash = "";
});

describe("Jarvis view", () => {
  beforeEach(() => {
    mockedGetJSON.mockReset();
    stubEngine.send.mockClear();
    stubEngine.newChat.mockClear();
    mockedGoToView.mockReset();
  });

  it("shows the operator-side companion intro when no agents are loaded", async () => {
    mockedGetJSON.mockImplementation(async () => ({ agents: [], stt: { configured: true }, tts: { configured: true } }));
    render(<UIProvider><Jarvis /></UIProvider>);
    await waitFor(() => expect(screen.getByText(/Agents ready/i)).toBeTruthy());
    // The empty-state hint in the agents rail is the right thing to assert —
    // "0/0" appears twice (counts and stat) which trips getByText.
    expect(screen.getByText(/No roster yet/i)).toBeTruthy();
  });

  it("renders quick prompts that send to the chat engine", async () => {
    mockedGetJSON.mockImplementation(async () => ({ agents: [], stt: { configured: true }, tts: { configured: true } }));
    render(<UIProvider><Jarvis /></UIProvider>);
    await waitFor(() => expect(screen.getByText(/Agents ready/i)).toBeTruthy());
    fireEvent.click(screen.getByText(/Summarize what changed/i));
    expect(stubEngine.send).toHaveBeenCalledWith("Summarize what changed in the last 6 hours");
  });

  // Day 28+1 navigate regression: clicking a QuickPrompt used to leave the
  // operator on Jarvis with only a toast hint to "switch to Chat to see the
  // stream". Now it both sends the prompt and navigates to Talk › Chat.
  it("QuickPrompt navigates to the chat view after sending", async () => {
    mockedGetJSON.mockImplementation(async () => ({ agents: [], stt: { configured: true }, tts: { configured: true } }));
    render(<UIProvider><Jarvis /></UIProvider>);
    await waitFor(() => expect(screen.getByText(/Summarize what changed/i)).toBeTruthy());
    fireEvent.click(screen.getByText(/Summarize what changed/i));
    expect(stubEngine.send).toHaveBeenCalledWith("Summarize what changed in the last 6 hours");
    expect(mockedGoToView).toHaveBeenCalledWith("chat");
  });

  it("shows the recent-runs empty-state when no runs are returned", async () => {
    mockedGetJSON.mockImplementation(async () => ({ agents: [], runs: [], stt: { configured: true }, tts: { configured: true } }));
    render(<UIProvider><Jarvis /></UIProvider>);
    await waitFor(() => expect(screen.getByText(/No recent runs/i)).toBeTruthy());
  });

  it("renders run rows when the daemon returns recent runs", async () => {
    mockedGetJSON.mockImplementation(async (path: string) => {
      if (path === "/api/agents") return { agents: [{ slug: "g", ready: true }] };
      if (path === "/api/runs") return { runs: [{ id: "r1", status: "completed", intent: "summarise", correlation_id: "01ABCDEF" }] };
      if (path === "/api/voice/status") return { stt: { configured: true }, tts: { configured: true } };
      return {};
    });
    render(<UIProvider><Jarvis /></UIProvider>);
    await waitFor(() => expect(screen.getByText("summarise")).toBeTruthy());
    expect(screen.getByText("completed")).toBeTruthy();
  });

  it("shows the Voice-needs-setup card when STT and TTS are not configured", async () => {
    mockedGetJSON.mockImplementation(async (path: string) => {
      if (path === "/api/agents") return { agents: [] };
      if (path === "/api/runs") return { runs: [] };
      if (path === "/api/voice/status") return { stt: { configured: false }, tts: { configured: false } };
      return {};
    });
    render(<UIProvider><Jarvis /></UIProvider>);
    await waitFor(() => {
      expect(screen.getByText(/Voice needs setup/i)).toBeTruthy();
      expect(screen.getAllByText(/provider not configured/i).length).toBeGreaterThan(0);
    });
  });

  // Day 28+1 navigate regression: the Voice-needs-setup card's "Open Talk › Voice"
  // button used to *toast* — the operator had to find Talk › Voice manually in
  // the nav rail. Now it actually navigates.
  it("'Open Talk › Voice' button navigates to the voice view", async () => {
    mockedGetJSON.mockImplementation(async (path: string) => {
      if (path === "/api/agents") return { agents: [] };
      if (path === "/api/runs") return { runs: [] };
      if (path === "/api/voice/status") return { stt: { configured: false }, tts: { configured: false } };
      return {};
    });
    render(<UIProvider><Jarvis /></UIProvider>);
    await waitFor(() => expect(screen.getByText(/Voice needs setup/i)).toBeTruthy());
    const btn = screen.getByRole("button", { name: /Open Talk › Voice/i });
    fireEvent.click(btn);
    expect(mockedGoToView).toHaveBeenCalledWith("voice");
  });

  // Day 28+1 navigate regression: the right-rail "Open chat" button used to
  // *toast* — operator had to manually click Talk › Chat. Now it actually
  // navigates, after starting a fresh thread.
  it("'Open chat' button starts a fresh thread and navigates to chat", async () => {
    mockedGetJSON.mockImplementation(async () => ({ agents: [], stt: { configured: true }, tts: { configured: true } }));
    render(<UIProvider><Jarvis /></UIProvider>);
    await waitFor(() => expect(screen.getByText(/Need to ask something longer/i)).toBeTruthy());
    const btn = screen.getByRole("button", { name: /Open chat/i });
    fireEvent.click(btn);
    expect(stubEngine.newChat).toHaveBeenCalledOnce();
    expect(mockedGoToView).toHaveBeenCalledWith("chat");
  });
});
