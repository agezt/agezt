// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { fetchSpeech, playBlob, speak, resetServerTTS } from "@/lib/tts";

beforeEach(() => resetServerTTS());
afterEach(() => vi.restoreAllMocks());

function mockFetchOnce(impl: (url: string, init: RequestInit) => unknown) {
  vi.stubGlobal("fetch", vi.fn(impl) as unknown as typeof fetch);
}

describe("fetchSpeech", () => {
  it("returns the audio blob on success", async () => {
    const audio = new Blob([new Uint8Array([1, 2, 3])], { type: "audio/mpeg" });
    mockFetchOnce(async () => ({ ok: true, status: 200, blob: async () => audio }));
    const got = await fetchSpeech("hello");
    expect(got).toBe(audio);
  });

  it("returns null and stops probing after a 501 (TTS not configured)", async () => {
    const f = vi.fn(async () => ({ ok: false, status: 501, blob: async () => new Blob() }));
    vi.stubGlobal("fetch", f as unknown as typeof fetch);
    expect(await fetchSpeech("hi")).toBeNull();
    expect(await fetchSpeech("again")).toBeNull();
    expect(f).toHaveBeenCalledTimes(1); // second call short-circuited
    resetServerTTS();
  });

  it("returns null on a network error rather than throwing", async () => {
    mockFetchOnce(async () => {
      throw new Error("offline");
    });
    expect(await fetchSpeech("hi")).toBeNull();
  });

  it("returns null for empty text without calling the server", async () => {
    const f = vi.fn();
    vi.stubGlobal("fetch", f as unknown as typeof fetch);
    expect(await fetchSpeech("   ")).toBeNull();
    expect(f).not.toHaveBeenCalled();
  });
});

// fakeAudio captures the handlers so a test can drive playback to completion.
class FakeAudio {
  onended: (() => void) | null = null;
  onerror: (() => void) | null = null;
  paused = false;
  played = false;
  constructor(public src: string) {}
  play() {
    this.played = true;
    return Promise.resolve();
  }
  pause() {
    this.paused = true;
  }
}

describe("playBlob", () => {
  beforeEach(() => {
    vi.stubGlobal("Audio", FakeAudio as unknown as typeof Audio);
    vi.stubGlobal("URL", {
      createObjectURL: vi.fn(() => "blob:fake"),
      revokeObjectURL: vi.fn(),
    } as unknown as typeof URL);
  });

  it("plays the blob and resolves done when playback ends", async () => {
    const created: FakeAudio[] = [];
    vi.stubGlobal(
      "Audio",
      class extends FakeAudio {
        constructor(src: string) {
          super(src);
          created.push(this);
        }
      } as unknown as typeof Audio,
    );
    const u = playBlob(new Blob(["x"]));
    expect(created[0].played).toBe(true);
    let done = false;
    u.done.then(() => (done = true));
    created[0].onended?.();
    await u.done;
    expect(done).toBe(true);
    expect((URL.revokeObjectURL as unknown as ReturnType<typeof vi.fn>)).toHaveBeenCalled();
  });

  it("stop() pauses and resolves done (barge-in)", async () => {
    const u = playBlob(new Blob(["x"]));
    u.stop();
    await expect(u.done).resolves.toBeUndefined();
  });
});

describe("speak fallback", () => {
  it("uses the browser voice when server TTS is unavailable", async () => {
    // No speechSynthesis in jsdom → browserUtterance settles immediately, proving
    // speak() degrades without hanging when /api/tts returns nothing.
    mockFetchOnce(async () => ({ ok: false, status: 501, blob: async () => new Blob() }));
    const u = await speak("the lights are off");
    await expect(u.done).resolves.toBeUndefined();
    resetServerTTS();
  });
});
