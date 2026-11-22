// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { Research } from "./Research";
import { UIProvider } from "@/components/ui/feedback";

vi.mock("@/app/api", () => ({
  // The daemon's answer: the text is `markdown`, the sub-questions are plain
  // strings, the claims carry a verdict, and the sources carry a hash.
  postJSON: vi.fn().mockImplementation(async (_url: string, body: any) => {
    return {
      markdown: `Answer to: ${body.question}`,
      sub_questions: ["Sub-question 1?"],
      sources: [{ id: "s-1", hash: "abc123", rank: 1, title: "Source 1", url: "https://example.com" }],
      claims: [
        { text: "Claim A", verdict: "supported", source_ids: ["s-1"], note: "" },
        { text: "Claim B", verdict: "refuted", source_ids: [], note: "" },
      ],
      notes: ["a note"],
      confidence: 0.8,
      cited_sources: 1,
      question: body.question,
      verified: true,
      correlation_id: "corr-1",
    };
  }),
}));

function wrap(ui: React.ReactNode) {
  return render(<UIProvider>{ui}</UIProvider>);
}

afterEach(cleanup);

describe("Research (Knowledge › Thinking Partners)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the heading + form", () => {
    wrap(<Research />);
    expect(screen.getByRole("heading", { level: 2, name: /Research/i })).toBeTruthy();
    expect(screen.getByPlaceholderText(/safest way to migrate/i)).toBeTruthy();
    expect(screen.getByRole("button", { name: /Ask Research/i })).toBeTruthy();
  });

  it("POSTs to /api/research/ask and renders the answer + sub-questions + sources + claims", async () => {
    wrap(<Research />);
    fireEvent.change(screen.getByPlaceholderText(/safest way to migrate/i), {
      target: { value: "How does RAG work?" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Ask Research/i }));

    await waitFor(() => {
      expect(screen.getByText(/Answer to: How does RAG work/i)).toBeTruthy();
    });
    expect(screen.getByText("Sub-question 1?")).toBeTruthy();
    expect(screen.getByText("Source 1")).toBeTruthy();
    expect(screen.getByText("Claim A")).toBeTruthy();
    expect(screen.getByText("Claim B")).toBeTruthy();
  });

  it("disables Ask when the question is empty", () => {
    wrap(<Research />);
    const btn = screen.getByRole("button", { name: /Ask Research/i }) as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
  });
});
