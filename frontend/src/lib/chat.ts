import { num } from "@/lib/rundetail";
import { withToken } from "@/lib/api";

// A frame is one SSE `data:` object streamed by the webui /api/run proxy. Most
// frames carry a forwarded agent event ({kind, payload, ...}); the proxy also
// emits three synthetic envelope frames: `open` (stream established), `error`
// (the loop failed mid-flight, after headers were flushed) and `done` (the
// terminal result of the governed run).
export interface ChatFrame {
  kind: string;
  subject?: string;
  payload?: any;
  correlation_id?: string;
  error?: string;
  result?: any;
}

// One tool call assembled across the policy.decision / tool.invoked /
// tool.result frames that share a call_id — the chat renders these as inline
// chips so the human can see what the agent actually did.
export interface ChatTool {
  callId: string;
  tool: string;
  capability?: string;
  allow?: boolean;
  hardDenied?: boolean;
  error?: boolean;
  output?: string;
}

// ChatTurn is the assistant's evolving response to one user intent. The Chat
// view folds frames into it live, so streaming tokens, tool chips and the final
// answer all render as they arrive.
export interface ChatTurn {
  status: "streaming" | "done" | "error";
  streamedText: string; // accumulated llm.token deltas (real providers)
  reasoning: string; // accumulated llm.reasoning deltas (a reasoning model's chain of thought)
  answer?: string; // authoritative final answer (task.completed / done)
  tools: ChatTool[];
  model?: string;
  iters: number;
  costMicrocents: number;
  error?: string;
  correlationId?: string;
}

export function newTurn(): ChatTurn {
  return { status: "streaming", streamedText: "", reasoning: "", tools: [], iters: 0, costMicrocents: 0 };
}

// foldChatFrame is the pure reducer at the heart of the Chat view: (turn, frame)
// → new turn. It never mutates its input (React state), cloning the tool list so
// a render sees a fresh object. Field names mirror deriveDetail's fold so the
// chat and the Runs detail agree on how an event arc reads.
export function foldChatFrame(prev: ChatTurn, f: ChatFrame): ChatTurn {
  const t: ChatTurn = { ...prev, tools: prev.tools.map((c) => ({ ...c })) };
  const p = f.payload || {};
  const tool = (id: string): ChatTool => {
    let c = t.tools.find((x) => x.callId === id);
    if (!c) {
      c = { callId: id, tool: "" };
      t.tools.push(c);
    }
    return c;
  };

  switch (f.kind) {
    case "open":
      break;
    case "llm.token":
      if (p.text != null) t.streamedText += String(p.text);
      break;
    case "llm.reasoning":
      // A reasoning model (deepseek-reasoner/-v4, o-series, Claude thinking)
      // streams its chain of thought separately from the answer content.
      if (p.text != null) t.reasoning += String(p.text);
      break;
    case "llm.request":
    case "llm.response":
      if (p.model) t.model = String(p.model);
      t.iters = Math.max(t.iters, num(p.iter) + 1);
      break;
    case "budget.consumed":
      t.costMicrocents += num(p.cost_microcents);
      if (p.model && !t.model) t.model = String(p.model);
      break;
    case "policy.decision": {
      const c = tool(String(p.call_id || ""));
      if (p.tool) c.tool = String(p.tool);
      if (p.capability) c.capability = String(p.capability);
      c.allow = !!p.allow;
      c.hardDenied = !!p.hard_denied;
      break;
    }
    case "tool.invoked": {
      const c = tool(String(p.call_id || ""));
      if (p.tool) c.tool = String(p.tool);
      break;
    }
    case "tool.result": {
      const c = tool(String(p.call_id || ""));
      if (p.tool) c.tool = String(p.tool);
      c.error = !!p.error;
      if (p.output != null) c.output = String(p.output);
      break;
    }
    case "task.completed":
      if (p.answer != null) t.answer = String(p.answer);
      break;
    case "task.failed":
      t.status = "error";
      if (p.error != null) t.error = String(p.error);
      break;
    case "error":
      t.status = "error";
      if (f.error) t.error = String(f.error);
      break;
    case "done": {
      const r = f.result || {};
      if (r.answer != null) t.answer = String(r.answer);
      if (r.model) t.model = String(r.model);
      if (r.iters != null) t.iters = Math.max(t.iters, num(r.iters));
      if (r.spent_mc != null) t.costMicrocents = num(r.spent_mc);
      if (r.correlation_id) t.correlationId = String(r.correlation_id);
      if (t.status !== "error") t.status = "done";
      break;
    }
  }

  // Drop the synthetic empty-id bucket if no real tool ever landed in it.
  t.tools = t.tools.filter((c) => c.callId !== "" || c.tool !== "");
  return t;
}

// turnText is what the bubble renders: while streaming, the live token text (or
// the answer if the provider didn't stream tokens); once done, the authoritative
// final answer wins over any intermediate "thinking" text.
export function turnText(t: ChatTurn): string {
  if (t.status === "done" && t.answer) return t.answer;
  return t.streamedText || t.answer || "";
}

// parseSSEChunk pulls complete `data:` frames out of a rolling buffer, returning
// the parsed frames and whatever partial tail remains for the next read. Pure,
// so the SSE framing is unit-testable without a network.
export function parseSSEChunk(buffer: string): { frames: ChatFrame[]; rest: string } {
  const frames: ChatFrame[] = [];
  let buf = buffer;
  let idx: number;
  while ((idx = buf.indexOf("\n\n")) >= 0) {
    const block = buf.slice(0, idx);
    buf = buf.slice(idx + 2);
    for (const line of block.split("\n")) {
      if (!line.startsWith("data:")) continue;
      const json = line.slice(5).trim();
      if (!json) continue;
      try {
        frames.push(JSON.parse(json) as ChatFrame);
      } catch {
        /* skip a malformed frame rather than abort the stream */
      }
    }
  }
  return { frames, rest: buf };
}

// streamRun is the Chat send button: it POSTs the intent to the webui /api/run
// proxy and folds the SSE response, invoking onFrame for each frame as it lands.
// Rejects (before any frame) on a non-stream error response so the composer can
// surface it; honors `signal` so a navigation/abort tears the request down.
// One prior turn sent back to the daemon for multi-turn continuity. The server
// folds these (with the new intent) into a transcript intent — the same convo
// mapping the OpenAI API uses.
export interface ChatHistoryTurn {
  role: "user" | "assistant";
  text: string;
}

export async function streamRun(
  body: { intent: string; model?: string; history?: ChatHistoryTurn[] },
  onFrame: (f: ChatFrame) => void,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch(withToken("/api/run"), {
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
