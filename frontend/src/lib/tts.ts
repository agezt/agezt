// tts.ts speaks text aloud for Voice Mode, preferring the daemon's configured
// text-to-speech backend (Kokoro / OpenAI via /api/tts — natural, high-quality)
// and falling back to the browser's built-in SpeechSynthesis when TTS isn't
// configured or the request fails. Every utterance is a handle the caller can
// stop instantly — that's what makes barge-in possible (interrupt the spoken
// reply the moment the user starts talking).

import { authHeaders } from "@/lib/api";
import { speak as browserSpeak, stopSpeaking as browserStop, speechSupported } from "@/lib/speech";

// Once /api/tts answers 501 (TTS not configured) we stop hitting it for the rest
// of the session and go straight to the browser voice — no point re-asking.
let serverTTSDisabled = false;

// resetServerTTS re-enables server probing (for tests).
export function resetServerTTS(): void {
  serverTTSDisabled = false;
}

// fetchSpeech asks the server to synthesize `text` and returns the audio Blob, or
// null when server TTS is unavailable (not configured / unsupported) so the caller
// falls back to the browser. Throws only on an unexpected transport error after
// the server was thought available.
export async function fetchSpeech(text: string, signal?: AbortSignal): Promise<Blob | null> {
  if (serverTTSDisabled) return null;
  const t = text.trim();
  if (!t) return null;
  let res: Response;
  try {
    res = await fetch("/api/tts", {
      method: "POST",
      headers: authHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ text: t }),
      signal,
    });
  } catch {
    return null; // network/abort — fall back to the browser voice
  }
  if (res.status === 501) {
    serverTTSDisabled = true; // not configured; don't ask again this session
    return null;
  }
  if (!res.ok) return null; // 4xx/5xx — degrade rather than go silent
  const blob = await res.blob();
  return blob.size > 0 ? blob : null;
}

// Utterance is an in-flight spoken phrase. `done` resolves when playback finishes
// (or is stopped); `stop` halts it immediately and resolves `done`.
export interface Utterance {
  stop(): void;
  done: Promise<void>;
}

// playBlob plays synthesized audio through an <audio> element and revokes the
// object URL when done. Exposed for the server-TTS path.
export function playBlob(blob: Blob): Utterance {
  const url = URL.createObjectURL(blob);
  const audio = new Audio(url);
  let settle: () => void = () => {};
  const done = new Promise<void>((resolve) => (settle = resolve));
  let finished = false;
  const finish = () => {
    if (finished) return;
    finished = true;
    URL.revokeObjectURL(url);
    settle();
  };
  audio.onended = finish;
  audio.onerror = finish;
  audio.play().catch(finish); // autoplay blocked / decode error → don't hang
  return {
    stop() {
      try {
        audio.pause();
      } catch {
        /* already gone */
      }
      finish();
    },
    done,
  };
}

// browserUtterance speaks via the browser SpeechSynthesis voice.
function browserUtterance(text: string): Utterance {
  let settle: () => void = () => {};
  const done = new Promise<void>((resolve) => (settle = resolve));
  if (!speechSupported()) {
    settle();
    return { stop: () => {}, done };
  }
  browserSpeak(text, () => settle());
  return {
    stop() {
      browserStop();
      settle();
    },
    done,
  };
}

// speak says `text` aloud, returning a stoppable handle. Prefers server TTS and
// transparently falls back to the browser voice. The signal aborts the synthesis
// request (not playback — use the handle's stop for that).
export async function speak(text: string, signal?: AbortSignal): Promise<Utterance> {
  const blob = await fetchSpeech(text, signal);
  if (blob) return playBlob(blob);
  return browserUtterance(text);
}
