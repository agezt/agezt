import { PATHS, mailAckToWire, mailDraftToWire, runRequestToWire } from "./generated.js";
import type {
  Health,
  Mail,
  MailAck,
  MailDraft,
  Models,
  RunArc,
  RunRequest,
  RunResult,
} from "./generated.js";
import { APIError } from "./errors.js";

/** One Server-Sent Event from a streaming run. `event` is one of
 * `start` | `token` | `done` | `error`; `data` is the decoded JSON payload. */
export interface StreamEvent {
  event: string;
  data: Record<string, unknown>;
}

export interface ClientOptions {
  /** Per-request timeout in milliseconds (default 30000). */
  timeoutMs?: number;
  /** Optional tenant id, sent as the `X-Agezt-Tenant` header. */
  tenant?: string;
}

/**
 * A client for a running Agezt daemon's REST API.
 *
 * ```ts
 * const c = new Client("http://127.0.0.1:8800", "<token>");
 * console.log((await c.health()).version);
 * const r = await c.run("summarise the latest commits");
 * for await (const ev of c.runStream("write a haiku")) {
 *   if (ev.event === "token") process.stdout.write(String(ev.data.text ?? ""));
 * }
 * ```
 */
export class Client {
  private readonly baseUrl: string;
  private readonly token: string;
  private readonly timeoutMs: number;
  private readonly tenant?: string;

  constructor(baseUrl: string, token: string, opts: ClientOptions = {}) {
    this.baseUrl = baseUrl.replace(/\/+$/, "");
    this.token = token;
    this.timeoutMs = opts.timeoutMs ?? 30000;
    this.tenant = opts.tenant;
  }

  health(): Promise<Health> {
    return this.getJSON<Health>(PATHS.health);
  }

  models(): Promise<Models> {
    return this.getJSON<Models>(PATHS.models);
  }

  /** Execute an intent and resolve with the final answer (blocking). Throws
   * APIError if the run fails or the request is rejected. */
  async run(intent: string, model?: string): Promise<RunResult> {
    const row: RunRequest = { intent };
    if (model) row.model = model;
    const res = await this.fetch("POST", PATHS.run, JSON.stringify(runRequestToWire(row)));
    if (!res.ok) throw await apiError(res);
    return (await res.json()) as RunResult;
  }

  /** Execute an intent, yielding StreamEvents (start/token/done/error) as the
   * agent produces them. */
  async *runStream(intent: string, model?: string): AsyncGenerator<StreamEvent> {
    const row: RunRequest = { intent, stream: true };
    if (model) row.model = model;
    const res = await this.fetch(
      "POST",
      PATHS.run,
      JSON.stringify(runRequestToWire(row)),
      "text/event-stream",
    );
    if (!res.ok) throw await apiError(res);
    if (!res.body) return;
    yield* parseSSE(res.body);
  }

  getRun(correlationId: string): Promise<RunArc> {
    return this.getJSON<RunArc>(PATHS.runArc.replace("{id}", encodeURIComponent(correlationId)));
  }

  // --- mailbox (the shared inter-agent message board, M937) ---

  /** Leave a message on the shared mailbox. See MailDraft for addressing. */
  async mailboxSend(draft: MailDraft): Promise<Mail> {
    const res = await this.fetch(
      "POST",
      PATHS.mailboxMessages,
      JSON.stringify(mailDraftToWire(draft)),
    );
    if (!res.ok) throw await apiError(res);
    const out = (await res.json()) as { message: Mail };
    return out.message;
  }

  /** Send an announcement to EVERY inbox except the sender's. */
  mailboxBroadcast(from: string, text: string): Promise<Mail> {
    return this.mailboxSend({ from, to: "*", text });
  }

  /** What waits for `name`, newest first: messages addressed to it plus
   * broadcasts it didn't send. Answered/acked messages are dropped unless
   * `includeRead`. */
  async mailboxInbox(name: string, includeRead = false, limit = 0): Promise<Mail[]> {
    const q = new URLSearchParams({ name });
    if (includeRead) q.set("all", "true");
    if (limit > 0) q.set("limit", String(limit));
    const out = await this.getJSON<{ waiting: Mail[] }>(PATHS.mailboxInbox + "?" + q.toString());
    return out.waiting ?? [];
  }

  /** Mark a message read for one reader (it leaves that reader's inbox without
   * a reply). Per-reader and idempotent. */
  async mailboxAck(messageId: string, by: string): Promise<void> {
    const row: MailAck = { by };
    const res = await this.fetch(
      "POST",
      PATHS.mailboxAck.replace("{id}", encodeURIComponent(messageId)),
      JSON.stringify(mailAckToWire(row)),
    );
    if (!res.ok) throw await apiError(res);
    await res.body?.cancel();
  }

  /** The answers to a sent message, oldest first (conversation order). */
  async mailboxReplies(messageId: string, limit = 0): Promise<Mail[]> {
    let path = PATHS.mailboxReplies.replace("{id}", encodeURIComponent(messageId));
    if (limit > 0) path += "?limit=" + limit;
    const out = await this.getJSON<{ replies: Mail[] }>(path);
    return out.replies ?? [];
  }

