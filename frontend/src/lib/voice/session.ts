// voiceSession.ts is the conversational loop behind Voice Mode — the thing that
// makes AGEZT feel like talking to Jarvis. It drives one hands-free cycle after
// another: (optionally wait for the wake word) → listen until you stop talking →
// transcribe → run the agent → speak the answer sentence-by-sentence as it streams
// → and if you start talking over it, stop instantly and listen again (barge-in).
//
// The state machine and streaming-speech orchestration are kept free of any
// browser API: all audio I/O is injected through a VoiceIO, so the loop is fully
// unit-testable with fakes. createBrowserVoiceIO() supplies the real Web Audio /
// MediaRecorder / STT / TTS implementation for the view.

import { transcribeAudio } from "./transcribe";
import { streamRun } from "@/lib/chat";
import { speak as ttsSpeak, type Utterance } from "./tts";
import { createSpeechChunker } from "./sentenceChunker";

export type { Utterance } from "./tts";

export type VoiceState = "idle" | "waking" | "listening" | "thinking" | "speaking";

// CaptureCtx is handed to the I/O primitives so they can stream the mic level
// (for the orb) and bail out the moment the session stops.
export interface CaptureCtx {
  signal: AbortSignal;
  onLevel: (level0to1: number) => void;
}

// VoiceIO is the audio seam. Each method is independently mockable.
export interface VoiceIO {
  // capture opens the mic, waits for speech then a trailing silence, and resolves
  // with the recorded utterance — or null if it stopped before any speech.
  capture(ctx: CaptureCtx): Promise<Blob | null>;
  // transcribe turns recorded audio into text (speech-to-text).
  transcribe(audio: Blob): Promise<string>;
  // run executes the agent for `intent`, calling onDelta with streamed answer text.
  run(intent: string, onDelta: (text: string) => void, signal: AbortSignal): Promise<void>;
  // speak says one sentence aloud, returning a stoppable handle.
  speak(text: string): Promise<Utterance>;
  // watchBargeIn resolves the moment the user starts speaking again — used while
  // the assistant is talking so we can interrupt it.
  watchBargeIn(ctx: CaptureCtx): Promise<void>;
  // awaitWake (optional) resolves true when a wake keyword is heard. When absent,
  // the session gates on the transcript instead.
  awaitWake?(keywords: string[], ctx: CaptureCtx): Promise<boolean>;
  // release frees any held mic/audio resources.
  release?(): void;
}

export interface VoiceCallbacks {
  onState?(s: VoiceState): void;
  onLevel?(level0to1: number): void;
  onUserText?(text: string): void; // a finalized thing the user said
  onAnswerDelta?(text: string): void; // streamed assistant text
  onError?(message: string): void;
}

export interface VoiceOptions {
  // wakeWords, when non-empty, requires one of these to be heard/said before a
  // turn starts. Matched case-insensitively as a whole word.
  wakeWords?: string[];
  agent?: string; // run as this agent (optional)
  model?: string; // run with this model (optional)
}

// StreamingSpeaker plays a growing queue of sentences in order, starting as soon
// as the first one is ready (so speech overlaps the still-streaming answer) and
// stoppable instantly for barge-in.
class StreamingSpeaker {
  private q: string[] = [];
  private current: Utterance | null = null;
  private ended = false;
  private stopped = false;
  private pumping = false;
  private waiters: Array<() => void> = [];

  constructor(
    private speakFn: (t: string) => Promise<Utterance>,
    private onStart: () => void,
  ) {}

  enqueue(s: string) {
    if (this.stopped || !s.trim()) return;
    this.q.push(s);
    void this.pump();
  }

  end() {
    this.ended = true;
    this.settleIfDone();
  }

  stop() {
    this.stopped = true;
    this.q = [];
    this.current?.stop();
    this.current = null;
    this.settleIfDone();
  }

  private async pump() {
    if (this.pumping || this.stopped) return;
    this.pumping = true;
    if (this.q.length) this.onStart();
    while (this.q.length && !this.stopped) {
      const s = this.q.shift() as string;
      try {
        this.current = await this.speakFn(s);
        await this.current.done;
      } catch {
        /* a single utterance failing shouldn't kill the conversation */
      }
      this.current = null;
    }
    this.pumping = false;
    this.settleIfDone();
  }

