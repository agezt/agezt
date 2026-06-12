// @vitest-environment jsdom
// chat.ts pulls in api.ts (streamRun → withToken), which reads location at module
// load — jsdom provides it. The fold/parse logic under test is pure regardless.
import { describe, it, expect } from "vitest";
import {
  foldChatFrame,
  newTurn,
  turnText,
  parseSSEChunk,
  contextTokensUsed,
  buildHistory,
  buildHistoryWithSummary,
  summaryFoldRange,
  HISTORY_SUMMARY_KEEP,
  type ChatFrame,
} from "@/lib/chat";

// A realistic streaming arc: provider streams tokens, calls one governed tool,
// then the run finishes with a terminal `done` envelope.
const arc: ChatFrame[] = [
  { kind: "open" },
  { kind: "task.received", payload: { intent: "turn off the light" } },
  { kind: "llm.request", payload: { iter: 0, model: "demo" } },
  { kind: "llm.token", payload: { iter: 0, text: "Turning " } },
  { kind: "llm.token", payload: { iter: 0, text: "it off…" } },
  { kind: "policy.decision", payload: { call_id: "c1", tool: "homeassistant", capability: "homeassistant.call", allow: true } },
  { kind: "tool.invoked", payload: { call_id: "c1", tool: "homeassistant", input: { entity: "light.living_room", action: "turn_off" } } },
  { kind: "tool.result", payload: { call_id: "c1", tool: "homeassistant", output: "light.turn_off ok", error: false } },
  { kind: "budget.consumed", payload: { model: "demo", cost_microcents: 4200 } },
  { kind: "task.completed", payload: { answer: "Done — the light is off." } },
  { kind: "done", result: { answer: "Done — the light is off.", model: "demo", iters: 1, spent_mc: 4200, correlation_id: "run-7" } },
];

function fold(frames: ChatFrame[]) {
  return frames.reduce(foldChatFrame, newTurn());
}

