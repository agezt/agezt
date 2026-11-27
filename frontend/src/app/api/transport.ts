// The console's transport concerns — the bearer token, the error shape and the
// event feed's URL.
//
// These are NOT part of the API contract: they are how this browser talks to
// the console at all, whatever it asks for. The API contract itself — which
// paths exist, which verb each takes, what a body may carry — lives in
// ./client.ts, which is GENERATED from the daemon's operation registry and the
// console's route catalogue (W5.1). Nothing here is hand-written twice: the
// client imports the three helpers below and adds the paths.
//
// The daemon prints the console URL with a ?token= on boot; the SPA reads it
// once and keeps it in memory only (never localStorage). Fetch calls send it as
// Authorization (the main console bearer token never appears in a URL query
// string after the VULN query-string-token fix). The EventSource URL for /events
// uses a separate ephemeral SSE token fetched at startup from /api/sse-token,
// because EventSource cannot set custom headers.
// Guard `location` so importing this module never throws under a node test
// environment (pure-logic specs that transitively import it); in the browser
// this is always defined.
// readAndScrubToken takes the token off the address bar the moment it is read.
// It stays in memory; leaving it in the URL puts a live console credential into
// browser history, the window title, bookmarks, and the Referer of anything the
// console ever links out to. replaceState rewrites the current entry rather than
// pushing one, so Back still behaves and a reload no longer re-reads it.
import type { ResponseOf } from "@/app/api/types.generated";

// The console's SSE-token answer is the console's OWN row, and the browser
// names none of its fields: the row is declared in the contract
// (kernel/contract/consoleapi) and the catalogue is generated from that
// declaration. The alias binds the PATH to the row, so the browser asks the
// catalogue what /api/sse-token answers with instead of guessing a shape.
type SSETokenRow = ResponseOf<"/api/sse-token">;

export function readAndScrubToken(): string {
  if (typeof location === "undefined") return "";
  const params = new URLSearchParams(location.search);
  const token = params.get("token") || "";
  if (token && typeof history !== "undefined" && typeof history.replaceState === "function") {
    params.delete("token");
    const qs = params.toString();
    history.replaceState(null, "", location.pathname + (qs ? "?" + qs : "") + location.hash);
  }
  return token;
}

const TOKEN = readAndScrubToken();

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
        if (!res.ok) throw new Error("/api/sse-token returned " + res.status);
        // The console's SSE-token answer is declared in the contract
        // (kernel/contract/consoleapi) and the browser's catalogue is generated
        // from that declaration — so the body is the row, not a shape this
        // file guesses.
        const body = (await res.json()) as SSETokenRow;
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
  return getSSEToken().then((st) => "/events?st=" + encodeURIComponent(st));
}

export function authHeaders(headers?: HeadersInit): Headers {
  const h = new Headers(headers);
  if (TOKEN) h.set("Authorization", "Bearer " + TOKEN);
  return h;
}

/** errMsg reads the console's error envelope, falling back to the status line. */
export async function errMsg(res: Response): Promise<string> {
  try {
    const j = await res.json();
    if (j?.error) return String(j.error);
  } catch {
    /* fall through */
  }
  return "HTTP " + res.status;
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
