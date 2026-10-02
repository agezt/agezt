import { defineConfig } from "vitest/config";
import path from "node:path";

// Paths were remapped when the voice and Jarvis surfaces moved out of the flat
// src/lib + src/views layout into feature slices. This config was not moved
// with them, so every path below pointed at a file that no longer existed and
// `npm run test:coverage:voice` died with "No test files found, exiting with
// code 1" — the ratchet was enforcing nothing while reporting itself green on
// the job that ran it. If a surface is moved again, this file has to move with
// it; `npm run test:coverage:voice` is what notices.
const coveredSources = [
  "src/features/voice/lib/voice.ts",
  "src/features/voice/lib/voiceCatalog.ts",
  "src/features/voice/lib/voiceSession.ts",
  "src/features/voice/lib/voiceStatus.ts",
  "src/features/voice/lib/tts.ts",
  "src/features/jarvis/components/Jarvis.tsx",
  "src/features/voice/components/Voice.tsx",
  "src/features/voice/components/VoiceSetup.tsx",
];

// This is intentionally a focused product-surface ratchet. The general suite
// remains broad, while Jarvis/Voice cannot merge if any exercised behavior
// drops below complete statement, branch, function, or line coverage.
export default defineConfig({
  resolve: { alias: { "@": path.resolve(import.meta.dirname, "src") } },
  test: {
    environment: "node",
    include: [
      "src/features/voice/lib/voice.test.ts",
      "src/features/voice/lib/voiceCatalog.test.ts",
      "src/features/voice/lib/voiceSession.test.ts",
      "src/features/voice/lib/voiceSession.browser.test.ts",
      "src/features/voice/lib/voiceStatus.test.ts",
      "src/features/voice/lib/tts.test.ts",
      "src/features/jarvis/components/Jarvis.test.tsx",
      "src/features/voice/components/Voice.test.tsx",
      "src/features/voice/components/VoiceSetup.test.tsx",
    ],
    coverage: {
      provider: "v8",
      reporter: ["text"],
      include: coveredSources,
      thresholds: {
        statements: 100,
        branches: 100,
        functions: 100,
        lines: 100,
      },
    },
  },
});
