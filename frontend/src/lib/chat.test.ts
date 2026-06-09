// @vitest-environment jsdom
// chat.ts pulls in api.ts (streamRun → withToken), which reads location at module
// load — jsdom provides it. The fold/parse logic under test is pure regardless.
import { describe, it, expect } from "vitest";
import { foldChatFrame, newTurn, turnText, parseSSEChunk, type ChatFrame } from "@/lib/chat";

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
