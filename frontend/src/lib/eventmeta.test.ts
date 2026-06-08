import { describe, it, expect } from "vitest";
import { categoryOf, isErrorKind, CATEGORIES } from "@/lib/eventmeta";

describe("categoryOf", () => {
  it("maps known prefixes to categories", () => {
    expect(categoryOf("task.completed").key).toBe("task");
    expect(categoryOf("llm.request").key).toBe("llm");
    expect(categoryOf("tool.invoked").key).toBe("tool");
    expect(categoryOf("policy.decision").key).toBe("policy");
    expect(categoryOf("budget.consumed").key).toBe("budget");
    expect(categoryOf("rate.limited").key).toBe("budget");
    expect(categoryOf("run.steered").key).toBe("steer");
    expect(categoryOf("provider.fallback").key).toBe("provider");
    expect(categoryOf("context.compacted").key).toBe("context");
    expect(categoryOf("memory.retrieved").key).toBe("knowledge");
    expect(categoryOf("subagent.spawned").key).toBe("knowledge");
    expect(categoryOf("warden.executed").key).toBe("system");
    expect(categoryOf("kernel.halt").key).toBe("system");
  });

  it("falls back to other for unknown/empty kinds", () => {
    expect(categoryOf("totally.unknown").key).toBe("other");
    expect(categoryOf(undefined).key).toBe("other");
    expect(categoryOf("").key).toBe("other");
  });

  it("every category has a non-empty color", () => {
    for (const c of CATEGORIES) expect(c.color.length).toBeGreaterThan(0);
  });
});

describe("isErrorKind", () => {
  it("flags failures and denials", () => {
    expect(isErrorKind("task.failed")).toBe(true);
    expect(isErrorKind("budget.exceeded")).toBe(true);
    expect(isErrorKind("netguard.blocked")).toBe(true);
    expect(isErrorKind("capability.rejected")).toBe(true);
    expect(isErrorKind("something.denied")).toBe(true);
  });
  it("does not flag normal events", () => {
    expect(isErrorKind("llm.request")).toBe(false);
    expect(isErrorKind("task.completed")).toBe(false);
  });
});
