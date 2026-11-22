// SPDX-License-Identifier: MIT

// GENERATED FILE — do not edit by hand.
//
// Source of truth: the daemon's operation registry and the console's route
// catalogue (kernel/contract/opapi/http.go).
// Regenerate:       go run ./tools/openapi
// Verify:          go run ./tools/openapi -check
//
// The path unions below ARE the console's HTTP surface. A path that is not
// named here is not a path the console answers. A verb that does not match
// the route's kind does not type-check. A parameter on a parameterless read
// does not type-check. The browser and the daemon cannot disagree about a
// route without the compiler saying so first.

import { HTTPError, authHeaders, errMsg } from "./transport";
import type { RequestOf, ResponseOf } from "./types.generated";

/** A parameterless read: one GET, one operation, no arguments. */
export type ReadPath =
  | "/api/acp/agents"
  | "/api/approvals"
  | "/api/board/help"
  | "/api/budget"
  | "/api/catalog"
  | "/api/chains"
  | "/api/channels"
  | "/api/config"
  | "/api/config/schema"
  | "/api/config/values"
  | "/api/council/members"
  | "/api/data/collections"
  | "/api/edict_show"
  | "/api/execution_profile_check"
  | "/api/execution_profiles"
  | "/api/mcp"
  | "/api/memory/audit"
  | "/api/nodes"
  | "/api/persona"
  | "/api/policy"
  | "/api/prompts"
  | "/api/pulse"
  | "/api/routing"
  | "/api/sandbox"
  | "/api/schedule/system_tasks"
  | "/api/schedules"
  | "/api/skills"
  | "/api/spend/today"
  | "/api/standing"
  | "/api/status"
  | "/api/tools_catalog"
  | "/api/version"
  | "/api/workflows/templates"
  | "/api/world";

/** A read whose arguments come from the query string. */
export type ReadArgsPath =
  | "/api/agents"
  | "/api/agents/activity"
  | "/api/agents/escalations"
  | "/api/agents/impact"
  | "/api/agents/permissions"
  | "/api/agents/repair_status"
  | "/api/approvals_log"
  | "/api/artifacts"
  | "/api/attention"
  | "/api/autonomy"
  | "/api/board"
  | "/api/configcenter/list"
  | "/api/data/records"
  | "/api/edict/test"
  | "/api/inbox"
  | "/api/journal"
  | "/api/market"
  | "/api/market/show"
  | "/api/market/sources"
  | "/api/memory"
  | "/api/memory_log"
  | "/api/netguard_log"
  | "/api/plan_history"
  | "/api/policy_log"
  | "/api/provider/keys"
  | "/api/provider_log"
  | "/api/ratelimit_log"
  | "/api/reaper/scan"
  | "/api/runs"
  | "/api/sandbox_file"
  | "/api/schedule/fires"
  | "/api/schedule/test"
  | "/api/skill/files"
  | "/api/skills/hygiene"
  | "/api/standing/why"
  | "/api/suggestions"
  | "/api/tool_log"
  | "/api/warden_log"
  | "/api/webhook_log"
  | "/api/workflows"
  | "/api/workflows/runs"
  | "/api/workflows/show"
  | "/api/world_log";

/** A mutation whose arguments come from the query string. */
export type WritePath =
  | "/api/agents/enable"
  | "/api/agents/retire"
  | "/api/agents/revive"
  | "/api/artifact/collect"
  | "/api/artifact/delete"
  | "/api/cancel_run"
  | "/api/data/delete"
  | "/api/decide"
  | "/api/edict/deny_add"
  | "/api/edict/deny_rm"
  | "/api/edict/set_level"
  | "/api/edict/set_mode"
  | "/api/halt"
  | "/api/mcp/attach"
  | "/api/mcp/detach"
  | "/api/mcp/enable"
  | "/api/mcp/remove"
  | "/api/memory/clean"
  | "/api/memory/forget"
  | "/api/memory/promote"
  | "/api/memory/prune"
  | "/api/memory/tidy"
  | "/api/profile/rebuild"
  | "/api/provider/keys/activate"
  | "/api/provider/keys/remove"
  | "/api/pulse/beat"
  | "/api/pulse/cadence"
  | "/api/pulse/dial"
  | "/api/pulse/flush"
  | "/api/pulse/pause"
  | "/api/pulse/probe"
  | "/api/pulse/quiet"
  | "/api/pulse/resume"
  | "/api/pulse/unwatch"
  | "/api/pulse/watch"
  | "/api/resume"
  | "/api/run/pause"
  | "/api/run/resume"
  | "/api/run/steer"
  | "/api/run/step"
  | "/api/sandbox/delete"
  | "/api/schedule/enable"
  | "/api/schedule/remove"
  | "/api/schedule/run"
  | "/api/skill/archive"
  | "/api/skill/promote"
  | "/api/skill/quarantine"
  | "/api/skill/revert"
  | "/api/skill/share"
  | "/api/standing/enable"
  | "/api/standing/fire"
  | "/api/standing/remove"
  | "/api/workflows/enable"
  | "/api/workflows/remove"
  | "/api/world/forget"
  | "/api/world/relate";

