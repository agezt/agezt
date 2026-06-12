/**
 * Agent SDK client for AI agent subprocess code to communicate with AGEZT.
 *
 * This module provides an `AgentClient` that connects to the AGEZT Agent Gateway
 * via a Unix domain socket. The gateway accepts scoped JWT tokens that limit
 * what capabilities the agent subprocess can access.
 *
 * @example
 * ```ts
 * import { AgentClient } from "@agezt/sdk";
 *
 * const client = new AgentClient({
 *   token: process.env.AGEZT_AGENT_TOKEN!,
 * });
 *
 * // Remember a fact
 * const record = await client.memory.write({
 *   type: "fact",
 *   subject: "test",
 *   content: "This is a test",
 * });
 *
 * // Search memories
 * const results = await client.memory.search("test");
 * for (const r of results) {
 *   console.log(r.subject, r.content);
 * }
 *
 * // Publish an event
 * await client.eventbus.publish("my.event", { key: "value" });
 *
 * // Subscribe to events
 * for await (const event of client.eventbus.subscribe("my.>")) {
 *   console.log(event);
 * }
 * ```
 */

import * as http from "node:http";
import { ConfigAccessError } from "./errors.js";

/** Default socket path for the agent gateway. */
export const DEFAULT_SOCKET_PATH = "@agezt/agentgw.sock";

/** Capability constants for agent subprocess tokens. */
export const Capability = {
  // Eventbus capabilities
  EVENTBUS_PUBLISH: "eventbus.publish",
  EVENTBUS_SUBSCRIBE: "eventbus.subscribe",

  // Memory capabilities
  MEMORY_READ: "memory.read",
  MEMORY_WRITE: "memory.write",
  MEMORY_DELETE: "memory.delete",
  MEMORY_SEARCH: "memory.search",
  MEMORY_LIST: "memory.list",

  // Log capabilities
  LOG_READ: "log.read",
  LOG_WRITE: "log.write",

  // Agent capabilities
  AGENT_LIST: "agent.list",
  AGENT_QUERY: "agent.query",

  // Config capabilities
  CONFIG_ACCESS: "config.access",
  CONFIG_LIST: "config.list",
  CONFIG_SEARCH: "config.search",
} as const;

export type Capability = (typeof Capability)[keyof typeof Capability];

/** Error from the agent gateway. */
export class AgentError extends Error {
  constructor(
    public readonly code: string,
    public readonly message: string,
    public readonly statusCode: number = 500
  ) {
    super(`[${code}] ${message}`);
    this.name = "AgentError";
  }
}

/** Memory record structure. */
export interface MemoryRecord {
  id: string;
  type: string;
  subject: string;
  content: string;
  confidence: number;
  last_seen_ms: number;
  tags?: Record<string, string>;
}

/** Search result with relevance score. */
export interface SearchResult {
  id: string;
  type: string;
  subject: string;
  content: string;
  confidence: number;
  last_seen_ms: number;
  score: number;
}

/** Agent profile from the roster. */
export interface AgentProfile {
  id: string;
  slug: string;
  name: string;
  model: string;
  enabled: boolean;
  retired: boolean;
}

/** Event from the bus. */
export interface BusEvent {
  id: string;
  seq: number;
  ts_unix_ms: number;
  subject: string;
  actor: string;
  kind: string;
  correlation_id?: string;
  causation_id?: string;
  payload?: unknown;
  tags?: Record<string, string>;
}

/** Options for creating an AgentClient. */
export interface AgentClientOptions {
  /** JWT capability token from the parent agent. */
  token: string;
  /** Path to the agent gateway Unix socket. */
  socketPath?: string;
  /** Per-request timeout in milliseconds (default 30000). */
  timeoutMs?: number;
}

/**
 * Synchronous client for AI agent subprocess code to access AGEZT.
 *
 * Connects to the AGEZT Agent Gateway via a Unix domain socket and authenticates
 * using a scoped JWT token. The token is typically provided by the parent agent
 * via environment variable `AGEZT_AGENT_TOKEN`.
 */
export class AgentClient {
  private readonly token: string;
  private readonly socketPath: string;
  private readonly timeoutMs: number;

