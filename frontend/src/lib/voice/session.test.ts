import { describe, it, expect, vi } from "vitest";
import { VoiceSession, type VoiceIO, type Utterance } from "./session";

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
});