  private settleIfDone() {
    if (!this.pumping && this.q.length === 0 && (this.ended || this.stopped)) {
      const w = this.waiters;
      this.waiters = [];
      w.forEach((f) => f());
    }
  }

  // idle resolves once the queue is drained (or stopped) and nothing is playing.
  idle(): Promise<void> {
    if (!this.pumping && this.q.length === 0 && (this.ended || this.stopped)) return Promise.resolve();
    return new Promise((r) => this.waiters.push(r));
  }
}

function matchesWake(text: string, keywords: string[]): boolean {
  const lc = text.toLowerCase();
  return keywords.some((k) => new RegExp(`\\b${k.toLowerCase().replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\b`).test(lc));
}

// stripWake removes a leading wake word ("agezt, turn off the lights" → "turn off
// the lights") so the agent doesn't see its own name as part of the command.
function stripWake(text: string, keywords: string[]): string {
  let out = text.trim();
  for (const k of keywords) {
    const re = new RegExp(`^${k.toLowerCase().replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}[\\s,.:!?-]*`, "i");
    out = out.replace(re, "");
  }
  return out.trim();
}

export class VoiceSession {
  private running = false;
  private state: VoiceState = "idle";
  private loopAbort: AbortController | null = null;

  constructor(
    private io: VoiceIO,
    private cb: VoiceCallbacks = {},
    private opts: VoiceOptions = {},
  ) {}

  getState(): VoiceState {
    return this.state;
  }

  isRunning(): boolean {
    return this.running;
  }

  private setState(s: VoiceState) {
    if (this.state === s) return;
    this.state = s;
    this.cb.onState?.(s);
  }

  start() {
    if (this.running) return;
    this.running = true;
    void this.loop();
  }

  stop() {
    this.running = false;
    this.loopAbort?.abort();
    this.io.release?.();
    this.setState("idle");
  }

  private ctx(signal: AbortSignal): CaptureCtx {
    return { signal, onLevel: (l) => this.cb.onLevel?.(l) };
  }

  private async loop() {
    const wake = (this.opts.wakeWords ?? []).filter(Boolean);
    while (this.running) {
      this.loopAbort = new AbortController();
      const signal = this.loopAbort.signal;
      try {
        // 1. Wake gate (cheap continuous listen) when the I/O supports it.
        if (wake.length && this.io.awaitWake) {
          this.setState("waking");
          const woke = await this.io.awaitWake(wake, this.ctx(signal));
          if (!this.running) break;
          if (!woke) continue;
        }
        // 2. Listen for one utterance.
        this.setState("listening");
        const audio = await this.io.capture(this.ctx(signal));
        if (!this.running) break;
        if (!audio) continue;
        // 3. Transcribe.
        this.setState("thinking");
        let userText = (await this.io.transcribe(audio)).trim();
        if (!this.running) break;
        if (!userText) continue;
        // 4. Wake gate on the transcript when there's no dedicated wake listener.
        if (wake.length && !this.io.awaitWake) {
          if (!matchesWake(userText, wake)) continue;
          userText = stripWake(userText, wake);
          if (!userText) continue;
        } else if (wake.length) {
          userText = stripWake(userText, wake);
        }
        this.cb.onUserText?.(userText);
        // 5. Run + speak (overlapped) with barge-in.
        await this.converse(userText);
      } catch (e) {
        if (this.running) this.cb.onError?.((e as Error).message || String(e));
      }
    }
    this.setState("idle");
  }

