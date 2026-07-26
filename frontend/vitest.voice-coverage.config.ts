import { defineConfig } from "vitest/config";
import path from "node:path";

const coveredSources = [
  "src/lib/voice.ts",
  "src/lib/voiceCatalog.ts",
  "src/lib/voiceSession.ts",
  "src/lib/voiceStatus.ts",
  "src/lib/tts.ts",
  "src/views/Jarvis.tsx",
  "src/views/Voice.tsx",
  "src/views/VoiceSetup.tsx",
];

// This is intentionally a focused product-surface ratchet. The general suite
// remains broad, while Jarvis/Voice cannot merge if any exercised behavior
// drops below complete statement, branch, function, or line coverage.
export default defineConfig({
  resolve: { alias: { "@": path.resolve(import.meta.dirname, "src") } },
  test: {
    environment: "node",
    include: [
      "src/lib/voice.test.ts",
      "src/lib/voiceCatalog.test.ts",
      "src/lib/voiceSession.test.ts",
      "src/lib/voiceSession.browser.test.ts",
      "src/lib/voiceStatus.test.ts",
      "src/lib/tts.test.ts",
      "src/views/Jarvis.test.tsx",
      "src/views/Voice.test.tsx",
      "src/views/VoiceSetup.test.tsx",
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