  /** Memory operations handle. */
  readonly memory: MemoryHandle;
  /** Eventbus operations handle. */
  readonly eventbus: EventbusHandle;
  /** Log operations handle. */
  readonly log: LogHandle;
  /** Agent operations handle. */
  readonly agent: AgentHandle;
  /** Config operations handle. */
  readonly config: ConfigHandle;

  constructor(opts: AgentClientOptions) {
    this.token = opts.token;
    this.socketPath = opts.socketPath ?? DEFAULT_SOCKET_PATH;
    this.timeoutMs = opts.timeoutMs ?? 30000;

    this.memory = new MemoryHandle(this);
    this.eventbus = new EventbusHandle(this);
    this.log = new LogHandle(this);
    this.agent = new AgentHandle(this);
    this.config = new ConfigHandle(this);
  }

  /** @internal Make a GET request to the gateway. */
  async get<T>(path: string): Promise<T> {
    return this.request<T>("GET", path);
  }

  /** @internal Make a POST request to the gateway. */
  async post<T>(path: string, body: Record<string, unknown>): Promise<T> {
    return this.request<T>("POST", path, body);
  }

  /** @internal Make a DELETE request to the gateway. */
  async delete<T>(path: string): Promise<T> {
    return this.request<T>("DELETE", path);
  }

  /** @internal Make a request to the gateway via Node.js HTTP-over-Unix socket. */
  private async request<T>(method: string, path: string, body?: Record<string, unknown>): Promise<T> {
    return new Promise((resolve, reject) => {
      const options: http.RequestOptions = {
        path,
        method,
        socketPath: this.socketPath,
        timeout: this.timeoutMs,
        headers: {
          Accept: "application/json",
          Authorization: `Bearer ${this.token}`,
          "Content-Type": "application/json",
        },
      };

      if (body !== undefined) {
        const bodyStr = JSON.stringify(body);
        (options.headers as Record<string, string>)["Content-Length"] = Buffer.byteLength(bodyStr).toString();
      }

      const req = http.request(options, (res) => {
        const chunks: Buffer[] = [];
        res.on("data", (chunk: Buffer) => chunks.push(chunk));
        res.on("end", () => {
          const respBody = Buffer.concat(chunks).toString();

          if (res.statusCode === 401) {
            reject(new AgentError("UNAUTHORIZED", "Invalid or missing token", 401));
            return;
          }
          if (res.statusCode === 403) {
            reject(new AgentError("FORBIDDEN", "Capability not granted", 403));
            return;
          }
          if (res.statusCode === 429) {
            reject(new AgentError("RATE_LIMITED", "Too many requests", 429));
            return;
          }
          if (res.statusCode && res.statusCode >= 400) {
            try {
              const err = JSON.parse(respBody);
              reject(new AgentError(err.error?.code ?? "ERROR", err.error?.message ?? respBody, res.statusCode));
            } catch {
              reject(new AgentError("HTTP_ERROR", respBody, res.statusCode ?? 500));
            }
            return;
          }

          try {
            resolve(respBody ? (JSON.parse(respBody) as T) : ({} as T));
          } catch {
            reject(new AgentError("INVALID_RESPONSE", "Cannot parse JSON response", 500));
          }
        });
      });

      req.on("error", (err) => {
        if ((err as NodeJS.ErrnoException).code === "ENOENT" || (err as NodeJS.ErrnoException).code === "ECONNREFUSED") {
          reject(new AgentError("CONNECTION_ERROR", `Cannot connect to gateway at ${this.socketPath}`, 503));
        } else {
          reject(new AgentError("REQUEST_ERROR", err.message, 500));
        }
      });

      req.on("timeout", () => {
        req.destroy();
        reject(new AgentError("TIMEOUT", `Request timed out after ${this.timeoutMs}ms`, 408));
      });

      if (body !== undefined) {
        req.write(JSON.stringify(body));
      }
      req.end();
    });
  }
}

/** Handle for memory operations. */
export class MemoryHandle {
  constructor(private client: AgentClient) {}

  /**
   * Remember (write) a memory record.
   *
   * @example
   * ```ts
   * const record = await client.memory.write({
   *   type: "fact",
   *   subject: "API endpoint",
   *   content: "Use POST /v1/memory/write",
   * });
   * console.log(record.id);
   * ```
   */
  async write(opts: {
    type?: string;
    subject?: string;
    content?: string;
    tags?: Record<string, string>;
  }): Promise<MemoryRecord> {
    const result = await this.client.post<{ record: MemoryRecord }>("/v1/memory/write", {
      type: opts.type ?? "fact",
      subject: opts.subject ?? "",
      content: opts.content ?? "",
      tags: opts.tags,
    });
    return result.record;
  }