  /** Recent mailbox messages, newest first, optionally one topic's. */
  async mailboxMessages(topic = "", limit = 0): Promise<Mail[]> {
    const q = new URLSearchParams();
    if (topic) q.set("topic", topic);
    if (limit > 0) q.set("limit", String(limit));
    const qs = q.toString();
    const out = await this.getJSON<{ messages: Mail[] }>(
      PATHS.mailboxMessages + (qs ? "?" + qs : ""),
    );
    return out.messages ?? [];
  }

  /** The mailbox's topics with their message counts. */
  async mailboxTopics(): Promise<Record<string, number>> {
    const out = await this.getJSON<{ topics: Record<string, number> }>(PATHS.mailboxTopics);
    return out.topics ?? {};
  }

  /** Stream new mail the moment it lands — the push counterpart of polling
   * `mailboxInbox`. `name` watches one agent/app's mail (messages addressed to
   * it plus broadcasts it didn't send); `topic` watches one topic; neither
   * tails every board message. The server's first frame is a `ready` marker —
   * messages sent after it are guaranteed delivered. Iterates until the
   * connection closes or the consumer breaks out.
   *
   * Note: pass `timeoutMs: 0`-like long timeouts via ClientOptions if the
   * default 30s would cut a quiet watch short. */
  async *mailboxWatch(name = "", topic = ""): AsyncGenerator<Mail> {
    const q = new URLSearchParams();
    if (name) q.set("name", name);
    if (topic) q.set("topic", topic);
    const qs = q.toString();
    const res = await this.fetch(
      "GET",
      PATHS.mailboxWatch + (qs ? "?" + qs : ""),
      undefined,
      "text/event-stream",
    );
    if (!res.ok) throw await apiError(res);
    if (!res.body) return;
    for await (const ev of parseSSE(res.body)) {
      if (ev.event === "mail") yield ev.data as unknown as Mail;
    }
  }

  // --- internals ---

  private async getJSON<T>(path: string): Promise<T> {
    const res = await this.fetch("GET", path);
    if (!res.ok) throw await apiError(res);
    return (await res.json()) as T;
  }

  private fetch(method: string, path: string, body?: string, accept = "application/json"): Promise<Response> {
    const headers: Record<string, string> = {
      Authorization: "Bearer " + this.token,
      Accept: accept,
    };
    if (body !== undefined) headers["Content-Type"] = "application/json";
    if (this.tenant) headers["X-Agezt-Tenant"] = this.tenant;
    const ac = new AbortController();
    const timer = setTimeout(() => ac.abort(), this.timeoutMs);
    return fetch(this.baseUrl + path, { method, headers, body, signal: ac.signal }).finally(() =>
      clearTimeout(timer),
    );
  }
}

async function apiError(res: Response): Promise<APIError> {
  let type = "";
  let detail = "";
  try {
    const body = (await res.json()) as Record<string, unknown>;
    const err = body.error;
    if (err && typeof err === "object") {
      const e = err as Record<string, unknown>;
      type = String(e.type ?? "");
      detail = String(e.message ?? "");
    } else if (typeof err === "string") {
      // failed-run body: { status: "failed", error: "…" }
      type = String(body.status ?? "");
      detail = err;
    }
  } catch {
    /* non-JSON body */
  }
  return new APIError(res.status, type, detail);
}

/** Parse a text/event-stream ReadableStream into StreamEvents. */
async function* parseSSE(stream: ReadableStream<Uint8Array>): AsyncGenerator<StreamEvent> {
  const reader = stream.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true });
      let sep: number;
      while ((sep = indexOfFrameEnd(buf)) >= 0) {
        const frame = buf.slice(0, sep);
        buf = buf.slice(sep).replace(/^(\r?\n){1,2}/, "");
        const ev = parseFrame(frame);
        if (ev) yield ev;
      }
    }
  } finally {
    // Cancel — not merely releaseLock — so an early consumer exit (`break` in a
    // for-await loop, or an exception in the loop body) aborts the underlying
    // HTTP request and closes the socket. releaseLock() alone detaches the
    // reader while the (still unread) request body keeps the connection open,
    // leaking one socket per early-terminated stream. On normal completion
    // (done=true) cancel() resolves immediately; a rejection (stream already
    // errored) is swallowed so the consumer's original error still surfaces.
    await reader.cancel().catch(() => {});
  }
  const ev = parseFrame(buf);
  if (ev) yield ev;
}

// indexOfFrameEnd returns the index of the blank-line separator (\n\n or
// \r\n\r\n) ending the first frame, or -1.
function indexOfFrameEnd(s: string): number {
  const a = s.indexOf("\n\n");
  const b = s.indexOf("\r\n\r\n");
  if (a < 0) return b;
  if (b < 0) return a;
  return Math.min(a, b);
}

function parseFrame(frame: string): StreamEvent | null {
  let event = "message";
  const dataLines: string[] = [];
  for (const raw of frame.split("\n")) {
    const line = raw.replace(/\r$/, "");
    if (line === "" || line.startsWith(":")) continue;
    if (line.startsWith("event:")) event = line.slice(6).trim();
    else if (line.startsWith("data:")) dataLines.push(line.slice(5).replace(/^ /, ""));
  }
  if (dataLines.length === 0) return null;
  const joined = dataLines.join("\n");
  let data: Record<string, unknown>;
  try {
    data = JSON.parse(joined) as Record<string, unknown>;
  } catch {
    data = { raw: joined };
  }
  return { event, data };
}
