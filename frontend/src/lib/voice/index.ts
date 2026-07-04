// lib/voice — the browser voice subsystem, previously five sibling modules
// under lib/ (voice, tts, voiceCatalog, voiceSession, sentenceChunker). They
// are one cohesive concern (speech-to-text capture → LLM stream → text-to-
// speech playback, plus the provider/voice catalog and sentence chunker that
// feeds incremental TTS), so they live together here behind this barrel.
//
// The public import surface is unchanged for consumers: `@/lib/voice` now
// resolves to this barrel and re-exports everything the old modules did.
//   • MicButton         → transcribeAudio            (./transcribe)
//   • VoiceSetup         → the STT/TTS provider catalog (./catalog)
//   • Voice.tsx          → VoiceSession + createBrowserVoiceIO + VoiceState (./session)
//
// `Utterance` is defined in ./tts and re-exported by ./session; re-exporting it
// from both `export *` lines below is not a conflict because it is the same
// symbol (TypeScript dedupes identical re-exports).
//
// Note: lib/speech stays outside this folder — it is a lower-level browser
// SpeechSynthesis primitive used by both ./tts and views/Chat, not part of the
// voice-capture pipeline.
export * from "./transcribe";
export * from "./tts";
export * from "./catalog";
export * from "./sentenceChunker";
export * from "./session";
