// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const deps = vi.hoisted(() => ({
  transcribeAudio: vi.fn(),
  streamRun: vi.fn(),
  ttsSpeak: vi.fn(),
}));

vi.mock("@/lib/voice", () => ({ transcribeAudio: deps.transcribeAudio }));
vi.mock("@/lib/chat", () => ({ streamRun: deps.streamRun }));
vi.mock("@/lib/tts", () => ({ speak: deps.ttsSpeak }));

import { createBrowserVoiceIO } from "@/lib/voiceSession";

class FakeAnalyser {
  fftSize = 0;
  samples: number[] = [];
  getByteTimeDomainData(buf: Uint8Array) {
    const value = this.samples.shift() ?? 128;
    buf.fill(value);
  }
}

class FakeAudioContext {
  static instances: FakeAudioContext[] = [];
  analyser = new FakeAnalyser();
  close = vi.fn(async () => {});
  createMediaStreamSource = vi.fn(() => ({ connect: vi.fn() }));
  createAnalyser = vi.fn(() => this.analyser);
  constructor() {
    FakeAudioContext.instances.push(this);
  }
}

class FakeMediaRecorder {
  static instances: FakeMediaRecorder[] = [];
  mimeType = "audio/webm";
  ondataavailable: ((e: { data: Blob }) => void) | null = null;
  onstop: (() => void) | null = null;
  start = vi.fn();
  stop = vi.fn(() => {
    this.ondataavailable?.({ data: new Blob(["audio"]) });
    this.onstop?.();
  });
  constructor(public stream: MediaStream) {
    FakeMediaRecorder.instances.push(this);
  }
}

function installAudioBrowser() {
  const track = { stop: vi.fn() };
  const stream = { getTracks: () => [track] } as unknown as MediaStream;
  const getUserMedia = vi.fn(async () => stream);
  Object.defineProperty(navigator, "mediaDevices", {
    configurable: true,
    value: { getUserMedia },
  });
  vi.stubGlobal("AudioContext", FakeAudioContext);
  vi.stubGlobal("MediaRecorder", FakeMediaRecorder);
  return { track, stream, getUserMedia };
}

beforeEach(() => {
  vi.useFakeTimers();
  deps.transcribeAudio.mockReset().mockResolvedValue("transcript");
  deps.streamRun.mockReset().mockResolvedValue(undefined);
  deps.ttsSpeak.mockReset().mockResolvedValue({ stop: vi.fn(), done: Promise.resolve() });
  FakeAudioContext.instances = [];
  FakeMediaRecorder.instances = [];
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  delete (window as unknown as { webkitSpeechRecognition?: unknown }).webkitSpeechRecognition;
  delete (window as unknown as { SpeechRecognition?: unknown }).SpeechRecognition;
});