/** A mutation whose arguments come from the JSON body. */
export type JsonPath =
  | "/api/agents/add"
  | "/api/agents/capabilities"
  | "/api/agents/edit"
  | "/api/agents/remove"
  | "/api/agents/repair"
  | "/api/agents/resolve"
  | "/api/agents/task"
  | "/api/agents/wake"
  | "/api/board/ack"
  | "/api/board/send"
  | "/api/catalog/sync"
  | "/api/chains/set"
  | "/api/channel/account/remove"
  | "/api/channel/account/set"
  | "/api/channel/oauth/start"
  | "/api/channel/oauth/status"
  | "/api/chat/summarize"
  | "/api/config/schema/register"
  | "/api/config/schema/unregister"
  | "/api/config/set"
  | "/api/configcenter/access"
  | "/api/configcenter/delete"
  | "/api/configcenter/rating"
  | "/api/configcenter/set"
  | "/api/council/ask"
  | "/api/council/set"
  | "/api/data/insert"
  | "/api/data/update"
  | "/api/market/source/add"
  | "/api/market/source/remove"
  | "/api/market/sync"
  | "/api/mcp/add"
  | "/api/memory/add"
  | "/api/memory/bulk_forget"
  | "/api/memory/supersede"
  | "/api/persona/set"
  | "/api/plan/generate"
  | "/api/plan/refine"
  | "/api/prompts/set"
  | "/api/provider/connect"
  | "/api/provider/keys/add"
  | "/api/provider/oauth/import"
  | "/api/provider/oauth/logout"
  | "/api/provider/oauth/start"
  | "/api/provider/oauth/status"
  | "/api/provider/probe"
  | "/api/provider/reload"
  | "/api/redact/test"
  | "/api/research/ask"
  | "/api/routing/set"
  | "/api/schedule/add"
  | "/api/schedule/edit"
  | "/api/send"
  | "/api/skill/import"
  | "/api/standing/add"
  | "/api/standing/edit"
  | "/api/whatsappgw/qr"
  | "/api/whatsappgw/status"
  | "/api/workflows/draft"
  | "/api/workflows/refine"
  | "/api/workflows/run"
  | "/api/workflows/save"
  | "/api/workflows/test_node"
  | "/api/world/add"
  | "/api/world/edit";

/** A mutation that streams frames; the console binds a bespoke handler. */
export type StreamPath =
  | "/api/market/install"
  | "/api/market/uninstall"
  | "/api/plan/run"
  | "/api/run"
  | "/api/toolbox/install";

/** The console's own endpoints: not operation bindings, but paths it answers. */
export type NativePath =
  | "/api/artifact/raw"
  | "/api/authmeta"
  | "/api/files/delete"
  | "/api/files/mkdir"
  | "/api/files/raw"
  | "/api/files/rename"
  | "/api/files/tree"
  | "/api/login"
  | "/api/logout"
  | "/api/rollback/apply"
  | "/api/rollback/checkpoints"
  | "/api/sse-token"
  | "/api/transcribe"
  | "/api/tts"
  | "/api/voice/status";

/** Every path the console answers for an operation. */
export type ApiPath = ReadPath | ReadArgsPath | WritePath | JsonPath | StreamPath;

/** RequestOptions are the per-call knobs the transport honours. */
export interface RequestOptions {
  signal?: AbortSignal;
  timeoutMs?: number;
}

/**
 * RuntimePath is a path that arrives at RUNTIME — a component that is handed
 * the path as a prop. The brand is what keeps the guard honest: a path
 * LITERAL cannot be a RuntimePath, so a literal is always checked against the
 * catalogue, and only a value the caller deliberately marked as runtime skips
 * that check (its own prop type is pinned to the catalogue instead).
 */
declare const runtimePathBrand: unique symbol;
export type RuntimePath = {
  readonly [runtimePathBrand]: true;
} & (ReadPath | ReadArgsPath | NativePath);

/** isRequestOptions tells a query bag from a request option bag. */
function isRequestOptions(value: unknown): value is RequestOptions {
  if (typeof value !== "object" || value === null) return false;
  const candidate = value as Record<string, unknown>;
  return "signal" in candidate || "timeoutMs" in candidate;
}

