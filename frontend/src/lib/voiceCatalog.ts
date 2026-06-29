// voiceCatalog is the curated, human-friendly map of speech providers AGEZT can
// use. The daemon's voice adapter speaks the OpenAI-compatible wire shape
// (POST <base>/v1/audio/transcriptions for hearing, POST <base>/v1/audio/speech
// for talking), so every entry here is a drop-in for that shape — hosted or
// self-hosted. Picking a provider + model in the UI just writes the matching
// AGEZT_STT_* / AGEZT_TTS_* config; nothing here needs a backend change.
//
// Deliberately excluded because they are NOT native-OpenAI-compatible for audio
// (they'd need a LiteLLM-style proxy): Deepgram, ElevenLabs, DeepInfra. A
// "Custom endpoint" escape hatch in the UI covers anything not listed.
interface SpeechModel {
  id: string;
  label?: string; // friendly name shown in the dropdown (falls back to id)
  note?: string; // tiny hint, e.g. "fastest", "best quality"
}

export interface SpeechVoice {
  id: string;
  label?: string;
}

type SpeechDialect = "openai" | "elevenlabs" | "deepgram" | "cartesia";

export interface SpeechProvider {
  id: string;
  name: string; // friendly display name
  blurb: string; // one warm sentence about what it is / when to pick it
  baseURL: string; // includes /v1 for OpenAI-compatible; native bases have none
  needsKey: boolean;
  local?: boolean; // runs on the operator's own machine — private, free
  /** Wire dialect — maps to AGEZT_*_PROVIDER. Omitted == "openai". */
  dialect?: SpeechDialect;
  models: SpeechModel[];
  /** TTS only: default voice list. */
  voices?: SpeechVoice[];
  /** TTS only: voices that depend on the chosen model (overrides `voices`). */
  voicesByModel?: Record<string, SpeechVoice[]>;
  /** TTS only: voice is a free-text id (ElevenLabs voice_id, Cartesia id). */
  voiceFree?: boolean;
  /** TTS only: the model name carries the voice (Deepgram Aura) — no voice field. */
  voiceInModel?: boolean;
  voiceHint?: string; // placeholder for a free-text voice field
  keyHint?: string; // placeholder for the API-key input
  keyLink?: string; // where to get a key
}

const v = (ids: string[]): SpeechVoice[] => ids.map((id) => ({ id }));

// --- Hearing · Speech-to-text -------------------------------------------------

export const STT_PROVIDERS: SpeechProvider[] = [
  {
    id: "openai",
    name: "OpenAI",
    blurb: "Whisper & GPT-4o transcription — the most accurate, hosted.",
    baseURL: "https://api.openai.com/v1",
    needsKey: true,
    keyHint: "sk-…",
    keyLink: "https://platform.openai.com/api-keys",
    models: [
      { id: "gpt-4o-transcribe", label: "GPT-4o Transcribe", note: "best accuracy" },
      { id: "gpt-4o-mini-transcribe", label: "GPT-4o mini Transcribe", note: "fast & cheap" },
      { id: "whisper-1", label: "Whisper v2", note: "classic, supports subtitles" },
    ],
  },
  {
    id: "groq",
    name: "Groq",
    blurb: "Whisper v3 on Groq's chips — blazing fast, generous free tier.",
    baseURL: "https://api.groq.com/openai/v1",
    needsKey: true,
    keyHint: "gsk_…",
    keyLink: "https://console.groq.com/keys",
    models: [
      { id: "whisper-large-v3-turbo", label: "Whisper Large v3 Turbo", note: "fastest" },
      { id: "whisper-large-v3", label: "Whisper Large v3", note: "most accurate" },
    ],
  },
  {
    id: "elevenlabs",
    name: "ElevenLabs",
    blurb: "Scribe — top-tier accuracy across 99 languages.",
    baseURL: "https://api.elevenlabs.io",
    needsKey: true,
    dialect: "elevenlabs",
    keyHint: "xi-api-key",
    keyLink: "https://elevenlabs.io/app/settings/api-keys",
    models: [
      { id: "scribe_v2", label: "Scribe v2", note: "best" },
      { id: "scribe_v1", label: "Scribe v1", note: "legacy" },
    ],
  },
  {
    id: "deepgram",
    name: "Deepgram",
    blurb: "Nova-3 — fast, accurate, streaming-grade.",
    baseURL: "https://api.deepgram.com",
    needsKey: true,
    dialect: "deepgram",
    keyHint: "Deepgram API key",
    keyLink: "https://console.deepgram.com/",
    models: [
      { id: "nova-3", label: "Nova-3", note: "best" },
      { id: "nova-2", label: "Nova-2" },
      { id: "whisper-large", label: "Whisper (hosted)" },
    ],
  },
  {
    id: "lemonfox",
    name: "Lemonfox",
    blurb: "Hosted Whisper, 100+ languages, very low cost.",
    baseURL: "https://api.lemonfox.ai/v1",
    needsKey: true,
    keyHint: "your Lemonfox key",
    keyLink: "https://www.lemonfox.ai/",
    models: [{ id: "whisper-1", label: "Whisper", note: "100+ languages" }],
  },
  {
    id: "speaches",
    name: "Local · Speaches",
    blurb: "Self-hosted faster-whisper. Private, free, no key — runs on your machine.",
    baseURL: "http://localhost:8000/v1",
    needsKey: false,
    local: true,
    models: [
      { id: "Systran/faster-whisper-large-v3", label: "faster-whisper large v3", note: "most accurate" },
      { id: "deepdml/faster-whisper-large-v3-turbo-ct2", label: "large v3 turbo", note: "fast" },
      { id: "Systran/faster-whisper-small", label: "faster-whisper small", note: "lightest" },
    ],
  },
  {
    id: "localai-stt",
    name: "Local · LocalAI",
    blurb: "Self-hosted LocalAI audio stack. No key.",
    baseURL: "http://localhost:8080/v1",
    needsKey: false,
    local: true,
    models: [{ id: "whisper-1", label: "Whisper" }],
  },
];

