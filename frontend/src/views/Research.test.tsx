// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const postJSON = vi.fn();

vi.mock("@/lib/api", () => ({
  getJSON: vi.fn(),
  postJSON: (...a: unknown[]) => postJSON(...a),
}));

import { Research } from "@/views/Research";

afterEach(cleanup);
beforeEach(() => {
  postJSON.mockReset();
});

const sampleReport = {
  question: "why is the sky blue?",
  sub_questions: ["why is the sky blue?"],
  sources: [{ id: "S1", url: "https://example.com/rayleigh", title: "Rayleigh scattering", rank: 1 }],
  markdown: "The sky is blue due to Rayleigh scattering [S1].",
  claims: [
    { text: "The sky is blue due to Rayleigh scattering.", source_ids: ["S1"], verdict: "supported", note: "source confirms" },
    { text: "The sky is blue because of the ocean.", source_ids: ["S1"], verdict: "refuted", note: "source contradicts" },
  ],
  confidence: 0.5,
  cited_sources: 1,
  verified: true,
  notes: ["1 of 2 verified claim(s) were REFUTED under adversarial check"],
};

describe("Research", () => {
  it("renders the ask surface", () => {
    render(<Research />);
    expect(screen.getByLabelText("Research question")).toBeTruthy();
    expect(screen.getByRole("button", { name: /research/i })).toBeTruthy();
  });

  it("runs a query and renders the cited report with verdicts", async () => {
    postJSON.mockResolvedValue(sampleReport);
    render(<Research />);

    fireEvent.change(screen.getByLabelText("Research question"), {
      target: { value: "why is the sky blue?" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^research$/i }));

    await waitFor(() => expect(postJSON).toHaveBeenCalledTimes(1));
    const [path, body] = postJSON.mock.calls[0];
    expect(path).toBe("/api/research/ask");
    expect((body as { question: string }).question).toBe("why is the sky blue?");
    expect((body as { verify: boolean }).verify).toBe(true);

    // Confidence + verified badge. With a refuted claim present the badge must
    // reflect the result (never a bare green "verified").
    expect(await screen.findByText(/50% confidence/)).toBeTruthy();
    expect(screen.getByText(/verified · 1 refuted/i)).toBeTruthy();
    // The refuted claim and its verdict chip are surfaced.
    expect(screen.getByText("The sky is blue because of the ocean.")).toBeTruthy();
    expect(screen.getAllByText(/refuted/i).length).toBeGreaterThan(0);
    // Source link points at the original page.
    const link = screen.getByRole("link", { name: /Rayleigh scattering/i });
    expect(link.getAttribute("href")).toBe("https://example.com/rayleigh");
  });

  it("can disable verification before running", async () => {
    postJSON.mockResolvedValue({ ...sampleReport, verified: false, claims: [] });
    render(<Research />);
    fireEvent.change(screen.getByLabelText("Research question"), { target: { value: "q" } });
    fireEvent.click(screen.getByLabelText(/adversarial verification/i)); // toggle off
    fireEvent.click(screen.getByRole("button", { name: /^research$/i }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledTimes(1));
    expect((postJSON.mock.calls[0][1] as { verify: boolean }).verify).toBe(false);
  });

  it("surfaces an error when the request fails", async () => {
    postJSON.mockRejectedValue(new Error("no provider configured"));
    render(<Research />);
    fireEvent.change(screen.getByLabelText("Research question"), { target: { value: "q" } });
    fireEvent.click(screen.getByRole("button", { name: /^research$/i }));
    expect(await screen.findByText(/no provider configured/)).toBeTruthy();
  });
});
