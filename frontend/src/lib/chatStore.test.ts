// @vitest-environment jsdom
// chatStore pulls in api.ts (reads location at module load) — jsdom provides it.
import { describe, it, expect } from "vitest";
import { collectLearned, type LearnedMem } from "@/lib/chatStore";
import type { AgentEvent } from "@/lib/events";

const written = (corr: string, p: Record<string, unknown>): AgentEvent => ({
  kind: "memory.written",
  correlation_id: corr,
  payload: p,
});

describe("collectLearned", () => {
  it("buckets memory.written events by correlation id", () => {
    let m: Record<string, LearnedMem[]> = {};
    m = collectLearned(m, written("run-1", { id: "a", type: "FACT", subject: "sky is blue", action: "write" }));
    m = collectLearned(m, written("run-1", { id: "b", type: "PREFERENCE", subject: "likes teal", action: "write" }));
    m = collectLearned(m, written("run-2", { id: "c", type: "FACT", subject: "other run", action: "write" }));
    expect(m["run-1"].map((x) => x.id)).toEqual(["a", "b"]);
    expect(m["run-1"][1].type).toBe("PREFERENCE");
    expect(m["run-2"]).toHaveLength(1);
  });

  it("ignores non-memory events and id-less notes", () => {
    let m: Record<string, LearnedMem[]> = {};
    m = collectLearned(m, { kind: "llm.token", correlation_id: "run-1", payload: { text: "hi" } });
    m = collectLearned(m, written("run-1", { action: "distill_failed", error: "boom" })); // no id
    m = collectLearned(m, { kind: "memory.written", payload: { id: "x" } }); // no correlation
    expect(m).toEqual({});
  });

  it("dedupes a repeated id within the same run", () => {
    let m: Record<string, LearnedMem[]> = {};
    const ev = written("run-1", { id: "a", type: "FACT", subject: "dup", action: "write" });
    m = collectLearned(m, ev);
    m = collectLearned(m, ev);
    expect(m["run-1"]).toHaveLength(1);
  });

  it("defaults missing fields", () => {
    const m = collectLearned({}, written("r", { id: "a" }));
    expect(m["r"][0]).toEqual({ id: "a", type: "FACT", subject: "", action: "write" });
  });
});
