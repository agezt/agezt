// @vitest-environment jsdom
// Context-window observability (M925): the per-turn gauge chip, the breakdown
// modal, and the in-thread compaction note. The catalog fetch (model → window
// size) is stubbed; everything else renders from the turn's folded context.
import { describe, it, expect, afterEach, vi, beforeAll } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import { ContextChip, ContextModal, CompactionNote, barTone } from "@/views/Chat";
import { newTurn, type ChatTurn, type TurnContext } from "@/lib/chat";

afterEach(cleanup);

beforeAll(() => {
  // /api/catalog: "demo" has a 64K window; anything else is unknown.
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => ({
      ok: true,
      json: async () => ({ providers: [{ id: "p", models: [{ id: "demo", context: 64000 }] }] }),
    })),
  );
});

function ctx(over: Partial<TurnContext> = {}): TurnContext {
  return {
    chars: 8000,
    byRole: { system: 2000, user: 1000, assistant: 1000, tool: 4000 },
    inputTokens: 4000,
    outputTokens: 500,
    cachedTokens: 1000,
    cacheWriteTokens: 0,
    lastInputTokens: 3200,
    compactions: [],
    ...over,
  };
}

function turnWith(over: Partial<ChatTurn> = {}): ChatTurn {
  return { ...newTurn(), status: "done", model: "demo", iters: 2, context: ctx(), ...over };
}

describe("barTone", () => {
  it("maps window usage to the traffic-light tones", () => {
    expect(barTone(10)).toBe("good");
    expect(barTone(50)).toBe("warn");
    expect(barTone(90)).toBe("bad");
  });
});

describe("ContextChip", () => {
  it("shows the window percentage once the catalog resolves", async () => {
    render(<ContextChip turn={turnWith()} />);
    // 3200 real prompt tokens of a 64K window → 5%.
    await waitFor(() => expect(screen.getByText("5%")).toBeTruthy());
  });

  it("falls back to the absolute token count when the model isn't in the catalog", async () => {
    render(<ContextChip turn={turnWith({ model: "not-in-catalog" })} />);
    await waitFor(() => expect(screen.getByText("3.2K tok")).toBeTruthy());
  });

  it("renders nothing for turns with no context accounting (old storage)", () => {
    const { container } = render(<ContextChip turn={turnWith({ context: undefined })} />);
    expect(container.innerHTML).toBe("");
  });

  it("opens the breakdown modal on click", async () => {
    render(<ContextChip turn={turnWith()} />);
    fireEvent.click(screen.getByTitle("Context window usage — click for the breakdown"));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeTruthy());
    expect(screen.getByText("Context window")).toBeTruthy();
  });
});

describe("ContextModal", () => {
  it("shows the headline, the role composition, and the billed tokens", () => {
    render(<ContextModal turn={turnWith()} windowTokens={64000} onClose={() => {}} />);
    expect(screen.getByText("3.2K tokens")).toBeTruthy();
    expect(screen.getByText("of 64K window")).toBeTruthy();
    // Role rows: tool is half the 8000-char context → 50%, ≈1000 tok estimate.
    expect(screen.getByText("tool")).toBeTruthy();
    expect(screen.getByText("≈1.0K tok · 4.0K chars")).toBeTruthy();
    // Billed tokens across both iterations, with the cached share called out.
    expect(screen.getByText("Tokens billed · 2 iterations")).toBeTruthy();
    expect(screen.getByText("4.0K")).toBeTruthy();
    expect(screen.getByText("· 1.0K cached")).toBeTruthy();
  });

  it("says so when no compaction was needed", () => {
    render(<ContextModal turn={turnWith()} windowTokens={64000} onClose={() => {}} />);
    expect(screen.getByText("None needed — the context fit the budget.")).toBeTruthy();
  });

  it("lists each compaction with before → after sizes", () => {
    const t = turnWith({
      context: ctx({
        compactions: [{ elided: 2, reclaimedChars: 8000, beforeChars: 45000, afterChars: 37000, skillRescuedCount: 1, skillRescuedChars: 2200 }],
      }),
    });
    render(<ContextModal turn={t} windowTokens={64000} onClose={() => {}} />);
    expect(screen.getByText(/2 tool outputs elided · reclaimed 8\.0K\s+chars/)).toBeTruthy();
    expect(screen.getByText(/\(45K → 37K\)/)).toBeTruthy();
    expect(screen.getByText("1 skill resource kept · 2.2K chars")).toBeTruthy();
  });

  it("escape closes it", () => {
    const onClose = vi.fn();
    render(<ContextModal turn={turnWith()} windowTokens={64000} onClose={onClose} />);
    fireEvent.keyDown(window, { key: "Escape" });
    expect(onClose).toHaveBeenCalled();
  });
});

describe("CompactionNote", () => {
  it("sums elisions and reclaimed chars across events", () => {
    render(
      <CompactionNote
        events={[
          { elided: 2, reclaimedChars: 8000, beforeChars: 45000, afterChars: 37000 },
          { elided: 1, reclaimedChars: 2000, beforeChars: 39000, afterChars: 37000 },
        ]}
      />,
    );
    expect(screen.getByText("context compacted")).toBeTruthy();
    expect(screen.getByText("3 old tool outputs elided · 10K chars reclaimed")).toBeTruthy();
  });

  it("shows rescued skill resources when compaction preserved them", () => {
    render(
      <CompactionNote
        events={[
          { elided: 1, reclaimedChars: 4000, beforeChars: 12000, afterChars: 8000, skillRescuedCount: 1, skillRescuedChars: 2200 },
          { elided: 1, reclaimedChars: 3000, beforeChars: 13000, afterChars: 10000, skillRescuedCount: 2, skillRescuedChars: 1200 },
        ]}
      />,
    );
    expect(screen.getByText("3 skill resources kept · 3.4K chars")).toBeTruthy();
  });
});
