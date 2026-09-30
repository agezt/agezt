// features/voice — Day 6 first feature carve-out.
// Owns:
//   - Browser voice I/O (MediaRecorder, AudioContext, Web Speech fallback)
//   - STT/TTS server status (getVoiceReadiness, transcribeAudio)
//   - The hands-free conversational loop (voiceSession state machine +
//     createBrowserVoiceIO for production)
//   - TTS helpers (speak, stopSpeech, Utterance) + sentence chunking
//   - Voice views (Voice + VoiceSetup)
//
// Public surface: `transcribeAudio()` from lib/voice — the STT entry point used
// by MicButton and the voice hooks. The two page-level views are imported from
// components/ directly, and the shared types (Utterance, VoiceReadiness,
// VoiceState, CaptureCtx, VoiceIO) are imported from their owning lib modules.
//
// This barrel used to re-export the views and to `export * from "./types"`.
// Nothing consumed either: every real caller reached past it into
// components/ and lib/, which is the same reasoning the other Day 6 feature
// barrels settled on. A types.ts that only the barrel pointed at is a public
// surface with no public.
//
// The 6 lib files (voiceSession, voiceStatus, voiceCatalog, tts,
// speech, sentenceChunker) stay package-internal — they're the
// feature's business logic; only the entry point is surfaced here.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { transcribeAudio } from "./lib/voice";
