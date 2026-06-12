/**
 * Official TypeScript/JavaScript client for the Agezt agentic OS.
 *
 * Talks to a running Agezt daemon's native REST API (`/api/v1`) — the same
 * governed kernel loop `agt run` uses — over `fetch` with a bearer token.
 * Zero runtime dependencies (uses the platform `fetch`); works in Node 18+ and
 * modern browsers.
 *
 * ```ts
 * import { Client } from "@agezt/sdk";
 * const c = new Client("http://127.0.0.1:8800", "<token>");
 * const r = await c.run("summarise the latest commits");
 * console.log(r.answer);
 * ```
 */
export { Client } from "./client.js";
export type {
  ClientOptions,
  Health,
  Mail,
  MailDraft,
  Models,
  RunResult,
  RunArc,
  StreamEvent,
} from "./client.js";
export { AgeztError, APIError } from "./errors.js";