  /**
   * Recall (search) memories matching a query.
   *
   * @example
   * ```ts
   * const results = await client.memory.search("API endpoint");
   * for (const r of results) {
   *   console.log(r.subject, r.score);
   * }
   * ```
   */
  async search(query: string, limit = 20): Promise<SearchResult[]> {
    const result = await this.client.get<{ results: SearchResult[] }>(
      `/v1/memory/search?q=${encodeURIComponent(query)}&limit=${limit}`
    );
    return result.results;
  }

  /**
   * Forget (delete) a specific memory record.
   *
   * @example
   * ```ts
   * const deleted = await client.memory.delete("01HABC...");
   * console.log(deleted);
   * ```
   */
  async delete(id: string): Promise<boolean> {
    const result = await this.client.delete<{ deleted: boolean }>(`/v1/memory/delete?id=${encodeURIComponent(id)}`);
    return result.deleted;
  }
}

/** Handle for eventbus operations. */
export class EventbusHandle {
  constructor(private client: AgentClient) {}

  /**
   * Publish an event to the bus.
   *
   * @example
   * ```ts
   * await client.eventbus.publish("agent.progress", {
   *   step: 1,
   *   total: 3,
   * });
   * ```
   */
  async publish(event: string, payload?: Record<string, unknown>): Promise<void> {
    await this.client.post("/v1/eventbus/publish", {
      event,
      payload,
    });
  }

  /**
   * Subscribe to events matching a pattern.
   *
   * @example
   * ```ts
   * for await (const ev of client.eventbus.subscribe("agent.>")) {
   *   console.log(ev.subject, ev.payload);
   * }
   * ```
   *
   * Note: This requires Node.js and the `node:http` module.
   * It uses Server-Sent Events (SSE) for streaming.
   */
  async *subscribe(pattern = ">"): AsyncGenerator<BusEvent> {
    const url = new URL(`http://localhost/v1/eventbus/subscribe?pattern=${encodeURIComponent(pattern)}`);

    const options: http.RequestOptions = {
      path: url.pathname + url.search,
      method: "GET",
      socketPath: (this.client as unknown as { socketPath: string }).socketPath,
      headers: {
        Accept: "text/event-stream",
        Authorization: `Bearer ${(this.client as unknown as { token: string }).token}`,
      },
    };

    const stream = await new Promise<ReadableStream<BusEvent>>((resolve, reject) => {
      const req = http.request(options, (res) => {
        if (!res.statusCode || res.statusCode >= 400) {
          reject(new AgentError("SUBSCRIBE_ERROR", `HTTP ${res.statusCode}`, res.statusCode ?? 500));
          return;
        }

        const stream = new ReadableStream<BusEvent>({
          start(controller) {
            // SSE parsing
            const decoder = new TextDecoder();
            let buffer = "";

            res.on("data", (chunk: Buffer) => {
              buffer += decoder.decode(chunk, { stream: true });

              // Process complete SSE messages
              const lines = buffer.split(/\r?\n/);
              buffer = lines.pop() ?? "";

              for (const line of lines) {
                if (line.startsWith("data:")) {
                  const data = line.slice(5).trim();
                  try {
                    const event = JSON.parse(data) as BusEvent;
                    controller.enqueue(event);
                  } catch {
                    // Skip invalid JSON
                  }
                }
              }
            });

            res.on("end", () => controller.close());
            res.on("error", (err) => controller.error(err));
          },
        });

        resolve(stream);
      });

      req.on("error", reject);
      req.on("timeout", () => {
        req.destroy();
        reject(new AgentError("TIMEOUT", "Subscription timed out", 408));
      });

      req.end();
    });

    const reader = stream.getReader();
    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        if (value) yield value;
      }
    } finally {
      reader.releaseLock();
    }
  }
}

/** Handle for log operations. */
export class LogHandle {
  constructor(private client: AgentClient) {}