  private async converse(intent: string) {
    const chunker = createSpeechChunker(0);
    const runAbort = new AbortController();
    const bargeAbort = new AbortController();
    let bargeStarted = false;
    let interrupted = false;

    const speaker = new StreamingSpeaker(
      (t) => this.io.speak(t),
      () => {
        this.setState("speaking");
        if (!bargeStarted) {
          bargeStarted = true;
          void this.io
            .watchBargeIn(this.ctx(bargeAbort.signal))
            .then(() => {
              interrupted = true;
              speaker.stop();
              runAbort.abort(); // stop the run too — the user wants to redirect
            })
            .catch(() => {});
        }
      },
    );

    try {
      await this.io.run(
        intent,
        (delta) => {
          this.cb.onAnswerDelta?.(delta);
          for (const s of chunker.push(delta)) speaker.enqueue(s);
        },
        runAbort.signal,
      );
    } catch (e) {
      if (!interrupted && this.running) this.cb.onError?.((e as Error).message || String(e));
    }
    const rest = chunker.flush();
    if (rest) speaker.enqueue(rest);
    speaker.end();
    await speaker.idle();
    bargeAbort.abort();
  }
}

// defaultRun adapts streamRun to the VoiceIO.run shape: it forwards each streamed
// answer-text delta (llm.token frames) to onDelta and ignores tool/meta frames.
function defaultRun(opts: { agent?: string; model?: string }) {
  return (intent: string, onDelta: (text: string) => void, signal: AbortSignal) =>
    streamRun(
      { intent, agent: opts.agent, model: opts.model },
      (f) => {
        if (f.kind === "llm.token" && f.payload && f.payload.text != null) onDelta(String(f.payload.text));
      },
      signal,
    );
}

// --- Browser audio I/O --------------------------------------------------------

interface BrowserIOConfig {
  agent?: string;
  model?: string;
  // VAD tuning (sensible defaults; exposed for the view if ever needed).
  speechThreshold?: number; // RMS above which we consider it speech (0..1)
  silenceMs?: number; // trailing silence that ends an utterance
  minSpeechMs?: number; // ignore blips shorter than this
  maxUtteranceMs?: number; // hard cap on one utterance
}

// SpeechRecognition isn't in TS's lib DOM by default; narrow it locally.
interface SpeechRecognitionLike {
  continuous: boolean;
  interimResults: boolean;
  lang: string;
  onresult: ((e: { results: ArrayLike<ArrayLike<{ transcript: string }>> }) => void) | null;
  onerror: (() => void) | null;
  onend: (() => void) | null;
  start(): void;
  stop(): void;
  abort(): void;
}

