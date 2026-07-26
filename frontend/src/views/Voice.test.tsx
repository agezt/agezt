// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor, within, act } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postJSON = vi.fn();
const getVoiceReadiness = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  authHeaders: () => new Headers(),
  withToken: (p: string) => p,
}));
vi.mock("@/lib/voiceStatus", () => ({
  getVoiceReadiness: (...a: unknown[]) => getVoiceReadiness(...a),
}));

// Keep the voice session inert in tests — we only verify the view's chrome and
// controls render and toggle, not the audio loop (covered in voiceSession.test).
const start = vi.fn();
const stop = vi.fn();
const createBrowserVoiceIO = vi.fn((..._args: unknown[]) => ({}));
let callbacks: Record<string, (...args: unknown[]) => void> = {};
let options: Record<string, unknown> = {};
vi.mock("@/lib/voiceSession", () => ({
  VoiceSession: class {
    constructor(_io: unknown, cb: Record<string, (...args: unknown[]) => void>, opts: Record<string, unknown>) {
      callbacks = cb;
      options = opts;
    }
    start = start;
    stop = stop;
  },
  createBrowserVoiceIO: (...args: unknown[]) => createBrowserVoiceIO(...args),
}));

import { Voice } from "@/views/Voice";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  getVoiceReadiness.mockReset();
  getVoiceReadiness.mockResolvedValue({
    serverSTT: true,
    serverTTS: false,
    browserInput: true,
    browserTTS: true,
    canListen: true,
    canSpeak: true,
  });
  postJSON.mockResolvedValue({ env: "AGEZT_STT_URL", saved: true, applied: "restart" });
  // Route by path: roster for the picker, plus the voice config schema/values the
  // inline VoiceSetup panel reads. Values start empty (nothing configured).
  getJSON.mockImplementation((path: string) => {
    if (path === "/api/agents")
      return Promise.resolve({
        profiles: [{ slug: "researcher", name: "Researcher" }, { slug: "ops" }, { slug: "disabled", enabled: false }],
      });
    if (path === "/api/config/values") return Promise.resolve({ fields: [] });
    if (path === "/api/config/schema") return Promise.resolve({ sections: [] });
    return Promise.resolve({});
  });
  start.mockReset();
  stop.mockReset();
  createBrowserVoiceIO.mockClear();
  callbacks = {};
  options = {};
  localStorage.clear();
});