export async function getJSON<P extends ReadPath>(
  path: P,
  options?: RequestOptions,
): Promise<ResponseOf<P>>;
export async function getJSON<P extends ReadArgsPath | NativePath>(
  path: P,
  params?: Record<string, string>,
  options?: RequestOptions,
): Promise<ResponseOf<P>>;
export async function getJSON<P extends RuntimePath>(
  path: P,
  params?: Record<string, string>,
  options?: RequestOptions,
): Promise<ResponseOf<P>>;
export async function getJSON<P extends ReadPath | ReadArgsPath | NativePath | RuntimePath>(
  path: P,
  paramsOrOptions?: Record<string, string> | RequestOptions,
  options?: RequestOptions,
): Promise<ResponseOf<P>> {
  const raw = paramsOrOptions as Record<string, string> | RequestOptions | undefined;
  const query = isRequestOptions(raw) ? undefined : raw;
  const callOptions = isRequestOptions(raw) ? raw : options;
  return requestGet<ResponseOf<P>>(path, query, callOptions);
}

/** requestGet performs the GET itself. */
async function requestGet<T>(
  path: string,
  params: Record<string, string> | undefined,
  callOptions: RequestOptions | undefined,
): Promise<T> {
  const query = new URLSearchParams(params || {}).toString();
  const url = query ? path + "?" + query : path;
  // Compose a request-scoped AbortSignal so callers can pass their own AND/OR a timeout. When either
  // signal aborts (caller-driven cancellation or timeout), the underlying fetch rejects with a
  // DOMException + the user's catch handler renders the existing error path. timeoutMs is opt-in;
  // leaving it undefined keeps the original "no client-side cap" behavior for the long-poll / SSE-
  // adjacent code that already manages its own cancellation.
  const externalSignal = callOptions?.signal;
  let timeoutController: AbortController | null = null;
  let signals: AbortSignal[] = [];
  if (externalSignal) signals.push(externalSignal);
  if (typeof callOptions?.timeoutMs === "number" && callOptions.timeoutMs > 0) {
    timeoutController = new AbortController();
    const t = setTimeout(() => timeoutController!.abort(), callOptions.timeoutMs);
    // tie the timeout out to the external signal so an early abort clears the timer (no stray abort).
    externalSignal?.addEventListener("abort", () => {
      clearTimeout(t);
      try { timeoutController?.abort(); } catch { /* already aborted */ }
    });
    signals.push(timeoutController.signal);
  }
  let composed: AbortSignal | undefined;
  if (signals.length === 1) {
    composed = signals[0];
  } else if (signals.length > 1) {
    composed = AbortSignal.any(signals);
  }
  try {
    const res = await fetch(url, {
      headers: authHeaders({ Accept: "application/json" }),
      ...(composed ? { signal: composed } : {}),
    });
    if (!res.ok) throw new HTTPError(res.status, url, await errMsg(res));
    return res.json() as Promise<T>;
  } catch (err) {
    if ((err as { name?: string }).name === "AbortError") {
      // Surface as a regular HTTP-shaped error so the caller's existing "if (!res.ok) throw" path
      // can treat it like any other failure (Roster's catch already handles it).
      throw new HTTPError(0, url, "request aborted" + (timeoutController?.signal.aborted ? " (timeout)" : ""));
    }
    throw err;
  }
}

export async function postJSON<P extends JsonPath | NativePath>(
  path: P,
  body: RequestOf<P>,
  options?: { signal?: AbortSignal },
): Promise<ResponseOf<P>> {
  return postBody<ResponseOf<P>>(path, body, options);
}

/** postJSONAt is the runtime-path twin of postJSON. */
export async function postJSONAt<T = any>(
  path: JsonPath | NativePath,
  body: unknown,
  options?: { signal?: AbortSignal },
): Promise<T> {
  return postBody<T>(path, body, options);
}

export async function postAction<P extends WritePath | NativePath>(
  path: P,
  params?: Record<string, string>,
): Promise<ResponseOf<P>> {
  return postQuery<ResponseOf<P>>(path, params);
}

/** postActionAt is the runtime-path twin of postAction. */
export async function postActionAt<T = any>(
  path: WritePath | NativePath,
  params?: Record<string, string>,
): Promise<T> {
  return postQuery<T>(path, params);
}

/** postBody performs a body-carrying POST: the body is always present, even
 * when it is empty, because the console decodes it. */
async function postBody<T>(
  path: string,
  body: unknown,
  options?: { signal?: AbortSignal },
): Promise<T> {
  const res = await fetch(path, {
    method: "POST",
    headers: authHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify(body ?? {}),
    signal: options?.signal,
  });
  if (!res.ok) throw new HTTPError(res.status, path, await errMsg(res));
  return res.json() as Promise<T>;
}

/** postQuery performs a POST whose arguments ride in the query string. */
async function postQuery<T>(path: string, params?: Record<string, string>): Promise<T> {
  const query = new URLSearchParams(params || {}).toString();
  const url = query ? path + "?" + query : path;
  const res = await fetch(url, { method: "POST", headers: authHeaders() });
  if (!res.ok) throw new HTTPError(res.status, url, await errMsg(res));
  return res.json() as Promise<T>;
}
