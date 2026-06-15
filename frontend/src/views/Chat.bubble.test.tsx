// @vitest-environment jsdom
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { AssistantBubble } from "@/views/Chat";
import { newTurn } from "@/lib/chat";

afterEach(cleanup);

function streamingTurn(text: string) {
  return { ...newTurn(), status: "streaming" as const, streamedText: text };
}

describe("AssistantBubble avatar", () => {
  it("wears the conversation agent's gradient monogram when an agent is set", () => {
    render(<AssistantBubble turn={streamingTurn("thinking…")} agent="researcher" />);
    // AgentAvatar renders the slug's initials (RESEARCHER → "RE").
    expect(screen.getByText("RE")).toBeTruthy();
    expect(screen.getByText("thinking…")).toBeTruthy();
  });

  it("falls back to the house assistant mark when no agent is set", () => {
    render(<AssistantBubble turn={streamingTurn("thinking…")} />);
    // No agent monogram — the default spark avatar carries no initials text.
    expect(screen.queryByText("RE")).toBeNull();
    expect(screen.getByText("thinking…")).toBeTruthy();
  });
});