// createBrowserVoiceIO builds the real audio loop: a single persistent mic stream
// feeds an AnalyserNode for VAD/level, a MediaRecorder captures utterances, STT is
// the server /api/transcribe (browser-independent quality), TTS prefers the server
// voice, and the wake word uses the browser SpeechRecognition when available.
export function createBrowserVoiceIO(cfg: BrowserIOConfig = {}): VoiceIO {
  const speechThreshold = cfg.speechThreshold ?? 0.035;
  const silenceMs = cfg.silenceMs ?? 900;
  const minSpeechMs = cfg.minSpeechMs ?? 200;
  const maxUtteranceMs = cfg.maxUtteranceMs ?? 15000;

  let stream: MediaStream | null = null;
  let audioCtx: AudioContext | null = null;
  let analyser: AnalyserNode | null = null;
  let buf: Uint8Array<ArrayBuffer> | null = null;

  async function ensureMic() {
    if (stream && audioCtx && analyser) return;
    if (!navigator.mediaDevices?.getUserMedia) throw new Error("microphone not available in this browser");
    stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    const Ctx = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
    audioCtx = new Ctx();
    const src = audioCtx.createMediaStreamSource(stream);
    analyser = audioCtx.createAnalyser();
    analyser.fftSize = 1024;
    src.connect(analyser);
    buf = new Uint8Array(analyser.fftSize);
  }

  function level(): number {
    if (!analyser || !buf) return 0;
    analyser.getByteTimeDomainData(buf);
    let sum = 0;
    for (let i = 0; i < buf.length; i++) {
      const v = (buf[i] - 128) / 128;
      sum += v * v;
    }
    return Math.sqrt(sum / buf.length);
  }

  // poll runs cb every ~50ms until it returns true or the signal aborts.
  function poll(signal: AbortSignal, cb: (lvl: number, elapsedMs: number) => boolean): Promise<boolean> {
    return new Promise((resolve) => {
      const t0 = performance.now();
      const id = setInterval(() => {
        if (signal.aborted) {
          clearInterval(id);
          resolve(false);
          return;
        }
        if (cb(level(), performance.now() - t0)) {
          clearInterval(id);
          resolve(true);
        }
      }, 50);
    });
  }

  async function capture(ctx: CaptureCtx): Promise<Blob | null> {
    await ensureMic();
    if (!stream) return null;
    const rec = new MediaRecorder(stream);
    const chunks: Blob[] = [];
    rec.ondataavailable = (e) => {
      if (e.data && e.data.size > 0) chunks.push(e.data);
    };
    rec.start();

    let speechStartedAt = -1;
    let lastLoud = -1;
    const ok = await poll(ctx.signal, (lvl, elapsed) => {
      ctx.onLevel(lvl);
      const loud = lvl >= speechThreshold;
      if (loud) {
        if (speechStartedAt < 0) speechStartedAt = elapsed;
        lastLoud = elapsed;
      }
      if (elapsed >= maxUtteranceMs) return true;
      // End once we've had real speech followed by enough trailing silence.
      if (speechStartedAt >= 0 && elapsed - speechStartedAt >= minSpeechMs && lastLoud >= 0 && elapsed - lastLoud >= silenceMs) {
        return true;
      }
      return false;
    });

    const blob: Blob = await new Promise((resolve) => {
      rec.onstop = () => resolve(new Blob(chunks, { type: rec.mimeType || "audio/webm" }));
      try {
        rec.stop();
      } catch {
        resolve(new Blob(chunks, { type: "audio/webm" }));
      }
    });
    if (!ok || speechStartedAt < 0 || blob.size === 0) return null; // aborted or no speech
    return blob;
  }

  async function watchBargeIn(ctx: CaptureCtx): Promise<void> {
    await ensureMic();
    let loudFor = 0;
    let prev = performance.now();
    await poll(ctx.signal, (lvl) => {
      ctx.onLevel(lvl);
      const now = performance.now();
      const dt = now - prev;
      prev = now;
      loudFor = lvl >= speechThreshold * 1.4 ? loudFor + dt : 0;
      return loudFor >= 160; // sustained speech → barge-in
    });
  }

  function makeRecognition(): SpeechRecognitionLike | null {
    const w = window as unknown as {
      SpeechRecognition?: new () => SpeechRecognitionLike;
      webkitSpeechRecognition?: new () => SpeechRecognitionLike;
    };
    const Ctor = w.SpeechRecognition || w.webkitSpeechRecognition;
    return Ctor ? new Ctor() : null;
  }

  const io: VoiceIO = {
    capture,
    watchBargeIn,
    transcribe: (audio) => transcribeAudio(audio, "utterance.webm"),
    run: defaultRun({ agent: cfg.agent, model: cfg.model }),
    speak: (t) => ttsSpeak(t),
    release() {
      try {
        stream?.getTracks().forEach((t) => t.stop());
        void audioCtx?.close();
      } catch {
        /* best effort */
      }
      stream = null;
      audioCtx = null;
      analyser = null;
      buf = null;
    },
  };

  // Only advertise awaitWake when the browser can actually do cheap keyword
  // spotting — otherwise the session falls back to transcript gating.
  if (makeRecognition()) {
    io.awaitWake = (keywords, ctx) =>
      new Promise<boolean>((resolve) => {
        const rec = makeRecognition();
        if (!rec) {
          resolve(true);
          return;
        }
        let done = false;
        const finish = (v: boolean) => {
          if (done) return;
          done = true;
          try {
            rec.abort();
          } catch {
            /* ignore */
          }
          resolve(v);
        };
        rec.continuous = true;
        rec.interimResults = true;
        rec.lang = navigator.language || "en-US";
        rec.onresult = (e) => {
          for (let i = 0; i < e.results.length; i++) {
            const t = e.results[i][0]?.transcript ?? "";
            if (matchesWake(t, keywords)) finish(true);
          }
        };
        rec.onerror = () => finish(false);
        rec.onend = () => finish(false);
        ctx.signal.addEventListener("abort", () => finish(false), { once: true });
        try {
          rec.start();
        } catch {
          finish(false);
        }
      });
  }

  return io;
}
