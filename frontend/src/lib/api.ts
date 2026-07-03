// The daemon prints the console URL with a ?token= on boot; the SPA reads it
// once and keeps it in memory only (never localStorage). Fetch calls send it as
// Authorization (the main console bearer token never appears in a URL query
// string after the VULN query-string-token fix). The EventSource URL for /events
// uses a separate ephemeral SSE token fetched at startup from /api/sse-token,
// because EventSource cannot set custom headers.
// Guard `location` so importing this module never throws under a node test
// environment (pure-logic specs that transitively import it); in the browser
// this is always defined.
const TOKEN =
  typeof location !== "undefined" ? new URLSearchParams(location.search).get("token") || "" : "";

// SSE_TOKEN is the ephemeral SSE-only token for the /events EventSource URL.
// Fetched once at module load from /api/sse-token (which requires the main
// bearer token in the Authorization header). Initialized to a promise so the
// EventsProvider can await it in its useEffect. Falls back to the main token
// if the fetch fails (backward compatibility).
let sseTokenPromise: Promise<string> | null = null;

function getSSEToken(): Promise<string> {
  if (!sseTokenPromise) {
    sseTokenPromise = (async () => {
      try {
        const res = await fetch("/api/sse-token", { headers: authHeaders() });
        if (!res.ok) throw new Error(`/api/sse-token returned ${res.status}`);
        const body = (await res.json()) as { token?: string };
        if (!body.token) throw new Error("empty sse token");
        return body.token;
      } catch (err) {
        // Fallback: use main console token. If that's also empty (no ?token=
        // in URL — e.g. password login), the EventSource URL won't carry a
        // token and the backend falls back to session-cookie auth.
        sseTokenPromise = null; // allow retry (e.g. after login)
        return TOKEN;
      }
    })();
  }
  return sseTokenPromise;
}

// eventsURLAsync returns a promise that resolves to the /events URL with the
// ephemeral SSE token. The EventsProvider awaits this before opening the
// EventSource connection.
export function eventsURLAsync(): Promise<string> {
  return getSSEToken().then((st) => `/events?st=${encodeURIComponent(st)}`);
}

export function authHeaders(headers?: HeadersInit): Headers {
  const h = new Headers(headers);
  if (TOKEN) h.set("Authorization", `Bearer ${TOKEN}`);
  return h;
}

async function errMsg(res: Response): Promise<string> {
  try {
    const j = await res.json();
    if (j?.error) return String(j.error);
  } catch {
    /* fall through */
  }
  return `HTTP ${res.status}`;
}

/**
 * HTTPError is the typed wrapper around the throw sites in this module. Callers
 * that need to branch on a status code (e.g. "the route exists in the future,
 * render a placeholder") can do so without parsing the human message string.
 *
 * Plain `Error` is still the throw contract for everything else (network
 * failures, malformed JSON, etc.); check `err instanceof HTTPError` when the
 * status matters.
 */
export class HTTPError extends Error {
  status: number;
  url: string;
  constructor(status: number, url: string, message: string) {
    super(message);
    this.name = "HTTPError";
    this.status = status;
    this.url = url;
  }
}

export async function getJSON<T = any>(path: string, params?: Record<string, string>): Promise<T> {
  const query = new URLSearchParams(params || {}).toString();
  const url = query ? `${path}?${query}` : path;
  const res = await fetch(url, { headers: authHeaders({ Accept: "application/json" }) });
  if (!res.ok) throw new HTTPError(res.status, url, await errMsg(res));
  return res.json() as Promise<T>;
}

export async function postJSON<T = any>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: "POST",
    headers: authHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify(body ?? {}),
  });
  if (!res.ok) throw new HTTPError(res.status, path, await errMsg(res));
  return res.json() as Promise<T>;
}

export async function postAction<T = any>(path: string, params?: Record<string, string>): Promise<T> {
  const query = new URLSearchParams(params || {}).toString();
  const url = query ? `${path}?${query}` : path;
  const res = await fetch(url, { method: "POST", headers: authHeaders() });
  if (!res.ok) throw new HTTPError(res.status, url, await errMsg(res));
  return res.json() as Promise<T>;
}
