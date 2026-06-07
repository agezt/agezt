// The daemon prints the console URL with a ?token= on boot; the SPA reads it
// once and rides it on every request. Kept in memory only (never localStorage);
// left in the URL so a refresh re-authorizes. EventSource can't set headers, so
// the token must travel on the query string.
export const TOKEN = new URLSearchParams(location.search).get("token") || "";

export function withToken(path: string, extra?: Record<string, string>): string {
  const u = new URLSearchParams(extra || {});
  if (TOKEN) u.set("token", TOKEN);
  const s = u.toString();
  return s ? `${path}?${s}` : path;
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
  const res = await fetch(withToken(path, params), { headers: { Accept: "application/json" } });
  if (!res.ok) throw new Error(await errMsg(res));
  return res.json() as Promise<T>;
}

export async function postJSON<T = any>(path: string, body: unknown): Promise<T> {
  const res = await fetch(withToken(path), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body ?? {}),
  });
  if (!res.ok) throw new Error(await errMsg(res));
  return res.json() as Promise<T>;
}

export async function postAction<T = any>(path: string, params?: Record<string, string>): Promise<T> {
  const res = await fetch(withToken(path, params), { method: "POST" });
  if (!res.ok) throw new Error(await errMsg(res));
  return res.json() as Promise<T>;
}