// --- Voice · Text-to-speech ---------------------------------------------------

export const TTS_PROVIDERS: SpeechProvider[] = [
  {
    id: "openai",
    name: "OpenAI",
    blurb: "Natural, expressive voices — the easiest way to sound good.",
    baseURL: "https://api.openai.com/v1",
    needsKey: true,
    keyHint: "sk-…",
    keyLink: "https://platform.openai.com/api-keys",
    models: [
      { id: "gpt-4o-mini-tts", label: "GPT-4o mini TTS", note: "best, steerable" },
      { id: "tts-1", label: "TTS-1", note: "fast" },
      { id: "tts-1-hd", label: "TTS-1 HD", note: "higher fidelity" },
    ],
    voices: v(["alloy", "ash", "ballad", "coral", "echo", "fable", "nova", "onyx", "sage", "shimmer", "verse", "marin", "cedar"]),
    voicesByModel: {
      "tts-1": v(["alloy", "ash", "coral", "echo", "fable", "onyx", "nova", "sage", "shimmer"]),
      "tts-1-hd": v(["alloy", "ash", "coral", "echo", "fable", "onyx", "nova", "sage", "shimmer"]),
    },
  },
  {
    id: "groq",
    name: "Groq",
    blurb: "Orpheus voices on Groq — fast and lifelike.",
    baseURL: "https://api.groq.com/openai/v1",
    needsKey: true,
    keyHint: "gsk_…",
    keyLink: "https://console.groq.com/keys",
    models: [
      { id: "canopylabs/orpheus-v1-english", label: "Orpheus (English)" },
      { id: "canopylabs/orpheus-arabic-saudi", label: "Orpheus (Arabic)" },
    ],
    voicesByModel: {
      "canopylabs/orpheus-v1-english": v(["autumn", "diana", "hannah", "austin", "daniel", "troy"]),
      "canopylabs/orpheus-arabic-saudi": v(["abdullah", "fahad", "sultan", "lulwa", "noura", "aisha"]),
    },
  },
  {
    id: "elevenlabs",
    name: "ElevenLabs",
    blurb: "The most lifelike voices — paste any voice_id from your library.",
    baseURL: "https://api.elevenlabs.io",
    needsKey: true,
    dialect: "elevenlabs",
    voiceFree: true,
    voiceHint: "voice_id from your ElevenLabs library",
    keyHint: "xi-api-key",
    keyLink: "https://elevenlabs.io/app/voice-library",
    models: [
      { id: "eleven_v3", label: "Eleven v3", note: "most expressive" },
      { id: "eleven_multilingual_v2", label: "Multilingual v2", note: "default" },
      { id: "eleven_turbo_v2_5", label: "Turbo v2.5", note: "low latency" },
      { id: "eleven_flash_v2_5", label: "Flash v2.5", note: "fastest" },
    ],
  },
  {
    id: "deepgram",
    name: "Deepgram",
    blurb: "Aura-2 — natural, real-time voices. Pick a voice as the model.",
    baseURL: "https://api.deepgram.com",
    needsKey: true,
    dialect: "deepgram",
    voiceInModel: true,
    keyHint: "Deepgram API key",
    keyLink: "https://console.deepgram.com/",
    models: [
      { id: "aura-2-thalia-en", label: "Thalia", note: "F · US" },
      { id: "aura-2-andromeda-en", label: "Andromeda", note: "F · US" },
      { id: "aura-2-helena-en", label: "Helena", note: "F · US" },
      { id: "aura-2-apollo-en", label: "Apollo", note: "M · US" },
      { id: "aura-2-arcas-en", label: "Arcas", note: "M · US" },
      { id: "aura-2-aries-en", label: "Aries", note: "M · US" },
    ],
  },
  {
    id: "cartesia",
    name: "Cartesia",
    blurb: "Sonic — ultra-low latency, expressive. Paste a Cartesia voice id.",
    baseURL: "https://api.cartesia.ai",
    needsKey: true,
    dialect: "cartesia",
    voiceFree: true,
    voiceHint: "Cartesia voice id",
    keyHint: "Cartesia API key",
    keyLink: "https://play.cartesia.ai/keys",
    models: [
      { id: "sonic-3.5", label: "Sonic 3.5", note: "newest" },
      { id: "sonic-3", label: "Sonic 3" },
      { id: "sonic-2", label: "Sonic 2" },
    ],
  },
  {
    id: "kokoro",
    name: "Local · Kokoro",
    blurb: "Self-hosted Kokoro — surprisingly good, private, free, no key.",
    baseURL: "http://localhost:8880/v1",
    needsKey: false,
    local: true,
    models: [{ id: "kokoro", label: "Kokoro 82M" }],
    voices: v([
      "af_heart", "af_bella", "af_sky", "af_nicole", "af_sarah", "af_aoede",
      "am_adam", "am_michael", "am_onyx", "am_fenrir", "am_puck",
      "bf_emma", "bf_isabella", "bm_george", "bm_lewis", "bm_fable",
    ]),
  },
  {
    id: "openedai",
    name: "Local · openedai-speech",
    blurb: "Self-hosted Piper / XTTS behind an OpenAI shim. No key.",
    baseURL: "http://localhost:8000/v1",
    needsKey: false,
    local: true,
    models: [
      { id: "tts-1", label: "TTS-1", note: "Piper, CPU" },
      { id: "tts-1-hd", label: "TTS-1 HD", note: "XTTS, GPU" },
    ],
    voices: v(["alloy", "echo", "fable", "onyx", "nova", "shimmer"]),
  },
  {
    id: "localai-tts",
    name: "Local · LocalAI",
    blurb: "Self-hosted LocalAI speech. Voices map to your installed models.",
    baseURL: "http://localhost:8080/v1",
    needsKey: false,
    local: true,
    models: [{ id: "tts-1", label: "TTS-1" }],
    voices: [],
  },
];

