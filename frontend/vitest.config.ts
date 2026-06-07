import { defineConfig } from "vitest/config";
import path from "node:path";

// Vitest config kept separate from vite.config.ts so the unit tests don't pull
// in the React/Tailwind build plugins — they exercise pure logic (lib/*), so a
// plain node environment with the "@" alias is all that's needed.
export default defineConfig({
  resolve: { alias: { "@": path.resolve(import.meta.dirname, "src") } },
  test: {
    // Default node environment keeps the pure-logic tests fast; component tests
    // (*.test.tsx) opt into jsdom per-file via a `// @vitest-environment jsdom`
    // docblock.
    environment: "node",
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