  /**
   * Write a log entry.
   *
   * @example
   * ```ts
   * await client.log.write("Processing item", {
   *   level: "info",
   *   meta: { itemId: "123" },
   * });
   * ```
   */
  async write(message: string, opts?: { level?: string; meta?: Record<string, unknown> }): Promise<void> {
    await this.client.post("/v1/log/write", {
      level: opts?.level ?? "info",
      message,
      meta: opts?.meta,
    });
  }
}

/** Handle for agent operations. */
export class AgentHandle {
  constructor(private client: AgentClient) {}

  /**
   * List all agents in the roster.
   *
   * @example
   * ```ts
   * const agents = await client.agent.list();
   * for (const a of agents) {
   *   console.log(a.name, a.model);
   * }
   * ```
   */
  async list(): Promise<AgentProfile[]> {
    const result = await this.client.get<{ agents: AgentProfile[] }>("/v1/agent/list");
    return result.agents;
  }

  /**
   * Query a specific agent's status.
   *
   * @example
   * ```ts
   * const agent = await client.agent.query("claude");
   * console.log(agent.name, agent.enabled);
   * ```
   */
  async query(id: string): Promise<AgentProfile> {
    return this.client.get<AgentProfile>(`/v1/agent/query?id=${encodeURIComponent(id)}`);
  }
}

/** Handle for config operations. */
export class ConfigHandle {
  constructor(private client: AgentClient) {}

  /**
   * Get a config value by key.
   *
   * Config values are rated based on sensitivity:
   * - ``public``: Endpoint URLs, version info (auto-allowed)
   * - ``internal``: Application settings (auto-allowed)
   * - ``restricted``: Semi-sensitive values (requires HITL approval)
   * - ``secret``: API keys, tokens, passwords (always denied)
   *
   * @example
   * ```ts
   * // Public/internal values — automatic access
   * const endpoint = await client.config.get("analytics:endpoint");
   * const version = await client.config.get("app:version");
   *
   * // Secret values — always denied
   * try {
   *   const apiKey = await client.config.get("github:token");
   * } catch (err) {
   *   if (err instanceof ConfigAccessError) {
   *     console.log(`Access denied: ${err.code}`);
   *   }
   * }
   *
   * // Restricted values — may require HITL approval
   * try {
   *   const awsKey = await client.config.get(
   *     "aws:readonly_key",
   *     "Need AWS credentials to read S3 data"
   *   );
   * } catch (err) {
   *   if (err instanceof ConfigAccessError) {
   *     console.log(`Approval denied or timed out: ${err.message}`);
   *   }
   * }
   * ```
   *
   * @param key - Config key (e.g., "github:endpoint", "analytics:api_key")
   * @param reason - Why the agent needs this value (shown to operator for HITL approval)
   * @returns The config value
   * @throws {ConfigAccessError} When access is denied
   */
  async get(key: string, reason?: string): Promise<string> {
    let path = `/v1/config/${encodeURIComponent(key)}`;
    if (reason) {
      path += `?reason=${encodeURIComponent(reason)}`;
    }

    try {
      const result = await this.client.get<{ value: string }>(path);
      return result.value ?? "";
    } catch (err) {
      if (err instanceof ConfigAccessError) throw err;
      // Wrap other errors
      throw new ConfigAccessError(key, "INTERNAL_ERROR", String(err), 500);
    }
  }

  /**
   * List accessible config keys.
   *
   * Returns only keys with ``public`` or ``internal`` ratings.
   * ``secret`` and ``restricted`` keys are excluded.
   *
   * @example
   * ```ts
   * const keys = await client.config.listKeys();
   * for (const key of keys) {
   *   console.log(key);
   * }
   * ```
   */
  async listKeys(): Promise<string[]> {
    const result = await this.client.get<{ keys: string[] }>("/v1/config");
    return result.keys ?? [];
  }

  /**
   * Search config keys by prefix.
   *
   * Only ``public`` rating keys are searchable.
   *
   * @example
   * ```ts
   * const results = await client.config.search("github:");
   * for (const r of results) {
   *   console.log(r.key, r.description);
   * }
   * ```
   *
   * @param query - Search prefix (e.g., "github:", "analytics:")
   * @returns Matching entries with ``key`` and ``description``
   */
  async search(query: string): Promise<Array<{ key: string; description?: string }>> {
    const path = `/v1/config/search?q=${encodeURIComponent(query)}`;
    const result = await this.client.get<{ results: Array<{ key: string; description?: string }> }>(path);
    return result.results ?? [];
  }
}