// normBase makes two API roots comparable regardless of a trailing slash or an
// optional /v1 suffix, so a stored AGEZT_*_URL matches its catalog provider.
function normBase(url?: string): string {
  let s = (url || "").trim().toLowerCase().replace(/\/+$/, "");
  s = s.replace(/\/v1$/, "");
  return s;
}

// providerFor finds the catalog entry whose base URL matches a stored value.
function providerFor(list: SpeechProvider[], url?: string): SpeechProvider | undefined {
  if (!url || !url.trim()) return undefined;
  const target = normBase(url);
  return list.find((p) => normBase(p.baseURL) === target);
}

// selectProvider resolves the active catalog entry from stored config: a native
// dialect (elevenlabs/deepgram/cartesia) identifies its single entry directly;
// otherwise (OpenAI-compatible) we match by base URL, since many share dialect.
export function selectProvider(list: SpeechProvider[], url?: string, dialect?: string): SpeechProvider | undefined {
  const d = (dialect || "").trim().toLowerCase();
  if (d && d !== "openai") return list.find((p) => (p.dialect || "openai") === d);
  return providerFor(list, url);
}

// dialectOf returns the AGEZT_*_PROVIDER value to persist for a provider.
export function dialectOf(p: SpeechProvider): string {
  return p.dialect || "openai";
}

// voicesFor returns the voice list appropriate to the chosen model.
export function voicesFor(p: SpeechProvider | undefined, model?: string): SpeechVoice[] {
  if (!p) return [];
  if (model && p.voicesByModel?.[model]) return p.voicesByModel[model];
  return p.voices ?? [];
}
