import { describe, it, expect } from "vitest";
import { createSpeechChunker } from "@/lib/sentenceChunker";

describe("createSpeechChunker", () => {
  it("emits a sentence as soon as its terminator arrives", () => {
    const c = createSpeechChunker();
    expect(c.push("Turning off the lights")).toEqual([]);
    expect(c.push(" now.")).toEqual(["Turning off the lights now."]);
  });

  it("splits multiple sentences in one delta", () => {
    const c = createSpeechChunker();
    expect(c.push("Done. The door is locked! Anything else?")).toEqual([
      "Done.",
      "The door is locked!",
      "Anything else?",
    ]);
  });

  it("does not split on abbreviations or initials", () => {
    const c = createSpeechChunker();
    expect(c.push("Dr. Stark will see you. ")).toEqual(["Dr. Stark will see you."]);
  });

  it("does not split inside decimals", () => {
    const c = createSpeechChunker();
    expect(c.push("It costs 3.14 dollars. ")).toEqual(["It costs 3.14 dollars."]);
  });

  it("skips fenced code blocks entirely", () => {
    const c = createSpeechChunker();
    const out = [
      ...c.push("Here is the fix.\n```go\nfunc main() { panic(1) }\n```\n"),
      ...c.push("It is done."),
    ];
    expect(out).toEqual(["Here is the fix.", "It is done."]);
  });

  it("handles a fence whose backticks straddle a chunk boundary", () => {
    const c = createSpeechChunker();
    // The ``` opening and closing each arrive split across two pushes; the code
    // body must still be dropped and the sentences around it spoken intact.
    const out = [...c.push("Look here. `"), ...c.push("``go\nx\n`"), ...c.push("``\nFixed.")];
    expect(out).toEqual(["Look here.", "Fixed."]);
  });

  it("flush returns the trailing fragment and clears state", () => {
    const c = createSpeechChunker();
    c.push("First. ");
    expect(c.push("A trailing thought")).toEqual([]);
    expect(c.flush()).toBe("A trailing thought");
    expect(c.flush()).toBeNull();
  });

  it("holds very short fragments when a minLen is set", () => {
    const c = createSpeechChunker(3);
    expect(c.push("Ok. ")).toEqual([]); // "Ok." is below minLen, held
    expect(c.push("Now we proceed.")).toEqual(["Ok. Now we proceed."]);
  });
});
