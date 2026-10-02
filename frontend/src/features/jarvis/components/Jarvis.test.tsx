// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
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

  // ---- Coverage ratchet: the states the assertions above never reached. ----
  // The 100% ratchet in vitest.voice-coverage.config.ts had been pointing at
  // files that no longer existed, so none of this was being measured. These
  // cases are the ones every other test in this file mocks away: a rejecting
  // endpoint, a payload with the key missing, and the per-capability
  // combinations the setup card can actually land in.

  it("says the voice endpoint is unreachable when the status request rejects", async () => {
    mockedGetJSON.mockImplementation(async (path: string) => {
      if (path === "/api/agents") return { agents: [] };
      if (path === "/api/runs") return { runs: [] };
      if (path === "/api/voice/status") throw new Error("daemon offline");
      return {};
    });
    render(<UIProvider><Jarvis /></UIProvider>);
    // The card still renders — and says "unreachable", which is a different
    // claim from "reachable but unconfigured". Conflating the two would tell an
    // operator their provider is fine when the daemon is simply not answering.
    await waitFor(() => expect(screen.getByText(/Voice needs setup/i)).toBeTruthy());
    expect(screen.getByText(/Voice status endpoint unreachable/i)).toBeTruthy();
  });

  it("treats a null voice status as an empty report instead of crashing", async () => {
    mockedGetJSON.mockImplementation(async (path: string) => {
      if (path === "/api/agents") return { agents: [] };
      if (path === "/api/runs") return { runs: [] };
      if (path === "/api/voice/status") return null;
      return {};
    });
    render(<UIProvider><Jarvis /></UIProvider>);
    await waitFor(() => expect(screen.getByText(/Voice needs setup/i)).toBeTruthy());
    expect(screen.getByText(/Voice status endpoint reachable/i)).toBeTruthy();
  });

  // The card only renders when at least one of STT/TTS is unready, so the
  // "ready" arm of each capability's label is reachable only in the mirrored
  // half-configured case. Both are asserted because the card's whole job is to
  // tell you which half is missing and which is fine.
  it("reports STT as ready with its provider while TTS is unconfigured but named", async () => {
    mockedGetJSON.mockImplementation(async (path: string) => {
      if (path === "/api/agents") return { agents: [] };
      if (path === "/api/runs") return { runs: [] };
      if (path === "/api/voice/status") {
        return { stt: { configured: true, provider: "whisper.cpp" }, tts: { configured: false, provider: "piper" } };
      }
      return {};
    });
    render(<UIProvider><Jarvis /></UIProvider>);
    await waitFor(() => expect(screen.getByText(/Voice needs setup/i)).toBeTruthy());
    expect(screen.getByText("ready")).toBeTruthy();
    // Two capabilities, so the unconfigured label can land on either half.
    expect(screen.getAllByText(/provider not configured/i).length).toBeGreaterThan(0);
    // The provider name is shown for an unconfigured capability too: it is the
    // env var the operator has to fill in.
    expect(screen.getByText("(whisper.cpp)")).toBeTruthy();
    expect(screen.getByText("(piper)")).toBeTruthy();
  });

  it("reports TTS as ready with its provider while STT is unconfigured but named", async () => {
    mockedGetJSON.mockImplementation(async (path: string) => {
      if (path === "/api/agents") return { agents: [] };
      if (path === "/api/runs") return { runs: [] };
      if (path === "/api/voice/status") {
        return { stt: { configured: false, provider: "whisper.cpp" }, tts: { configured: true, provider: "piper" } };
      }
      return {};
    });
    render(<UIProvider><Jarvis /></UIProvider>);
    await waitFor(() => expect(screen.getByText(/Voice needs setup/i)).toBeTruthy());
    expect(screen.getByText("ready")).toBeTruthy();
    expect(screen.getAllByText(/provider not configured/i).length).toBeGreaterThan(0);
    expect(screen.getByText("(piper)")).toBeTruthy();
  });

  it("treats an agents payload with no agents key as an empty roster", async () => {
    mockedGetJSON.mockImplementation(async (path: string) => {
      // No `agents` key at all — the daemon answered, it just had nothing to say.
      if (path === "/api/agents") return {};
      if (path === "/api/runs") return {};
      if (path === "/api/voice/status") return { stt: { configured: true }, tts: { configured: true } };
      return {};
    });
    render(<UIProvider><Jarvis /></UIProvider>);
    await waitFor(() => expect(screen.getByText(/No roster yet/i)).toBeTruthy());
  });

  it("colours each run status and labels a run that carries no intent", async () => {
    mockedGetJSON.mockImplementation(async (path: string) => {
      if (path === "/api/agents") return { agents: [] };
      if (path === "/api/runs") {
        return {
          runs: [
            { id: "r1", status: "running", intent: "live" },
            { id: "r2", status: "completed", intent: "done" },
            { id: "r3", status: "failed", intent: "broke" },
            { id: "r4", status: "queued" },
          ],
        };
      }
      if (path === "/api/voice/status") return { stt: { configured: true }, tts: { configured: true } };
      return {};
    });
    render(<UIProvider><Jarvis /></UIProvider>);
    await waitFor(() => expect(screen.getByText("(no intent)")).toBeTruthy());
    expect(screen.getByText("queued")).toBeTruthy();
    // One dot colour per status, asserted on the class rather than the label:
    // running/completed/failed/anything-else each take a different branch.
    expect(document.querySelector(".bg-accent")).toBeTruthy();
    expect(document.querySelector(".bg-good")).toBeTruthy();
    expect(document.querySelector(".bg-bad")).toBeTruthy();
    expect(document.querySelector(".bg-muted")).toBeTruthy();
  });

  it("shows an unready agent's reported status, and 'offline' when it reports none", async () => {
    mockedGetJSON.mockImplementation(async (path: string) => {
      if (path === "/api/agents") {
        return {
          agents: [
            { slug: "a", name: "Alpha", ready: true },
            { slug: "b", name: "Beta", ready: false, status: "degraded" },
            { slug: "c", name: "Gamma", ready: false },
          ],
        };
      }
      if (path === "/api/runs") return { runs: [] };
      if (path === "/api/voice/status") return { stt: { configured: true }, tts: { configured: true } };
      return {};
    });
    render(<UIProvider><Jarvis /></UIProvider>);
    await waitFor(() => expect(screen.getByText("Alpha")).toBeTruthy());
    expect(screen.getByText("ready")).toBeTruthy();
    expect(screen.getByText("degraded")).toBeTruthy();
    expect(screen.getByText("offline")).toBeTruthy();
  });

  // The `if (!stop)` guard in each of the three polling hooks exists for exactly
  // this race: the operator navigates away while a request is still in flight,
  // and the daemon answers afterwards. React no longer warns about a state
  // update after unmount, so nothing else would notice a regression — the guard
  // would simply stop being the thing that runs, and the view would keep a live
  // subscription to a component that no longer exists.
  //
  // This is also the only way to reach the false arm of those guards, which is
  // why the 100% branch ratchet needed it: every other test in this file lets
  // its requests settle while the view is still mounted.
  it("does not touch state when requests resolve after unmount", async () => {
    const deferred = new Map<string, { resolve: (v: unknown) => void; reject: (e: unknown) => void }>();
    mockedGetJSON.mockImplementation(
      (path: string) =>
        new Promise((resolve, reject) => {
          deferred.set(path, { resolve, reject });
        }),
    );
    const errors: unknown[][] = [];
    const spy = vi.spyOn(console, "error").mockImplementation((...args: unknown[]) => {
      errors.push(args);
    });
    try {
      // Two mount/unmount cycles, because each hook fires its request once per
      // mount: the first cycle lets all three resolve late, the second lets the
      // voice request fail late. The catch guard and the success guard are
      // separate branches and only one of them runs per mount.
      for (const voiceOutcome of ["resolve", "reject"] as const) {
        deferred.clear();
        const { unmount } = render(<UIProvider><Jarvis /></UIProvider>);
        await waitFor(() => expect(deferred.size).toBe(3));

        unmount();

        // The daemon answers now — for a view that is no longer on screen.
        deferred.get("/api/agents")?.resolve({ agents: [{ slug: "late", ready: true }] });
        deferred.get("/api/runs")?.resolve({ runs: [{ id: "late", status: "running" }] });
        if (voiceOutcome === "resolve") deferred.get("/api/voice/status")?.resolve({ stt: { configured: true } });
        else deferred.get("/api/voice/status")?.reject(new Error("too late"));
        await act(async () => {
          await Promise.resolve();
        });
        cleanup();
      }

      expect(errors).toEqual([]);
    } finally {
      spy.mockRestore();
    }
  });
});
