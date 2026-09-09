// features/voice — Day 6 first feature carve-out.
// Owns:
//   - Browser voice I/O (MediaRecorder, AudioContext, Web Speech fallback)
//   - STT/TTS server status (getVoiceReadiness, transcribeAudio)
//   - The hands-free conversational loop (voiceSession state machine +
//     createBrowserVoiceIO for production)
//   - TTS helpers (speak, stopSpeech, Utterance) + sentence chunking
//   - Voice views (Voice + VoiceSetup)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/voice and never needs to know about the sub-paths):
//   - components/Voice.tsx, VoiceSetup.tsx — the two page-level views
//   - transcribeAudio() from lib/voice — the STT entry point used by
//     MicButton and other voice hooks
//   - types.ts — shared types (VoiceReadiness, Utterance, VoiceState, ...)
//
// The 6 lib files (voiceSession, voiceStatus, voiceCatalog, tts,
// speech, sentenceChunker) stay package-internal — they're the
// feature's business logic; only the entry points + views + types
// are surfaced.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Voice } from "./components/Voice";
export { VoiceSetup } from "./components/VoiceSetup";
export { transcribeAudio } from "./lib/voice";
export * from "./types";
