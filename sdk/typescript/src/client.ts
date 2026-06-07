import { APIError } from "./errors.js";

/** Daemon health summary (`GET /api/v1/health`). */
export interface Health {
  status: string;
  version: string;
  default_model: string;
  model_count: number;
}

/** Available models (`GET /api/v1/models`). */
export interface Models {
  default: string;
  models: string[];
}

/** A completed (non-streaming) run. */
export interface RunResult {
  correlation_id: string;
  model: string;
  status: string;
  answer: string;
}

/** One Server-Sent Event from a streaming run. `event` is one of
 * `start` | `token` | `done` | `error`; `data` is the decoded JSON payload. */
export interface StreamEvent {
  event: string;
  data: Record<string, unknown>;
}

/** The journaled event arc of a past run (`GET /api/v1/runs/{id}`). */
export interface RunArc {
  correlation_id: string;
  count: number;
  events: unknown[];
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
    return this.getJSON<Health>("/api/v1/health");
  }

  models(): Promise<Models> {
    return this.getJSON<Models>("/api/v1/models");
  }

  /** Execute an intent and resolve with the final answer (blocking). Throws
   * APIError if the run fails or the request is rejected. */
  async run(intent: string, model?: string): Promise<RunResult> {
    const body: Record<string, unknown> = { intent };
    if (model) body.model = model;
    const res = await this.fetch("POST", "/api/v1/runs", JSON.stringify(body));
    if (!res.ok) throw await apiError(res);
    return (await res.json()) as RunResult;
  }

  /** Execute an intent, yielding StreamEvents (start/token/done/error) as the
   * agent produces them. */
  async *runStream(intent: string, model?: string): AsyncGenerator<StreamEvent> {
    const body: Record<string, unknown> = { intent, stream: true };
    if (model) body.model = model;
    const res = await this.fetch("POST", "/api/v1/runs", JSON.stringify(body), "text/event-stream");
    if (!res.ok) throw await apiError(res);
    if (!res.body) return;
    yield* parseSSE(res.body);
  }

  getRun(correlationId: string): Promise<RunArc> {
    return this.getJSON<RunArc>("/api/v1/runs/" + encodeURIComponent(correlationId));
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
    reader.releaseLock();
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
