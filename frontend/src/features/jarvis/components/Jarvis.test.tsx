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

const mockedGetJSON = vi.mocked(await import("@/app/api")).getJSON;

afterEach(cleanup);

describe("Jarvis view", () => {
  beforeEach(() => {
    mockedGetJSON.mockReset();
    stubEngine.send.mockClear();
    stubEngine.newChat.mockClear();
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
});