describe("Voice view", () => {
  it("renders the header, orb prompt, and start control", async () => {
    render(withUI(<Voice />));
    expect(screen.getByRole("heading", { name: "Voice", level: 2 })).toBeTruthy();
    expect(screen.getByText(/hands-free conversation/i)).toBeTruthy();
    expect(await screen.findByRole("button", { name: /start talking/i })).toBeTruthy();
  });

  it("loads the roster into the agent picker", async () => {
    render(withUI(<Voice />));
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/agents"));
    await waitFor(() => expect(screen.getByText("Researcher")).toBeTruthy());
    fireEvent.click(within(screen.getByRole("group", { name: "Voice agent" })).getByRole("button", { name: "Researcher" }));
    expect(localStorage.getItem("agezt.voice.agent")).toBe("researcher");
  });

  it("accepts a roster response without profiles", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/agents") return Promise.resolve({});
      if (path === "/api/config/values") return Promise.resolve({ fields: [] });
      return Promise.resolve({ sections: [] });
    });
    render(withUI(<Voice />));
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/agents"));
    expect(within(screen.getByRole("group", { name: "Voice agent" })).getAllByRole("button")).toHaveLength(1);
  });

  it("starts a session and swaps to a Stop control", async () => {
    render(withUI(<Voice />));
    await waitFor(() => expect(screen.getByRole("button", { name: /start talking/i })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /start talking/i }));
    expect(start).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: /stop/i })).toBeTruthy();
  });

  it("opens setup instead of starting when transcription is not configured", async () => {
    getVoiceReadiness.mockResolvedValue({
      serverSTT: false,
      serverTTS: false,
      browserInput: true,
      browserTTS: true,
      canListen: false,
      canSpeak: true,
    });
    render(withUI(<Voice />));

    const button = await screen.findByRole("button", { name: /set up hearing/i });
    fireEvent.click(button);
    expect(start).not.toHaveBeenCalled();
    expect(screen.getAllByText("Voice setup").length).toBeGreaterThanOrEqual(1);
  });

  it("persists the wake-word toggle", () => {
    render(withUI(<Voice />));
    const sw = screen.getByRole("switch");
    expect(sw.getAttribute("aria-checked")).toBe("false");
    fireEvent.click(sw);
    expect(sw.getAttribute("aria-checked")).toBe("true");
    expect(localStorage.getItem("agezt.voice.wake")).toBe("1");
  });

  it("opens voice setup with Hearing + Voice provider pickers", async () => {
    render(withUI(<Voice />));
    fireEvent.click(screen.getByRole("button", { name: /setup/i }));
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/config/values"));
    expect(screen.getAllByText("Voice setup").length).toBeGreaterThanOrEqual(1);
    // Opened in the setup modal because nothing is configured — both halves present.
    expect(screen.getByRole("heading", { name: "Hearing" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Voice", level: 4 })).toBeTruthy();
    // Provider chips from the catalog (Groq appears in both halves).
    expect(screen.getAllByRole("button", { name: /groq/i }).length).toBeGreaterThan(0);
  });

  it("selects a provider and writes its endpoint + model to the daemon", async () => {
    render(withUI(<Voice />));
    fireEvent.click(screen.getByRole("button", { name: /setup/i }));
    await waitFor(() => expect(screen.getAllByText("Voice setup").length).toBeGreaterThanOrEqual(1));
    // Click the first OpenAI chip (Hearing half).
    fireEvent.click(screen.getAllByRole("button", { name: "OpenAI" })[0]);
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_STT_URL", value: "https://api.openai.com/v1" }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_STT_MODEL", value: "gpt-4o-transcribe" }));
  });

  it("selects a native provider and writes its dialect (ElevenLabs)", async () => {
    render(withUI(<Voice />));
    fireEvent.click(screen.getByRole("button", { name: /setup/i }));
    await waitFor(() => expect(screen.getAllByText("Voice setup").length).toBeGreaterThanOrEqual(1));
    // Second ElevenLabs chip is the Voice (TTS) half.
    fireEvent.click(screen.getAllByRole("button", { name: "ElevenLabs" })[1]);
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_TTS_PROVIDER", value: "elevenlabs" }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_TTS_URL", value: "https://api.elevenlabs.io" }));
  });

  it("does not mark hosted providers ready until their required keys are set", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/agents") return Promise.resolve({ profiles: [] });
      if (path === "/api/config/schema") return Promise.resolve({ sections: [] });
      if (path === "/api/config/values")
        return Promise.resolve({
          fields: [
            { env: "AGEZT_STT_PROVIDER", set: true, value: "openai" },
            { env: "AGEZT_STT_URL", set: true, value: "https://api.openai.com/v1" },
            { env: "AGEZT_STT_MODEL", set: true, value: "gpt-4o-transcribe" },
            { env: "AGEZT_TTS_PROVIDER", set: true, value: "elevenlabs" },
            { env: "AGEZT_TTS_URL", set: true, value: "https://api.elevenlabs.io" },
            { env: "AGEZT_TTS_MODEL", set: true, value: "eleven_multilingual_v2" },
            { env: "AGEZT_TTS_VOICE", set: true, value: "voice-id" },
          ],
        });
      return Promise.resolve({});
    });
    render(withUI(<Voice />));
    fireEvent.click(screen.getByRole("button", { name: /setup/i }));

    expect(await screen.findByText("0/2 ready")).toBeTruthy();
    expect(screen.getAllByText("not set up")).toHaveLength(2);
  });

  it("renders every live session phase, transcript role, streaming append, and fatal-error stop", async () => {
    localStorage.setItem("agezt.voice.wake", "1");
    localStorage.setItem("agezt.voice.agent", "ops");
    const scrollTo = vi.fn();
    Object.defineProperty(HTMLElement.prototype, "scrollTo", { configurable: true, value: scrollTo });
    render(withUI(<Voice />));
    const startButton = await screen.findByRole("button", { name: /start talking/i });
    fireEvent.click(startButton);

    expect(createBrowserVoiceIO).toHaveBeenCalledWith({ agent: "ops" });
    expect(options).toEqual({ wakeWords: ["agezt", "jarvis"], agent: "ops" });
    for (const [state, label] of [
      ["waking", /Listening for "agezt"/],
      ["listening", "Listening…"],
      ["thinking", "Thinking…"],
      ["speaking", "Speaking…"],
      ["idle", "Idle"],
    ] as const) {
      act(() => callbacks.onState(state));
      expect(screen.getByText(label)).toBeTruthy();
    }
    act(() => {
      callbacks.onLevel(0.9);
      callbacks.onUserText("hello");
      callbacks.onAnswerDelta("first ");
      callbacks.onAnswerDelta("second");
    });
    expect(screen.getByText("hello")).toBeTruthy();
    expect(screen.getByText("first second")).toBeTruthy();
    expect(scrollTo).toHaveBeenCalled();

    act(() => callbacks.onError("microphone lost"));
    expect(stop).toHaveBeenCalled();
    expect(await screen.findByRole("button", { name: /start talking/i })).toBeTruthy();
  });

  it("handles unavailable browser capture, readiness failure, roster failure, and modal close", async () => {
    getJSON.mockRejectedValue(new Error("roster offline"));
    getVoiceReadiness.mockRejectedValue(new Error("status offline"));
    render(withUI(<Voice />));
    const setupButton = await screen.findByRole("button", { name: /set up hearing/i });
    fireEvent.click(setupButton);
    expect(screen.getByRole("button", { name: "Close Voice setup" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Close Voice setup" }));
    expect(screen.queryByRole("button", { name: "Close Voice setup" })).toBeNull();

    cleanup();
    getJSON.mockResolvedValue({ profiles: [] });
    getVoiceReadiness.mockResolvedValue({
      serverSTT: true,
      serverTTS: false,
      browserInput: false,
      browserTTS: false,
      canListen: false,
      canSpeak: false,
    });
    render(withUI(<Voice />));
    const unsupported = await screen.findByRole("button", { name: /set up hearing/i });
    expect(screen.getByText(/microphone capture unsupported/)).toBeTruthy();
    expect(screen.getByText(/Speaking: unavailable/)).toBeTruthy();
    fireEvent.click(unsupported);
    expect(start).not.toHaveBeenCalled();
  });

  it("stops from the control and again on unmount when a session is active", async () => {
    const view = render(withUI(<Voice />));
    fireEvent.click(await screen.findByRole("button", { name: /start talking/i }));
    fireEvent.click(screen.getByRole("button", { name: /stop/i }));
    expect(stop).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: /start talking/i }));
    view.unmount();
    expect(stop).toHaveBeenCalledTimes(2);
  });

  it("shows a natural provider status when server TTS is configured", async () => {
    getVoiceReadiness.mockResolvedValue({
      serverSTT: true,
      serverTTS: true,
      browserInput: true,
      browserTTS: false,
      canListen: true,
      canSpeak: true,
    });
    render(withUI(<Voice />));
    expect(await screen.findByText("Speaking: natural provider")).toBeTruthy();
  });

  it("renders fully ready local providers without requiring keys", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/agents") return Promise.resolve({ profiles: [] });
      if (path === "/api/config/schema") return Promise.resolve({ sections: [] });
      if (path === "/api/config/values")
        return Promise.resolve({
          fields: [
            { env: "AGEZT_STT_PROVIDER", set: true, value: "openai" },
            { env: "AGEZT_STT_URL", set: true, value: "http://localhost:8000/v1" },
            { env: "AGEZT_STT_MODEL", set: true, value: "Systran/faster-whisper-large-v3" },
            { env: "AGEZT_TTS_PROVIDER", set: true, value: "openai" },
            { env: "AGEZT_TTS_URL", set: true, value: "http://localhost:8880/v1" },
            { env: "AGEZT_TTS_MODEL", set: true, value: "kokoro" },
            { env: "AGEZT_TTS_VOICE", set: true, value: "af_heart" },
          ],
        });
      return Promise.resolve({});
    });
    render(withUI(<Voice />));
    fireEvent.click(screen.getByRole("button", { name: /setup/i }));
    expect(await screen.findByText("2/2 ready")).toBeTruthy();
    expect(screen.getAllByText(/No API key needed/)).toHaveLength(2);
  });

  it("edits model, catalog voice, and API key through a selected hosted TTS provider", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/agents") return Promise.resolve({ profiles: [] });
      if (path === "/api/config/schema") return Promise.resolve({ sections: [] });
      if (path === "/api/config/values")
        return Promise.resolve({
          fields: [
            { env: "AGEZT_TTS_PROVIDER", set: true, value: "openai" },
            { env: "AGEZT_TTS_URL", set: true, value: "https://api.openai.com/v1" },
            { env: "AGEZT_TTS_MODEL", set: true, value: "gpt-4o-mini-tts" },
            { env: "AGEZT_TTS_VOICE", set: true, value: "alloy" },
            { env: "AGEZT_TTS_KEY", set: true, value: "" },
          ],
        });
      return Promise.resolve({});
    });
    render(withUI(<Voice />));
    fireEvent.click(screen.getByRole("button", { name: /setup/i }));
    const ttsCard = (await screen.findByRole("heading", { name: "Voice", level: 4 })).closest(".flex.flex-col") as HTMLElement;

    fireEvent.click(within(ttsCard).getByRole("button", { name: /TTS-1\s*fast/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_TTS_MODEL", value: "tts-1" }),
    );
    fireEvent.click(within(ttsCard).getByRole("button", { name: "echo" }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_TTS_VOICE", value: "echo" }),
    );

    const key = within(ttsCard).getByPlaceholderText(/set — type to replace/);
    fireEvent.change(key, { target: { value: "  secret-key  " } });
    fireEvent.keyDown(key, { key: "Enter" });
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_TTS_KEY", value: "secret-key" }),
    );
  });

  it("edits a free-form voice id and leaves it alone when unchanged", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/agents") return Promise.resolve({ profiles: [] });
      if (path === "/api/config/schema") return Promise.resolve({ sections: [] });
      if (path === "/api/config/values")
        return Promise.resolve({
          fields: [
            { env: "AGEZT_TTS_PROVIDER", set: true, value: "elevenlabs" },
            { env: "AGEZT_TTS_URL", set: true, value: "https://api.elevenlabs.io" },
            { env: "AGEZT_TTS_MODEL", set: true, value: "eleven_multilingual_v2" },
            { env: "AGEZT_TTS_VOICE", set: true, value: "old-voice" },
            { env: "AGEZT_TTS_KEY", set: true, value: "" },
          ],
        });
      return Promise.resolve({});
    });
    render(withUI(<Voice />));
    fireEvent.click(screen.getByRole("button", { name: /setup/i }));
    const voiceInput = await screen.findByDisplayValue("old-voice");
    fireEvent.blur(voiceInput);
    const before = postJSON.mock.calls.length;
    fireEvent.change(voiceInput, { target: { value: " new-voice " } });
    fireEvent.blur(voiceInput);
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_TTS_VOICE", value: "new-voice" }),
    );
    expect(postJSON.mock.calls.length).toBe(before + 1);
  });

  it("shows pinned fields, custom endpoints, refreshes, and surfaces save failures", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/agents") return Promise.resolve({ profiles: [] });
      if (path === "/api/config/schema")
        return Promise.resolve({
          sections: [{ id: "voice", fields: [{ env: "AGEZT_STT_URL", label: "STT URL", type: "text" }] }],
        });
      if (path === "/api/config/values")
        return Promise.resolve({
          fields: [
            { env: "AGEZT_STT_PROVIDER", set: true, value: "openai", env_pinned: true },
            { env: "AGEZT_STT_URL", set: true, value: "https://custom.example/v1", env_pinned: true },
            { env: "AGEZT_STT_MODEL", set: true, value: "custom-model" },
          ],
        });
      return Promise.resolve({});
    });
    render(withUI(<Voice />));
    fireEvent.click(screen.getByRole("button", { name: /setup/i }));
    expect(await screen.findByText(/Set from the environment/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Refresh voice setup/i }));

    cleanup();
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/agents") return Promise.resolve({ profiles: [] });
      if (path === "/api/config/values")
        return Promise.resolve({
          fields: [
            { env: "AGEZT_STT_URL", set: true, value: "https://custom.example/v1" },
            { env: "AGEZT_STT_MODEL", set: true, value: "custom-model" },
          ],
        });
      return Promise.resolve({ sections: [] });
    });
    postJSON.mockRejectedValueOnce(new Error("cannot save"));
    render(withUI(<Voice />));
    fireEvent.click(screen.getByRole("button", { name: /setup/i }));
    expect((await screen.findAllByText(/Advanced · custom endpoint/)).length).toBeGreaterThan(0);
    fireEvent.click(screen.getAllByRole("button", { name: /Custom/ })[0]);
    fireEvent.click(screen.getAllByRole("button", { name: "OpenAI" })[0]);
    await waitFor(() => expect(postJSON).toHaveBeenCalled());
  });

  it("shows environment-pinned feedback after a direct key save", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/agents") return Promise.resolve({ profiles: [] });
      if (path === "/api/config/values")
        return Promise.resolve({
          fields: [
            { env: "AGEZT_STT_PROVIDER", set: true, value: "openai" },
            { env: "AGEZT_STT_URL", set: true, value: "https://api.openai.com/v1" },
            { env: "AGEZT_STT_MODEL", set: true, value: "whisper-1" },
          ],
        });
      return Promise.resolve({ sections: [] });
    });
    postJSON.mockResolvedValueOnce({ env_pinned: true });
    render(withUI(<Voice />));
    fireEvent.click(screen.getByRole("button", { name: /setup/i }));
    const key = await screen.findByPlaceholderText(/sk-/);
    fireEvent.change(key, { target: { value: "secret" } });
    fireEvent.click(screen.getAllByRole("button", { name: "Save" })[0]);
    await waitFor(() => expect(postJSON).toHaveBeenCalled());
  });
});
