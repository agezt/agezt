// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from "vitest";

const getJSON = vi.fn();
const speechSupported = vi.fn();

vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
}));
vi.mock("@/lib/speech", () => ({
  speechSupported: () => speechSupported(),
}));

import { browserVoiceCapabilities, getVoiceReadiness } from "@/lib/voiceStatus";

describe("voice readiness", () => {
  beforeEach(() => {
    getJSON.mockReset();
    speechSupported.mockReset();
    Object.defineProperty(navigator, "mediaDevices", {
      configurable: true,
      value: { getUserMedia: vi.fn() },
    });
    vi.stubGlobal("MediaRecorder", class {});
    vi.stubGlobal("AudioContext", class {});
  });

  it("requires both browser capture and server STT for hearing", async () => {
    getJSON.mockResolvedValue({ stt: { configured: false }, tts: { configured: true } });
    speechSupported.mockReturnValue(true);

    expect(await getVoiceReadiness()).toEqual({
      serverSTT: false,
      serverTTS: true,
      browserInput: true,
      browserTTS: true,
      canListen: false,
      canSpeak: true,
    });
    expect(getJSON).toHaveBeenCalledWith("/api/voice/status");
  });

  it("accepts browser speech synthesis as a TTS fallback", async () => {
    getJSON.mockResolvedValue({ stt: { configured: true }, tts: { configured: false } });
    speechSupported.mockReturnValue(true);

    const status = await getVoiceReadiness();
    expect(status.canListen).toBe(true);
    expect(status.canSpeak).toBe(true);
  });

  it("detects a browser without microphone capture support", () => {
    Object.defineProperty(navigator, "mediaDevices", { configurable: true, value: undefined });
    expect(browserVoiceCapabilities().browserInput).toBe(false);
  });

  it("accepts the webkit AudioContext fallback", () => {
    vi.stubGlobal("AudioContext", undefined);
    (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext =
      class {} as unknown as typeof AudioContext;
    expect(browserVoiceCapabilities().browserInput).toBe(true);
    delete (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
  });
});
