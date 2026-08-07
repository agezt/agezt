import { describe, it, expect } from "vitest";
import { humanizeIntent } from "@/lib/intent";

describe("humanizeIntent", () => {
  it("passes plain intents through", () => {
    expect(humanizeIntent("Reply with exactly: OK")).toBe("Reply with exactly: OK");
    expect(humanizeIntent("Run one system-health sweep.")).toBe("Run one system-health sweep.");
  });

  it("returns empty for missing/blank intents", () => {
    expect(humanizeIntent(undefined)).toBe("");
    expect(humanizeIntent("   ")).toBe("");
  });

  it("extracts the ask from a composed == QUESTION == prompt", () => {
    const raw =
      "You are AGEZT's observability analyst, embedded in a running agent operating system. " +
      "Using ONLY the live system snapshot below, answer the operator's question.\n\n" +
      "== SYSTEM SNAPSHOT ==\nRUNS: total=21 failed=16\n\n" +
      "== QUESTION ==\nSummarize the system's health right now.";
    expect(humanizeIntent(raw)).toBe("Summarize the system's health right now.");
  });

  it("titles a chat transcript by its newest user message", () => {
    const raw =
      "User: What can you do?\nAssistant: I'm AGEZT—Jarvis's more practical cousin.\n" +
      "User: Continue from where you stopped and finish the task.";
    expect(humanizeIntent(raw)).toBe("Continue from where you stopped and finish the task.");
  });

  it("cuts a single-line transcript before the assistant reply", () => {
    const raw = "User: What can you do? Assistant: I'm AGEZT—Jarvis's more practical cousin.";
    expect(humanizeIntent(raw)).toBe("What can you do?");
  });

  it("falls back to the first line and caps very long titles", () => {
    const raw = `${"x".repeat(300)}\nsecond line`;
    const out = humanizeIntent(raw);
    expect(out.endsWith("…")).toBe(true);
    expect(out.length).toBeLessThanOrEqual(201);
  });
});
