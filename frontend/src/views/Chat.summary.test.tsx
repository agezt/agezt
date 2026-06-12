// @vitest-environment jsdom
// History compaction surface (M925): the fold-point divider in the thread, and
// the conversation-store helpers that persist the briefing.
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { SummaryDivider } from "@/views/Chat";
import { activeSummary, withActiveSummary, newConversation, type Store } from "@/lib/conversations";

afterEach(cleanup);

describe("SummaryDivider", () => {
  it("labels how many messages were folded and reveals the briefing on click", () => {
    render(<SummaryDivider summary={{ text: "Owner plans an Oslo trip; budget 2000 EUR.", upto: 19 }} />);
    expect(screen.getByText("19 older messages summarized for the agent")).toBeTruthy();
    expect(screen.queryByText(/Oslo trip/)).toBeNull();
    fireEvent.click(screen.getByText("19 older messages summarized for the agent"));
    expect(screen.getByText(/Oslo trip; budget 2000 EUR/)).toBeTruthy();
  });
});

describe("conversation summary helpers", () => {
  function store(): Store {
    return { conversations: [{ ...newConversation("c1", 1) }], activeId: "c1" };
  }

  it("round-trips the active conversation's briefing", () => {
    let s = store();
    expect(activeSummary(s)).toBeUndefined();
    s = withActiveSummary(s, { text: "briefing", upto: 19 }, 2);
    expect(activeSummary(s)).toEqual({ text: "briefing", upto: 19 });
    expect(s.conversations[0].updatedAt).toBe(2);
    s = withActiveSummary(s, undefined, 3);
    expect(activeSummary(s)).toBeUndefined();
  });

  it("only touches the active conversation", () => {
    let s: Store = {
      conversations: [newConversation("c1", 1), newConversation("c2", 1)],
      activeId: "c1",
    };
    s = withActiveSummary(s, { text: "x", upto: 4 }, 2);
    expect(s.conversations[1].summary).toBeUndefined();
  });
});
