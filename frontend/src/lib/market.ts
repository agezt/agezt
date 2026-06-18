import { withToken } from "@/lib/api";
import { parseSSEChunk, type ChatFrame } from "@/lib/chat";

// One marketplace install/uninstall progress step (kernel/market.Event).
export interface MarketStep {
  stage: string; // skill | mcp | tool | done
  name?: string;
  ok: boolean;
  detail?: string;
}

// streamMarket POSTs to a market install/uninstall endpoint and yields each SSE
// frame (open → per-item progress → done/error), reusing the shared SSE parser.
// Mirrors lib/toolbox.streamInstall.
export async function streamMarket(
  path: "/api/market/install" | "/api/market/uninstall",
  body: Record<string, unknown>,
  onFrame: (f: ChatFrame) => void,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch(withToken(path), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    signal,
  });
  if (!res.ok || !res.body) {
    let msg = `HTTP ${res.status}`;
    try {
      const j = await res.json();
      if (j?.error) msg = String(j.error);
    } catch {
      /* no JSON body */
    }
    throw new Error(msg);
  }
  const reader = res.body.getReader();
  const dec = new TextDecoder();
  let buf = "";
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buf += dec.decode(value, { stream: true });
    const { frames, rest } = parseSSEChunk(buf);
    buf = rest;
    for (const f of frames) onFrame(f);
  }
}

// stepFromFrame extracts a MarketStep from a progress frame's payload, or null
// for non-progress frames (open/done/error).
export function stepFromFrame(f: ChatFrame): MarketStep | null {
  if (f.kind !== "market.install.progress" && f.kind !== "market.uninstall.progress") return null;
  const p = f.payload || {};
  return {
    stage: String(p.stage ?? ""),
    name: p.name ? String(p.name) : undefined,
    ok: Boolean(p.ok),
    detail: p.detail ? String(p.detail) : undefined,
  };
}
