// features/voice/types.ts — shared types for the voice feature.
// Day 6 carve-out: the voice feature owns the VoiceReadiness
// interface (browser + server capability check) and re-exports
// Utterance from the TTS subsystem. See docs/FRONTEND-REFACTOR-PLAN.md.

export type { Utterance } from "./lib/tts";
export type { VoiceReadiness } from "./lib/voiceStatus";
export type { VoiceState, CaptureCtx, VoiceIO } from "./lib/voiceSession";