describe("foldChatFrame", () => {
  it("accumulates streaming tokens while the run is live", () => {
    const live = fold(arc.slice(0, 5)); // through the two llm.token frames
    expect(live.status).toBe("streaming");
    expect(live.streamedText).toBe("Turning it off…");
    expect(turnText(live)).toBe("Turning it off…"); // live: show streamed text
  });

  it("assembles tool calls by call_id, result winning over invoked", () => {
    const t = fold(arc);
    expect(t.tools).toHaveLength(1);
    const c = t.tools[0];
    expect(c.tool).toBe("homeassistant");
    expect(c.capability).toBe("homeassistant.call");
    expect(c.allow).toBe(true);
    expect(c.error).toBe(false);
    expect(c.output).toBe("light.turn_off ok");
    // The tool's arguments (object payload) are folded as stringified JSON.
    expect(c.input).toBe('{"entity":"light.living_room","action":"turn_off"}');
  });

  it("records model-chain fallback hops, ignoring provider-level fallbacks", () => {
    const t = fold([
      { kind: "open" },
      { kind: "llm.request", payload: { iter: 0, model: "deepseek-chat" } },
      // provider→provider fallback: infra noise, must NOT show in chat.
      { kind: "provider.fallback", payload: { failed: "deepseek", next: "openrouter", reason: "503" } },
      // model→model fallback: the chain moved to the next model.
      { kind: "provider.fallback", payload: { scope: "model-chain", failed_model: "deepseek-chat", next_model: "gpt-4o", reason: "503" } },
      { kind: "done", result: { answer: "ok", model: "gpt-4o", iters: 1, spent_mc: 10 } },
    ]);
    expect(t.fallbacks).toEqual([{ from: "deepseek-chat", to: "gpt-4o" }]);
    expect(t.model).toBe("gpt-4o"); // the model that ultimately answered
  });

  it("captures the named agent the run executed as (M789)", () => {
    const t = fold([{ kind: "done", result: { answer: "ok", agent: "researcher" } }]);
    expect(t.agent).toBe("researcher");
    const plain = fold([{ kind: "done", result: { answer: "ok" } }]);
    expect(plain.agent).toBeUndefined();
  });

  it("chains multiple model-chain fallback hops in order", () => {
    const t = fold([
      { kind: "provider.fallback", payload: { scope: "model-chain", failed_model: "a", next_model: "b" } },
      { kind: "provider.fallback", payload: { scope: "model-chain", failed_model: "b", next_model: "c" } },
    ]);
    expect(t.fallbacks).toEqual([
      { from: "a", to: "b" },
      { from: "b", to: "c" },
    ]);
  });

  it("leaves fallbacks undefined when none fired", () => {
    expect(fold(arc).fallbacks).toBeUndefined();
  });

  it("keeps a pre-stringified tool input as-is", () => {
    const t = fold([
      { kind: "open" },
      { kind: "tool.invoked", payload: { call_id: "c2", tool: "shell", input: '{"cmd":"ls"}' } },
    ]);
    expect(t.tools[0].input).toBe('{"cmd":"ls"}');
  });

  it("records a chronological timeline of text runs and tool calls", () => {
    const tl = fold(arc).timeline ?? [];
    // The model said "Turning it off…" and THEN called the tool — that order is
    // preserved: one text run, then the tool ref.
    expect(tl.map((it) => it.kind)).toEqual(["text", "tool"]);
    const txt = tl[0];
    expect(txt.kind === "text" && txt.text).toBe("Turning it off…");
    const t1 = tl[1];
    expect(t1.kind === "tool" && t1.callId).toBe("c1");
  });

  it("interleaves text before and after a tool call in order", () => {
    const t = fold([
      { kind: "open" },
      { kind: "llm.token", payload: { text: "Let me check. " } },
      { kind: "tool.invoked", payload: { call_id: "x", tool: "shell", input: "{}" } },
      { kind: "tool.result", payload: { call_id: "x", tool: "shell", output: "ok" } },
      { kind: "llm.token", payload: { text: "Done." } },
      { kind: "done", result: { answer: "Done." } },
    ]);
    expect(t.timeline).toEqual([
      { kind: "text", text: "Let me check. " },
      { kind: "tool", callId: "x" },
      { kind: "text", text: "Done." },
    ]);
  });

  it("adds the final answer as a closing text run when no tokens streamed", () => {
    const t = fold([
      { kind: "open" },
      { kind: "tool.invoked", payload: { call_id: "y", tool: "shell", input: "{}" } },
      { kind: "tool.result", payload: { call_id: "y", tool: "shell", output: "done" } },
      { kind: "task.completed", payload: { answer: "All set." } },
      { kind: "done", result: { answer: "All set." } },
    ]);
    expect(t.timeline).toEqual([
      { kind: "tool", callId: "y" },
      { kind: "text", text: "All set." },
    ]);
  });

  it("prefers the authoritative answer over intermediate text once done", () => {
    const t = fold(arc);
    expect(t.status).toBe("done");
    expect(t.answer).toBe("Done — the light is off.");
    expect(turnText(t)).toBe("Done — the light is off.");
    expect(t.model).toBe("demo");
    expect(t.iters).toBe(1);
    expect(t.costMicrocents).toBe(4200);
    expect(t.correlationId).toBe("run-7");
  });

  it("falls back to the final answer when a provider streams no tokens", () => {
    const noStream: ChatFrame[] = [
      { kind: "open" },
      { kind: "task.completed", payload: { answer: "[echo]\nhello" } },
      { kind: "done", result: { answer: "[echo]\nhello", iters: 1 } },
    ];
    const t = fold(noStream);
    expect(t.streamedText).toBe("");
    expect(turnText(t)).toBe("[echo]\nhello");
  });

  it("accumulates a reasoning model's chain of thought separately from the answer", () => {
    const t = fold([
      { kind: "open" },
      { kind: "llm.reasoning", payload: { text: "Let me think… " } },
      { kind: "llm.reasoning", payload: { text: "the number was 42." } },
      { kind: "llm.token", payload: { text: "It's 42." } },
      { kind: "task.completed", payload: { answer: "It's 42." } },
      { kind: "done", result: { answer: "It's 42." } },
    ]);
    expect(t.reasoning).toBe("Let me think… the number was 42.");
    expect(turnText(t)).toBe("It's 42."); // reasoning is NOT the answer
  });

  it("marks the turn errored when the loop fails mid-stream", () => {
    const t = fold([{ kind: "open" }, { kind: "error", error: "budget exhausted" }]);
    expect(t.status).toBe("error");
    expect(t.error).toBe("budget exhausted");
  });

  it("does not mutate the previous turn (React state safety)", () => {
    const a = newTurn();
    const b = foldChatFrame(a, { kind: "llm.token", payload: { text: "hi" } });
    expect(a.streamedText).toBe("");
    expect(b.streamedText).toBe("hi");
    expect(b).not.toBe(a);
  });

  it("folds context size + role split from llm.request (latest wins)", () => {
    const t = fold([
      { kind: "llm.request", payload: { iter: 0, context_chars: 1000, context_by_role: { system: 600, user: 400 } } },
      {
        kind: "llm.request",
        payload: { iter: 1, context_chars: 5000, context_by_role: { system: 600, user: 400, assistant: 1000, tool: 3000 } },
      },
    ]);
    expect(t.context?.chars).toBe(5000);
    expect(t.context?.byRole).toEqual({ system: 600, user: 400, assistant: 1000, tool: 3000 });
  });

  it("accumulates provider usage across iterations and keeps the last prompt size", () => {
    const t = fold([
      { kind: "llm.response", payload: { iter: 0, usage: { input_tokens: 100, output_tokens: 20, cached_input_tokens: 50 } } },
      {
        kind: "llm.response",
        payload: { iter: 1, usage: { input_tokens: 300, output_tokens: 40, cached_input_tokens: 90, cache_write_input_tokens: 10 } },
      },
    ]);
    expect(t.context?.inputTokens).toBe(400);
    expect(t.context?.outputTokens).toBe(60);
    expect(t.context?.cachedTokens).toBe(140);
    expect(t.context?.cacheWriteTokens).toBe(10);
    expect(t.context?.lastInputTokens).toBe(300); // the run's final context, in real tokens
  });

  it("records each context.compacted event", () => {
    const t = fold([
      {
        kind: "context.compacted",
        payload: { elided: 2, reclaimed_chars: 8000, context_chars_before: 45000, context_chars_after: 37000 },
      },
    ]);
    expect(t.context?.compactions).toEqual([
      { elided: 2, reclaimedChars: 8000, beforeChars: 45000, afterChars: 37000 },
    ]);
  });

  it("does not mutate the previous turn's context accounting", () => {
    const a = fold([{ kind: "llm.response", payload: { iter: 0, usage: { input_tokens: 100 } } }]);
    const b = foldChatFrame(a, { kind: "llm.response", payload: { iter: 1, usage: { input_tokens: 200 } } });
    expect(a.context?.inputTokens).toBe(100);
    expect(b.context?.inputTokens).toBe(300);
    expect(b.context).not.toBe(a.context);
  });

  it("leaves context absent when the run never reported any (old turns stay clean)", () => {
    const t = fold([{ kind: "llm.token", payload: { text: "hi" } }, { kind: "done", result: { answer: "hi" } }]);
    expect(t.context).toBeUndefined();
  });
});

