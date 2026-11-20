// SPDX-License-Identifier: MIT

package main

// The TypeScript client's renderer.
//
// The client is the console's HTTP surface, written down once. It is GENERATED
// because the surface is derived: the route catalogue says which path answers
// which operation and which verb it takes, and the console's own endpoint
// list says which paths are the console's rather than an operation's.
//
// What the generated file buys the frontend is not convenience — it is
// agreement. A path the console does not answer does not type-check. A verb
// that does not match the route's kind does not type-check. A parameter on a
// parameterless read does not type-check. Those were three real bugs in the
// hand-written wrapper layer (W5.1), and a compiler is a better guard than a
// review.
//
// The verbs come in one shape each. getJSON carries the verb/kind rule in its
// TYPE: the second argument is the query bag for a route that takes one, and
// the request options for a parameterless read. So a parameter on a
// parameterless read does not type-check, and neither does a route's kind
// disagreeing with the verb.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

// clientHeader is the provenance banner every generated client starts with.
const clientHeader = `// SPDX-License-Identifier: MIT

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
`

// pathGroups are the route kinds, in the order the unions are written.
var pathGroups = []struct {
	kind opapi.HTTPRouteKind
	name string
	doc  string
}{
	{opapi.HTTPRouteRead, "ReadPath", "A parameterless read: one GET, one operation, no arguments."},
	{opapi.HTTPRouteReadArgs, "ReadArgsPath", "A read whose arguments come from the query string."},
	{opapi.HTTPRouteWrite, "WritePath", "A mutation whose arguments come from the query string."},
	{opapi.HTTPRouteJSON, "JsonPath", "A mutation whose arguments come from the JSON body."},
	{opapi.HTTPRouteStream, "StreamPath", "A mutation that streams frames; the console binds a bespoke handler."},
}

// renderClient renders the generated TypeScript client.
//
// routes are the catalogue's routes; native are the console's own endpoints.
func renderClient(routes []opapi.HTTPRoute, native []string) string {
	var out strings.Builder
	out.WriteString(clientHeader)

	byKind := map[opapi.HTTPRouteKind][]string{}
	for _, route := range routes {
		byKind[route.Kind] = append(byKind[route.Kind], route.Path)
	}
	for kind := range byKind {
		sort.Strings(byKind[kind])
	}

	for _, group := range pathGroups {
		out.WriteString("\n/** " + group.doc + " */\n")
		out.WriteString("export type " + group.name + " =\n")
		out.WriteString(union(byKind[group.kind]))
	}

	// The console's own endpoints. They are not operation bindings, so the
	// catalogue says nothing about their shape — but they are paths the
	// console answers, and the browser has to be able to name them.
	nativePaths := make([]string, 0, len(native))
	for _, path := range native {
		if strings.HasPrefix(path, "/api/") {
			nativePaths = append(nativePaths, path)
		}
	}
	sort.Strings(nativePaths)
	out.WriteString("\n/** The console's own endpoints: not operation bindings, but paths it answers. */\n")
	out.WriteString("export type NativePath =\n")
	out.WriteString(union(nativePaths))

	out.WriteString("\n/** Every path the console answers for an operation. */\n")
	out.WriteString("export type ApiPath = ReadPath | ReadArgsPath | WritePath | JsonPath | StreamPath;\n")
	out.WriteString(clientFunctions)
	return out.String()
}

// union renders a TypeScript union of path literals.
func union(paths []string) string {
	if len(paths) == 0 {
		return "  | never;\n"
	}
	var out strings.Builder
	for i, path := range paths {
		suffix := ""
		if i == len(paths)-1 {
			suffix = ";"
		}
		out.WriteString("  | " + quote(path) + suffix + "\n")
	}
	return out.String()
}

// quote renders a path as a TypeScript string literal.
func quote(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

// clientFunctions is the generated client's function surface.
//
// The three verbs are the ones the console binds: a read answers GET, a query
// mutation answers POST with its arguments in the query string, and a body
// mutation answers POST with its arguments in the JSON body. The transport
// concerns — the bearer token, the error shape, the abort composition — live
// in ./transport, which is hand-written because it is not part of the API
// contract.
const clientFunctions = `
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

export async function getJSON<T = any>(
  path: ReadPath,
  options?: RequestOptions,
): Promise<T>;
export async function getJSON<T = any>(
  path: ReadArgsPath,
  params?: Record<string, string>,
  options?: RequestOptions,
): Promise<T>;
export async function getJSON<T = any>(
  path: NativePath,
  params?: Record<string, string>,
  options?: RequestOptions,
): Promise<T>;
export async function getJSON<T = any>(
  path: RuntimePath,
  params?: Record<string, string>,
  options?: RequestOptions,
): Promise<T>;
export async function getJSON<T = any>(
  path: ReadPath | ReadArgsPath | NativePath | RuntimePath,
  paramsOrOptions?: Record<string, string> | RequestOptions,
  options?: RequestOptions,
): Promise<T> {
  const raw = paramsOrOptions as Record<string, string> | RequestOptions | undefined;
  const query = isRequestOptions(raw) ? undefined : raw;
  const callOptions = isRequestOptions(raw) ? raw : options;
  return requestGet<T>(path, query, callOptions);
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

export async function postJSON<T = any>(
  path: JsonPath | NativePath,
  body: unknown,
  options?: { signal?: AbortSignal },
): Promise<T> {
  return postBody<T>(path, body, options);
}

/** postJSONAt is the runtime-path twin of postJSON. */
export async function postJSONAt<T = any>(
  path: JsonPath | NativePath,
  body: unknown,
  options?: { signal?: AbortSignal },
): Promise<T> {
  return postBody<T>(path, body, options);
}

export async function postAction<T = any>(
  path: WritePath | NativePath,
  params?: Record<string, string>,
): Promise<T> {
  return postQuery<T>(path, params);
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
`

// renderOpenAPI renders the API document.
func renderOpenAPI(doc *opapi.OpenAPIDocument) ([]byte, error) {
	raw, err := opapi.MarshalOpenAPI(doc)
	if err != nil {
		return nil, fmt.Errorf("render the API document: %w", err)
	}
	return raw, nil
}
