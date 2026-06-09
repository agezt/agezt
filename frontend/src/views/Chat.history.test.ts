// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { buildHistory, newTurn, type ChatTurn } from "@/lib/chat";
import type { Msg } from "@/lib/conversations";

// A done assistant turn carrying answer text (turnText reads `answer` when done).
function botTurn(answer: string): ChatTurn {
  return { ...newTurn(), status: "done", answer };
}

describe("buildHistory", () => {
  it("maps user and assistant messages to history turns in order", () => {
    const msgs: Msg[] = [
      { role: "user", text: "hi" },
      { role: "assistant", turn: botTurn("hello there") },
      { role: "user", text: "thanks" },
    ];
    expect(buildHistory(msgs)).toEqual([
      { role: "user", text: "hi" },
      { role: "assistant", text: "hello there" },
      { role: "user", text: "thanks" },
    ]);
  });

  it("drops turns with empty/whitespace text", () => {
    const msgs: Msg[] = [
      { role: "user", text: "  " },
      { role: "assistant", turn: botTurn("") },
      { role: "user", text: "real" },
    ];
    expect(buildHistory(msgs)).toEqual([{ role: "user", text: "real" }]);
  });

  it("returns an empty array for an empty thread", () => {
    expect(buildHistory([])).toEqual([]);
  });
});
