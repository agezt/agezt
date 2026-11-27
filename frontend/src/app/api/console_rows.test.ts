// The console's own rows reach the browser — and the browser's views take
// them from there.
//
// The console answers a handful of paths itself: the auth meta, the voice
// status, the SSE token. Each answer is a ROW the console's contract declares
// (kernel/contract/consoleapi), and the browser's catalogue is GENERATED from
// that declaration. So the browser names a PATH, and the catalogue says what
// the path answers with.
//
// Two teeth:
//
//  1. THE CATALOGUE CARRIES THE ROW. A console-native path that resolves to
//     `any` is the old drift waiting to happen: the browser reads a shape
//     nobody declared, and the two can drift apart without a word.
//
//  2. THE PATH RESOLVES TO ITS ROW. The compile-time assertions below do not
//     run; they COMPILE. If the catalogue stopped binding a path to its row,
//     `ResponseOf` would fall back to `any` and the typecheck gate would say
//     so — before a browser view could read a field the console never sends.
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import type {
  AuthmetaResponse,
  ResponseOf,
  SseTokenResponse,
  VoiceStatusResponse,
} from "@/app/api/types.generated";

/** Equal says two types are the same type. */
type Equal<X, Y> = (<T>() => T extends X ? 1 : 2) extends (<T>() => T extends Y ? 1 : 2)
  ? true
  : false;
/** Expect fails the typecheck when its argument is not true. */
type Expect<T extends true> = T;

const SRC = new URL("../../", import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, "$1");
const generated = readFileSync(SRC + "app/api/types.generated.ts", "utf8");
const client = readFileSync(SRC + "app/api/client.ts", "utf8");

/** The console-native paths the generated client knows. */
function nativePaths(): string[] {
  const out: string[] = [];
  for (const m of client.matchAll(/export type Native(?:Read|Write)Path =([\s\S]*?);/g)) {
    for (const q of m[1].matchAll(/"([^"]+)"/g)) out.push(q[1]);
  }
  return out;
}

/** The directory the console's own paths live in — derived, never written down. */
function pathPrefix(paths: string[]): string {
  const first = paths[0] ?? "";
  const cut = first.lastIndexOf("/");
  return cut > 0 ? first.slice(0, cut + 1) : "";
}

/** The path → row bindings the catalogue's native map declares. */
function nativeMap(): Map<string, string> {
  const out = new Map<string, string>();
  const m = /export interface NativeResponseMap \{([\s\S]*?)\n\}/.exec(generated);
  if (!m) return out;
  for (const line of m[1].split("\n")) {
    const row = /^\s*"([^"]+)":\s*([A-Za-z0-9_]+);/.exec(line);
    if (row) out.set(row[1], row[2]);
  }
  return out;
}

// ── the rows the browser's console-native views read ─────────────────────────
// The auth gate reads the auth meta, the Jarvis card reads the voice status,
// and the event feed reads the SSE token. All three come from the catalogue.
type _authMeta = Expect<Equal<ResponseOf<"/api/authmeta">, AuthmetaResponse>>;
type _voiceStatus = Expect<Equal<ResponseOf<"/api/voice/status">, VoiceStatusResponse>>;
type _sseToken = Expect<Equal<ResponseOf<"/api/sse-token">, SseTokenResponse>>;

describe("the console's own rows in the browser's catalogue", () => {
  it("binds every console-native path that names a row", () => {
    const map = nativeMap();
    const paths = nativePaths();
    const apiPrefix = pathPrefix(paths);
    expect(map.size).toBeGreaterThan(0);
    for (const [path, row] of map) {
      // the row is one the catalogue really declares
      expect(
        generated.includes(`export interface ${row} {`),
        `the native map binds ${path} to ${row}, which the catalogue does not declare`,
      ).toBe(true);
      // the path is one the console's generated client really serves — unless
      // it lives outside the console's own path prefix (the webhook), which the
      // browser never fetches
      if (path.startsWith(apiPrefix)) {
        expect(paths, `the native map binds ${path}, which the client does not serve`).toContain(path);
      }
    }
  });

  it("binds the paths the browser's console-native views read", () => {
    const map = nativeMap();
    expect(map.get("/api/authmeta")).toBe("AuthmetaResponse");
    expect(map.get("/api/voice/status")).toBe("VoiceStatusResponse");
    expect(map.get("/api/sse-token")).toBe("SseTokenResponse");
  });
});
