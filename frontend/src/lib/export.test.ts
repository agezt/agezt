// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { conversationToMarkdown, slugify } from "@/lib/export";
import { newTurn, type ChatTurn } from "@/lib/chat";
import type { Msg } from "@/lib/conversations";

function doneTurn(answer: string, model?: string): ChatTurn {
  return { ...newTurn(), status: "done", answer, model };
}

describe("conversationToMarkdown", () => {
  it("serialises user + agent turns with a heading and the model footnote", () => {
    const msgs: Msg[] = [
      { role: "user", text: "what is 2+2?" },
      { role: "assistant", turn: doneTurn("4", "demo-model") },
    ];
    const md = conversationToMarkdown("Math chat", msgs);
    expect(md).toContain("# Math chat");
    expect(md).toContain("## You\n\nwhat is 2+2?");
    expect(md).toContain("## Agent\n\n4");
    expect(md).toContain("> demo-model");
    expect(md.endsWith("\n")).toBe(true);
  });

  it("marks an empty answer rather than dropping the turn", () => {
    const md = conversationToMarkdown("x", [{ role: "assistant", turn: newTurn() }]);
    expect(md).toContain("_(no answer)_");
  });

  it("defaults the heading when the title is blank", () => {
    expect(conversationToMarkdown("", [])).toContain("# Conversation");
  });
});

describe("slugify", () => {
  it("makes a safe file stem", () => {
    expect(slugify("Q3 Research! (final)")).toBe("q3-research-final");
    expect(slugify("   ")).toBe("conversation");
  });
});
