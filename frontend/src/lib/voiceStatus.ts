import { getJSON } from "@/lib/api";
import { speechSupported } from "@/lib/speech";

interface ServerVoiceStatus {
  stt?: { configured?: boolean };
  tts?: { configured?: boolean };
}

export interface VoiceReadiness {
  serverSTT: boolean;
  serverTTS: boolean;
  browserInput: boolean;
  browserTTS: boolean;
  canListen: boolean;
  canSpeak: boolean;
}

export function browserVoiceCapabilities(): Pick<VoiceReadiness, "browserInput" | "browserTTS"> {
  const w = window as typeof window & { webkitAudioContext?: typeof AudioContext };
  const browserInput =
    !!navigator.mediaDevices?.getUserMedia &&
    typeof MediaRecorder !== "undefined" &&
    (typeof window.AudioContext !== "undefined" || typeof w.webkitAudioContext !== "undefined");
  return { browserInput, browserTTS: speechSupported() };
}

// Voice capture is browser-side, but transcription is not: every recorded
// utterance is sent to /api/transcribe. Browser SpeechRecognition is used only
// for optional wake-word spotting and must never be presented as an STT fallback.
export async function getVoiceReadiness(): Promise<VoiceReadiness> {
  const status = await getJSON<ServerVoiceStatus>("/api/voice/status");
  const browser = browserVoiceCapabilities();
  const serverSTT = !!status.stt?.configured;
  const serverTTS = !!status.tts?.configured;
  return {
    ...browser,
    serverSTT,
    serverTTS,
    canListen: serverSTT && browser.browserInput,
    canSpeak: serverTTS || browser.browserTTS,
  };
}
