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
export type { ClientOptions, StreamEvent } from "./client.js";
export {
  MAIL_ACK_NAMES,
  MAIL_DRAFT_NAMES,
  PATHS,
  RUN_REQUEST_NAMES,
  mailAckToWire,
  mailDraftToWire,
  runRequestToWire,
} from "./generated.js";
export type {
  ApiError,
  Artifacts,
  Health,
  Mail,
  MailAck,
  MailAcked,
  MailDraft,
  MailboxInbox,
  MailboxMessages,
  MailboxReplies,
  MailboxSent,
  MailboxTopics,
  Models,
  ProbeStatus,
  RunArc,
  RunRequest,
  RunResult,
} from "./generated.js";
export { AgeztError, APIError, ConfigAccessError } from "./errors.js";

// Agent SDK - for AI agent subprocess communication
export { AgentClient, Capability } from "./agent.js";
export type {
  AgentClientOptions,
  MemoryRecord,
  SearchResult,
  AgentProfile,
  BusEvent,
} from "./agent.js";
export { AgentError } from "./agent.js";
