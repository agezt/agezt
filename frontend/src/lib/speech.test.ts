// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { speak, stopSpeaking, speechSupported } from "@/lib/speech";

// jsdom has no speech synthesis; install a minimal fake.
class FakeUtterance {
  text: string;
  onend: (() => void) | null = null;
  onerror: (() => void) | null = null;
  constructor(text: string) {
    this.text = text;
  }
}

function installSpeech() {
  const spoken: FakeUtterance[] = [];
  const synth = {
    speak: vi.fn((u: FakeUtterance) => spoken.push(u)),
    cancel: vi.fn(),
    speaking: false,
  };
  vi.stubGlobal("speechSynthesis", synth);
  vi.stubGlobal("SpeechSynthesisUtterance", FakeUtterance as unknown);
  // `in window` check needs window to carry it too.
  (window as unknown as { speechSynthesis: unknown }).speechSynthesis = synth;
  return { synth, spoken };
}

afterEach(() => {
  vi.unstubAllGlobals();
  delete (window as unknown as { speechSynthesis?: unknown }).speechSynthesis;
});

describe("speech", () => {
  it("reports unsupported when the API is absent", () => {
    expect(speechSupported()).toBe(false);
  });

  it("speaks the text, cancelling anything already in flight first", () => {
    const { synth, spoken } = installSpeech();
    expect(speechSupported()).toBe(true);
    speak("Done — the light is off.");
    expect(synth.cancel).toHaveBeenCalled();
    expect(spoken).toHaveLength(1);
    expect(spoken[0].text).toBe("Done — the light is off.");
  });

  it("does not speak empty text but still resolves onEnd", () => {
    const { synth } = installSpeech();
    const onEnd = vi.fn();
    speak("   ", onEnd);
    expect(synth.speak).not.toHaveBeenCalled();
    expect(onEnd).toHaveBeenCalled();
  });

  it("wires onEnd to the utterance's onend", () => {
    const { spoken } = installSpeech();
    const onEnd = vi.fn();
    speak("hello", onEnd);
    expect(spoken[0].onend).toBeTypeOf("function");
    spoken[0].onend?.();
    expect(onEnd).toHaveBeenCalled();
  });

  it("stopSpeaking cancels", () => {
    const { synth } = installSpeech();
    stopSpeaking();
    expect(synth.cancel).toHaveBeenCalled();
  });

  it("is a safe no-op when unsupported", () => {
    const onEnd = vi.fn();
    expect(() => speak("hi", onEnd)).not.toThrow();
    expect(onEnd).toHaveBeenCalled();
    expect(() => stopSpeaking()).not.toThrow();
  });
});