describe("history summary folding (M925)", () => {
  const user = (text: string) => ({ role: "user", text }) as import("@/lib/conversations").Msg;
  const bot = (text: string) =>
    ({ role: "assistant", turn: { ...newTurn(), status: "done", answer: text } }) as import("@/lib/conversations").Msg;
  const thread = (n: number) =>
    Array.from({ length: n }, (_, i) => (i % 2 === 0 ? user(`q${i}`) : bot(`a${i}`)));

  it("summaryFoldRange stays quiet until the unsummarized tail outgrows the trigger", () => {
    expect(summaryFoldRange(30, 0)).toBeNull();
    expect(summaryFoldRange(31, 0)).toEqual({ from: 0, to: 31 - HISTORY_SUMMARY_KEEP });
    // After a fold at 19, quiet again until 30 more pile up past it.
    expect(summaryFoldRange(48, 19)).toBeNull();
    expect(summaryFoldRange(50, 19)).toEqual({ from: 19, to: 50 - HISTORY_SUMMARY_KEEP });
  });

  it("buildHistoryWithSummary leads with the briefing and sends only the tail verbatim", () => {
    const msgs = thread(40);
    const h = buildHistoryWithSummary(msgs, { text: "the briefing", upto: 28 });
    expect(h[0].role).toBe("system");
    expect(h[0].text).toContain("the briefing");
    expect(h).toHaveLength(1 + 12); // briefing + the 12 kept messages
    expect(h[1].text).toBe("q28");
  });

  it("falls back to the plain history when the briefing no longer fits the thread", () => {
    const msgs = thread(10);
    // upto beyond the (sliced) thread — e.g. an edit cut back past the fold.
    expect(buildHistoryWithSummary(msgs, { text: "stale", upto: 28 })).toEqual(buildHistory(msgs));
    expect(buildHistoryWithSummary(msgs, undefined)).toEqual(buildHistory(msgs));
    expect(buildHistoryWithSummary(msgs, { text: "  ", upto: 4 })).toEqual(buildHistory(msgs));
  });
});

describe("contextTokensUsed", () => {
  it("prefers the provider's real prompt tokens", () => {
    const t = fold([
      { kind: "llm.request", payload: { iter: 0, context_chars: 8000 } },
      { kind: "llm.response", payload: { iter: 0, usage: { input_tokens: 1500 } } },
    ]);
    expect(contextTokensUsed(t.context!)).toBe(1500);
  });

  it("falls back to a chars/4 estimate when usage is unreported", () => {
    const t = fold([{ kind: "llm.request", payload: { iter: 0, context_chars: 8000 } }]);
    expect(contextTokensUsed(t.context!)).toBe(2000);
  });
});

describe("parseSSEChunk", () => {
  it("extracts complete data frames and keeps the partial tail", () => {
    const chunk =
      'data: {"kind":"open"}\n\n' +
      'data: {"kind":"llm.token","payload":{"text":"hi"}}\n\n' +
      'data: {"kind":"do'; // partial — must be retained
    const { frames, rest } = parseSSEChunk(chunk);
    expect(frames.map((f) => f.kind)).toEqual(["open", "llm.token"]);
    expect(rest).toBe('data: {"kind":"do');
  });

  it("skips malformed frames without aborting the stream", () => {
    const { frames } = parseSSEChunk('data: not-json\n\ndata: {"kind":"done"}\n\n');
    expect(frames.map((f) => f.kind)).toEqual(["done"]);
  });
});
