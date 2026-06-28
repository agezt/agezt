// The daemon prints the console URL with a ?token= on boot; the SPA reads it
// once and keeps it in memory only (never localStorage). Fetch calls send it as
// Authorization; EventSource can't set headers, so SSE keeps the query fallback.
// Guard `location` so importing this module never throws under a node test
// environment (pure-logic specs that transitively import it); in the browser
// this is always defined.
const TOKEN =
  typeof location !== "undefined" ? new URLSearchParams(location.search).get("token") || "" : "";

export function withToken(path: string, extra?: Record<string, string>): string {
  const u = new URLSearchParams(extra || {});
  if (TOKEN) u.set("token", TOKEN);
  const s = u.toString();
  return s ? `${path}?${s}` : path;
}

export function authHeaders(headers?: HeadersInit): Headers {
  const h = new Headers(headers);
  if (TOKEN) h.set("Authorization", `Bearer ${TOKEN}`);
  return h;
}

export const eventsURL = withToken("/events");

async function errMsg(res: Response): Promise<string> {
  try {
    const j = await res.json();
    if (j?.error) return String(j.error);
  } catch {
    /* fall through */
  }
  return `HTTP ${res.status}`;
}

export async function getJSON<T = any>(path: string, params?: Record<string, string>): Promise<T> {
  const query = new URLSearchParams(params || {}).toString();
  const res = await fetch(query ? `${path}?${query}` : path, { headers: authHeaders({ Accept: "application/json" }) });
  if (!res.ok) throw new Error(await errMsg(res));
  return res.json() as Promise<T>;
}

export async function postJSON<T = any>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: "POST",
    headers: authHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify(body ?? {}),
  });
  if (!res.ok) throw new Error(await errMsg(res));
  return res.json() as Promise<T>;
}

export async function postAction<T = any>(path: string, params?: Record<string, string>): Promise<T> {
  const query = new URLSearchParams(params || {}).toString();
  const res = await fetch(query ? `${path}?${query}` : path, { method: "POST", headers: authHeaders() });
  if (!res.ok) throw new Error(await errMsg(res));
  return res.json() as Promise<T>;
}
