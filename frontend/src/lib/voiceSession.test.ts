import { describe, it, expect, vi } from "vitest";
import { VoiceSession, type VoiceIO, type Utterance } from "@/lib/voiceSession";

// Note: Utterance is re-exported from tts via voiceSession's import; declare the
// shape locally to avoid importing the audio module.
type Deferred<T> = { promise: Promise<T>; resolve: (v: T) => void };
function deferred<T>(): Deferred<T> {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}
function immediateUtterance(onStop?: () => void): Utterance {
  return { stop: () => onStop?.(), done: Promise.resolve() };
}

// blobOf makes a non-empty fake audio blob.
function blobOf(): Blob {
  return new Blob(["audio"], { type: "audio/webm" });
}

describe("VoiceSession core loop", () => {
  it("exposes lifecycle state, ignores duplicate starts, forwards levels, and releases once stopped", async () => {
    const entered = deferred<void>();
    const release = vi.fn();
    const onLevel = vi.fn();
    let captures = 0;
    const io: VoiceIO = {
      async capture(ctx) {
        captures++;
        ctx.onLevel(0.5);
        entered.resolve();
        await new Promise<void>((resolve) => ctx.signal.addEventListener("abort", () => resolve(), { once: true }));
        return null;
      },
      transcribe: vi.fn(),
      run: vi.fn(),
      speak: vi.fn(),
      watchBargeIn: vi.fn(),
      release,
    };
    const session = new VoiceSession(io, { onLevel });
    expect(session.getState()).toBe("idle");
    expect(session.isRunning()).toBe(false);
    session.start();
    session.start();
    await entered.promise;
    expect(session.isRunning()).toBe(true);
    expect(captures).toBe(1);
    expect(onLevel).toHaveBeenCalledWith(0.5);
    session.stop();
    expect(session.getState()).toBe("idle");
    expect(session.isRunning()).toBe(false);
    expect(release).toHaveBeenCalledTimes(1);
  });

  it("runs a full turn and speaks the answer sentence-by-sentence", async () => {
    const spoken: string[] = [];
    const states: string[] = [];
    let captureCalls = 0;
    const secondCapture = deferred<void>();

    const io: VoiceIO = {
      async capture(ctx) {
        captureCalls++;
        if (captureCalls === 1) return blobOf();
        secondCapture.resolve();
        // park until the session aborts us
        await new Promise<void>((r) => ctx.signal.addEventListener("abort", () => r(), { once: true }));
        return null;
      },
      async transcribe() {
        return "turn on the lights";
      },
      async run(_intent, onDelta) {
        onDelta("Sure thing. ");
        onDelta("The lights are on.");
      },
      async speak(t) {
        spoken.push(t);
        return immediateUtterance();
      },
      async watchBargeIn(ctx) {
        // never barges in this test
        await new Promise<void>((r) => ctx.signal.addEventListener("abort", () => r(), { once: true }));
      },
    };

    const onUserText = vi.fn();
    const session = new VoiceSession(io, { onState: (s) => states.push(s), onUserText });
    session.start();
    await secondCapture.promise; // first turn completed, loop came back to listen
    session.stop();

    expect(spoken).toEqual(["Sure thing.", "The lights are on."]);
    expect(onUserText).toHaveBeenCalledWith("turn on the lights");
    expect(states).toContain("listening");
    expect(states).toContain("thinking");
    expect(states).toContain("speaking");
  });

  it("barge-in stops speaking and aborts the run", async () => {
    const states: string[] = [];
    let runAborted = false;
    const barge = deferred<void>();
    const speakingSeen = deferred<void>();
    let captureCalls = 0;

    const io: VoiceIO = {
      async capture(ctx) {
        captureCalls++;
        if (captureCalls === 1) return blobOf();
        await new Promise<void>((r) => ctx.signal.addEventListener("abort", () => r(), { once: true }));
        return null;
      },
      async transcribe() {
        return "tell me a long story";
      },
      run(_intent, onDelta, signal) {
        return new Promise<void>((resolve) => {
          onDelta("Once upon a time. ");
          if (signal.aborted) return resolve();
          signal.addEventListener(
            "abort",
            () => {
              runAborted = true;
              onDelta("This late sentence must not play.");
              resolve();
            },
            { once: true },
          );
        });
      },
      async speak() {
        // a long-playing utterance: only ends when stopped (barge-in)
        const d = deferred<void>();
        return { stop: () => d.resolve(), done: d.promise };
      },
      async watchBargeIn() {
        await barge.promise; // test triggers the barge-in
      },
    };

    const session = new VoiceSession(io, {
      onState: (s) => {
        states.push(s);
        if (s === "speaking") speakingSeen.resolve();
      },
    });
    session.start();
    await speakingSeen.promise; // it's talking now
    barge.resolve(); // user interrupts
    // give the loop a tick to abort + return to listening
    await new Promise((r) => setTimeout(r, 20));
    session.stop();

    expect(runAborted).toBe(true);
  });

  it("wake word gates a turn via the transcript and is stripped from the command", async () => {
    const utterances = ["what time is it", "agezt, what is the weather"];
    let i = 0;
    const ran: string[] = [];
    const thirdCapture = deferred<void>();

    const io: VoiceIO = {
      async capture(ctx) {
        if (i < utterances.length) return blobOf();
        thirdCapture.resolve();
        await new Promise<void>((r) => ctx.signal.addEventListener("abort", () => r(), { once: true }));
        return null;
      },
      async transcribe() {
        return utterances[i++] ?? "";
      },
      async run(intent) {
        ran.push(intent);
      },
      async speak() {
        return immediateUtterance();
      },
      async watchBargeIn(ctx) {
        await new Promise<void>((r) => ctx.signal.addEventListener("abort", () => r(), { once: true }));
      },
      // no awaitWake → transcript gating path
    };

    const session = new VoiceSession(io, {}, { wakeWords: ["agezt"] });
    session.start();
    await thirdCapture.promise;
    session.stop();

    // The non-wake utterance is ignored; the wake one runs with the name stripped.
    expect(ran).toEqual(["what is the weather"]);
  });

  it("covers empty captures/transcripts, wake-only commands, recoverable errors, and a final fragment", async () => {
    const actions: Array<"none" | "empty" | "wake" | "error" | "run"> = ["none", "empty", "wake", "error", "run"];
    const parked = deferred<void>();
    const spoken: string[] = [];
    const errors: string[] = [];
    const ran: string[] = [];
    let index = 0;
    const io: VoiceIO = {
      async capture(ctx) {
        if (index >= actions.length) {
          parked.resolve();
          await new Promise<void>((resolve) => ctx.signal.addEventListener("abort", () => resolve(), { once: true }));
          return null;
        }
        return actions[index] === "none" ? (index++, null) : blobOf();
      },
      async transcribe() {
        const action = actions[index++];
        if (action === "empty") return " ";
        if (action === "wake") return "agezt!!!";
        if (action === "error") throw new Error("provider down");
        return "agezt, finish";
      },
      async run(intent, onDelta) {
        ran.push(intent);
        onDelta("final fragment without punctuation");
      },
      async speak(text) {
        spoken.push(text);
        throw new Error("speaker unavailable");
      },
      async watchBargeIn() {
        throw new Error("barge watcher unavailable");
      },
    };
    const session = new VoiceSession(io, { onError: (message) => errors.push(message) }, { wakeWords: ["", "agezt"] });
    session.start();
    await parked.promise;
    session.stop();

    expect(errors).toContain("provider down");
    expect(ran).toEqual(["finish"]);
    expect(spoken).toEqual(["final fragment without punctuation"]);
  });

  it("uses dedicated wake detection and stops cleanly while waiting for the next wake", async () => {
    const wakeCalls = deferred<void>();
    const ran: string[] = [];
    let wakes = 0;
    const io: VoiceIO = {
      async awaitWake(_keywords, ctx) {
        wakes++;
        if (wakes === 1) return false;
        if (wakes === 2) return true;
        wakeCalls.resolve();
        await new Promise<void>((resolve) => ctx.signal.addEventListener("abort", () => resolve(), { once: true }));
        return false;
      },
      async capture() {
        return blobOf();
      },
      async transcribe() {
        return "jarvis: status";
      },
      async run(intent) {
        ran.push(intent);
      },
      async speak() {
        return immediateUtterance();
      },
      async watchBargeIn(ctx) {
        await new Promise<void>((resolve) => ctx.signal.addEventListener("abort", () => resolve(), { once: true }));
      },
    };
    const session = new VoiceSession(io, {}, { wakeWords: ["jarvis"] });
    session.start();
    await wakeCalls.promise;
    session.stop();
    expect(ran).toEqual(["status"]);
  });

  it("does not report a run failure after stop, but reports one while active", async () => {
    const firstRun = deferred<void>();
    const secondCapture = deferred<void>();
    let captures = 0;
    const errors: string[] = [];
    const io: VoiceIO = {
      async capture(ctx) {
        captures++;
        if (captures === 1) return blobOf();
        secondCapture.resolve();
        await new Promise<void>((resolve) => ctx.signal.addEventListener("abort", () => resolve(), { once: true }));
        return null;
      },
      async transcribe() {
        return "test";
      },
      async run() {
        firstRun.resolve();
        throw new Error("run failed");
      },
      async speak() {
        return immediateUtterance();
      },
      async watchBargeIn(ctx) {
        await new Promise<void>((resolve) => ctx.signal.addEventListener("abort", () => resolve(), { once: true }));
      },
    };
    const active = new VoiceSession(io, { onError: (message) => errors.push(message) });
    active.start();
    await firstRun.promise;
    await secondCapture.promise;
    active.stop();
    expect(errors).toContain("run failed");

    const stoppedRun = deferred<void>();
    const stoppedIO: VoiceIO = {
      ...io,
      capture: async () => blobOf(),
      run: async () => {
        stoppedRun.resolve();
        await Promise.resolve();
        throw "stopped failure";
      },
    };
    const stoppedErrors = vi.fn();
    const stopped = new VoiceSession(stoppedIO, { onError: stoppedErrors });
    stopped.start();
    await stoppedRun.promise;
    stopped.stop();
    await Promise.resolve();
    expect(stoppedErrors).not.toHaveBeenCalled();
  });

  it("stops safely while transcription is in flight", async () => {
    const transcribing = deferred<void>();
    const transcript = deferred<string>();
    const run = vi.fn();
    const io: VoiceIO = {
      async capture() {
        return blobOf();
      },
      async transcribe() {
        transcribing.resolve();
        return transcript.promise;
      },
      run,
      async speak() {
        return immediateUtterance();
      },
      watchBargeIn: vi.fn(),
    };
    const session = new VoiceSession(io);
    session.start();
    await transcribing.promise;
    session.stop();
    transcript.resolve("too late");
    await Promise.resolve();
    expect(run).not.toHaveBeenCalled();
  });

  it("reports non-Error loop failures using their string representation", async () => {
    const parked = deferred<void>();
    let calls = 0;
    const onError = vi.fn();
    const io: VoiceIO = {
      async capture(ctx) {
        calls++;
        if (calls === 1) throw "raw failure";
        parked.resolve();
        await new Promise<void>((resolve) => ctx.signal.addEventListener("abort", () => resolve(), { once: true }));
        return null;
      },
      transcribe: vi.fn(),
      run: vi.fn(),
      speak: vi.fn(),
      watchBargeIn: vi.fn(),
    };
    const session = new VoiceSession(io, { onError });
    session.start();
    await parked.promise;
    session.stop();
    expect(onError).toHaveBeenCalledWith("raw failure");
  });

  it("does not report a loop failure that arrives after stop", async () => {
    const entered = deferred<void>();
    let rejectCapture!: (reason: unknown) => void;
    const captureResult = new Promise<Blob | null>((_resolve, reject) => {
      rejectCapture = reject;
    });
    const onError = vi.fn();
    const io: VoiceIO = {
      async capture() {
        entered.resolve();
        return captureResult;
      },
      transcribe: vi.fn(),
      run: vi.fn(),
      speak: vi.fn(),
      watchBargeIn: vi.fn(),
    };
    const session = new VoiceSession(io, { onError });
    session.start();
    await entered.promise;
    session.stop();
    rejectCapture("late capture failure");
    await Promise.resolve();
    expect(onError).not.toHaveBeenCalled();
  });

  it("starts the barge watcher only once across separate streamed speech pumps", async () => {
    const parked = deferred<void>();
    let captures = 0;
    const spoken: string[] = [];
    const watchBargeIn = vi.fn(async () => {
      throw new Error("not listening");
    });
    const io: VoiceIO = {
      async capture(ctx) {
        captures++;
        if (captures === 1) return blobOf();
        parked.resolve();
        await new Promise<void>((resolve) => ctx.signal.addEventListener("abort", () => resolve(), { once: true }));
        return null;
      },
      async transcribe() {
        return "two sentences";
      },
      async run(_intent, onDelta) {
        onDelta("One.");
        await Promise.resolve();
        await Promise.resolve();
        onDelta("Two.");
      },
      async speak(text) {
        spoken.push(text);
        return immediateUtterance();
      },
      watchBargeIn,
    };
    const session = new VoiceSession(io);
    session.start();
    await parked.promise;
    session.stop();
    expect(spoken).toEqual(["One.", "Two."]);
    expect(watchBargeIn).toHaveBeenCalledTimes(1);
  });

  it("suppresses converse errors when invoked outside a running session", async () => {
    const onError = vi.fn();
    const io: VoiceIO = {
      capture: vi.fn(),
      transcribe: vi.fn(),
      async run() {
        throw new Error("inactive");
      },
      speak: vi.fn(),
      watchBargeIn: vi.fn(),
    };
    const session = new VoiceSession(io, { onError });
    await (session as unknown as { converse(intent: string): Promise<void> }).converse("test");
    expect(onError).not.toHaveBeenCalled();
  });

  it("stringifies non-Error converse failures while running", async () => {
    const onError = vi.fn();
    const io: VoiceIO = {
      capture: vi.fn(),
      transcribe: vi.fn(),
      async run() {
        throw "raw run failure";
      },
      speak: vi.fn(),
      watchBargeIn: vi.fn(),
    };
    const session = new VoiceSession(io, { onError });
    (session as unknown as { running: boolean }).running = true;
    await (session as unknown as { converse(intent: string): Promise<void> }).converse("test");
    expect(onError).toHaveBeenCalledWith("raw run failure");
    session.stop();
  });
});
