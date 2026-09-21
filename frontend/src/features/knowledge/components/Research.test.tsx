// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { Research } from "./Research";
import { UIProvider } from "@/components/ui/feedback";

vi.mock("@/app/api", () => ({
  postJSON: vi.fn().mockImplementation(async (_url: string, body: any) => {
    return {
      answer: `Answer to: ${body.question}`,
      sub_questions: [
        { id: "sq-1", question: "Sub-question 1?", answer: "Yes." },
      ],
      sources: [{ id: "s-1", title: "Source 1", url: "https://example.com", confidence: 0.9 }],
      verified_claims: [
        { claim: "Claim A", verified: true, sources: ["s-1"] },
        { claim: "Claim B", verified: false, sources: [] },
      ],
      duration_ms: 1234,
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