describe("createBrowserVoiceIO", () => {
  it("captures speech, reuses the mic, proxies STT/run/TTS, detects barge-in, and releases resources", async () => {
    const { track, getUserMedia } = installAudioBrowser();
    const io = createBrowserVoiceIO({
      agent: "ops",
      model: "model-x",
      speechThreshold: 0.01,
      silenceMs: 50,
      minSpeechMs: 0,
      maxUtteranceMs: 1000,
    });
    const levels: number[] = [];
    const captureAbort = new AbortController();
    const capturePromise = io.capture({ signal: captureAbort.signal, onLevel: (v) => levels.push(v) });
    await vi.advanceTimersByTimeAsync(1);
    FakeAudioContext.instances[0].analyser.samples.push(200, 200, 128);
    await vi.advanceTimersByTimeAsync(170);
    const blob = await capturePromise;

    expect(blob?.size).toBeGreaterThan(0);
    expect(levels.some((v) => v > 0)).toBe(true);
    expect(getUserMedia).toHaveBeenCalledTimes(1);
    expect(FakeMediaRecorder.instances[0].start).toHaveBeenCalled();

    await expect(io.transcribe(blob!)).resolves.toBe("transcript");
    expect(deps.transcribeAudio).toHaveBeenCalledWith(blob, "utterance.webm");
    await io.run("hello", vi.fn(), new AbortController().signal);
    expect(deps.streamRun).toHaveBeenCalledWith(
      { intent: "hello", agent: "ops", model: "model-x" },
      expect.any(Function),
      expect.any(AbortSignal),
    );
    const frameHandler = deps.streamRun.mock.calls[0][1];
    const onDelta = vi.fn();
    deps.streamRun.mockImplementationOnce(async (_input, cb) => {
      cb({ kind: "meta", payload: { text: "ignore" } });
      cb({ kind: "llm.token", payload: null });
      cb({ kind: "llm.token", payload: { text: 42 } });
    });
    await io.run("again", onDelta, new AbortController().signal);
    expect(onDelta).toHaveBeenCalledWith("42");
    expect(frameHandler).toBeTypeOf("function");
    await io.speak("reply");
    expect(deps.ttsSpeak).toHaveBeenCalledWith("reply");

    FakeAudioContext.instances[0].analyser.samples.push(128, 200, 200, 200, 200, 200);
    const barge = io.watchBargeIn({ signal: new AbortController().signal, onLevel: vi.fn() });
    await vi.advanceTimersByTimeAsync(260);
    await barge;
    expect(getUserMedia).toHaveBeenCalledTimes(1);

    io.release?.();
    expect(track.stop).toHaveBeenCalled();
    expect(FakeAudioContext.instances[0].close).toHaveBeenCalled();
  });

  it("rejects capture when microphone APIs are unavailable", async () => {
    Object.defineProperty(navigator, "mediaDevices", { configurable: true, value: undefined });
    const io = createBrowserVoiceIO();
    await expect(io.capture({ signal: new AbortController().signal, onLevel: vi.fn() })).rejects.toThrow(
      "microphone not available",
    );
  });

  it("returns null when capture is aborted before speech", async () => {
    installAudioBrowser();
    const io = createBrowserVoiceIO();
    const abort = new AbortController();
    const result = io.capture({ signal: abort.signal, onLevel: vi.fn() });
    await vi.advanceTimersByTimeAsync(1);
    abort.abort();
    await vi.advanceTimersByTimeAsync(60);
    await expect(result).resolves.toBeNull();
  });

  it("finishes at the utterance hard cap and rejects an empty recording", async () => {
    installAudioBrowser();
    class EmptyRecorder extends FakeMediaRecorder {
      stop = vi.fn(() => this.onstop?.());
    }
    vi.stubGlobal("MediaRecorder", EmptyRecorder);
    const io = createBrowserVoiceIO({ speechThreshold: 0.01, maxUtteranceMs: 50 });
    const result = io.capture({ signal: new AbortController().signal, onLevel: vi.fn() });
    await vi.advanceTimersByTimeAsync(1);
    FakeAudioContext.instances[0].analyser.samples.push(200);
    await vi.advanceTimersByTimeAsync(60);
    await expect(result).resolves.toBeNull();
  });

  it("ignores empty recorder chunks and falls back when the recorder has no MIME type", async () => {
    installAudioBrowser();
    class MixedRecorder extends FakeMediaRecorder {
      mimeType = "";
      stop = vi.fn(() => {
        this.ondataavailable?.({ data: new Blob() });
        this.ondataavailable?.({ data: new Blob(["audio"]) });
        this.onstop?.();
      });
    }
    vi.stubGlobal("MediaRecorder", MixedRecorder);
    const io = createBrowserVoiceIO({ speechThreshold: 0.01, maxUtteranceMs: 50 });
    const result = io.capture({ signal: new AbortController().signal, onLevel: vi.fn() });
    await vi.advanceTimersByTimeAsync(1);
    FakeAudioContext.instances[0].analyser.samples.push(200);
    await vi.advanceTimersByTimeAsync(60);
    const blob = await result;
    expect(blob?.type).toBe("audio/webm");
    expect(blob?.size).toBeGreaterThan(0);
  });

  it("survives recorder-stop and release cleanup failures", async () => {
    const { stream } = installAudioBrowser();
    class ThrowingRecorder extends FakeMediaRecorder {
      stop = vi.fn(() => {
        throw new Error("already stopped");
      });
    }
    vi.stubGlobal("MediaRecorder", ThrowingRecorder);
    const io = createBrowserVoiceIO({ speechThreshold: 0.01, maxUtteranceMs: 50 });
    const result = io.capture({ signal: new AbortController().signal, onLevel: vi.fn() });
    await vi.advanceTimersByTimeAsync(1);
    FakeAudioContext.instances[0].analyser.samples.push(200);
    await vi.advanceTimersByTimeAsync(60);
    await expect(result).resolves.toBeNull();
    vi.spyOn(stream, "getTracks").mockImplementation(() => {
      throw new Error("gone");
    });
    expect(() => io.release?.()).not.toThrow();
  });

  it("uses webkit AudioContext when the standard constructor is absent", async () => {
    installAudioBrowser();
    vi.stubGlobal("AudioContext", undefined);
    (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext =
      FakeAudioContext as unknown as typeof AudioContext;
    const io = createBrowserVoiceIO();
    const abort = new AbortController();
    const result = io.capture({ signal: abort.signal, onLevel: vi.fn() });
    await vi.advanceTimersByTimeAsync(1);
    abort.abort();
    await vi.advanceTimersByTimeAsync(60);
    await result;
    expect(FakeAudioContext.instances).toHaveLength(1);
  });

  it("spots wake words and covers recognition error, end, abort, and start-failure paths", async () => {
    installAudioBrowser();
    const instances: FakeRecognition[] = [];
    class FakeRecognition {
      continuous = false;
      interimResults = false;
      lang = "";
      onresult: ((e: { results: ArrayLike<ArrayLike<{ transcript: string }>> }) => void) | null = null;
      onerror: (() => void) | null = null;
      onend: (() => void) | null = null;
      abort = vi.fn();
      start = vi.fn();
      stop = vi.fn();
      constructor() {
        instances.push(this);
      }
    }
    (window as unknown as { SpeechRecognition: typeof FakeRecognition }).SpeechRecognition = FakeRecognition;
    vi.spyOn(navigator, "language", "get").mockReturnValue("");
    const io = createBrowserVoiceIO();
    expect(io.awaitWake).toBeTypeOf("function");

    const first = io.awaitWake!(["jarvis"], { signal: new AbortController().signal, onLevel: vi.fn() });
    const active = instances.at(-1)!;
    active.onresult?.({
      results: [[], [{ transcript: "" }], [{ transcript: "hello" }], [{ transcript: "hey Jarvis!" }]],
    });
    await expect(first).resolves.toBe(true);
    active.onerror?.();
    expect(active.abort).toHaveBeenCalledTimes(1);

    const second = io.awaitWake!(["jarvis"], { signal: new AbortController().signal, onLevel: vi.fn() });
    instances.at(-1)!.onerror?.();
    await expect(second).resolves.toBe(false);

    const third = io.awaitWake!(["jarvis"], { signal: new AbortController().signal, onLevel: vi.fn() });
    instances.at(-1)!.onend?.();
    await expect(third).resolves.toBe(false);

    const abort = new AbortController();
    const fourth = io.awaitWake!(["jarvis"], { signal: abort.signal, onLevel: vi.fn() });
    abort.abort();
    await expect(fourth).resolves.toBe(false);

    instances.length = 0;
    class StartFailure extends FakeRecognition {
      start = vi.fn(() => {
        throw new Error("denied");
      });
      abort = vi.fn(() => {
        throw new Error("gone");
      });
    }
    (window as unknown as { SpeechRecognition: typeof StartFailure }).SpeechRecognition = StartFailure;
    const failing = createBrowserVoiceIO();
    await expect(
      failing.awaitWake!(["jarvis"], { signal: new AbortController().signal, onLevel: vi.fn() }),
    ).resolves.toBe(false);
  });
});
