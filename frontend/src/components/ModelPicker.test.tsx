// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
}));

import { ModelPicker } from "@/components/ModelPicker";

const CATALOG = {
  providers: [
    { id: "openai", name: "OpenAI", credentialed: true, models: [{ id: "gpt-4o", name: "GPT-4o" }] },
    { id: "anthropic", name: "Anthropic", credentialed: false, models: [{ id: "claude-x", name: "Claude X" }] },
  ],
};

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  getJSON.mockResolvedValue(CATALOG);
});

function openPicker() {
  render(<ModelPicker value="" onChange={() => {}} />);
  fireEvent.click(screen.getByTitle("Choose model"));
}

describe("ModelPicker keyed-only filter", () => {
  it("shows only keyed providers by default", async () => {
    openPicker();
    await waitFor(() => expect(screen.getByText("GPT-4o")).toBeTruthy());
    // Keyed provider's model is shown…
    expect(screen.getByText("OpenAI")).toBeTruthy();
    // …the un-keyed provider's model is hidden.
    expect(screen.queryByText("Claude X")).toBeNull();
    expect(screen.queryByText("Anthropic")).toBeNull();
    // Footer offers to reveal the rest.
    expect(screen.getByText(/Show all \(1 more\)/)).toBeTruthy();
  });

  it("reveals all providers when 'Show all' is clicked", async () => {
    openPicker();
    await waitFor(() => expect(screen.getByText("GPT-4o")).toBeTruthy());
    fireEvent.click(screen.getByText(/Show all \(1 more\)/));
    await waitFor(() => expect(screen.getByText("Claude X")).toBeTruthy());
    expect(screen.getByText("Anthropic")).toBeTruthy();
    // And can switch back.
    expect(screen.getByText("Show keyed only")).toBeTruthy();
  });

  it("explains the empty state when no provider has a key", async () => {
    getJSON.mockResolvedValue({
      providers: [{ id: "anthropic", name: "Anthropic", credentialed: false, models: [{ id: "claude-x", name: "Claude X" }] }],
    });
    openPicker();
    await waitFor(() => expect(screen.getByText(/No providers have an API key yet/)).toBeTruthy());
    // The hint links to revealing all providers.
    expect(screen.getByText(/show all 1 providers/)).toBeTruthy();
  });
});

describe("ModelPicker pinned group (M931)", () => {
  it("shows the pinned routing-chain group first, in chain order", async () => {
    getJSON.mockResolvedValue({
      providers: [
        {
          id: "openai",
          name: "OpenAI",
          credentialed: true,
          models: [
            { id: "gpt-4o", name: "GPT-4o" },
            { id: "gpt-4o-mini", name: "GPT-4o mini" },
          ],
        },
      ],
    });
    render(
      <ModelPicker
        value=""
        onChange={() => {}}
        pinned={{ label: "chat routing chain", ids: ["gpt-4o-mini", "gpt-4o", "not-in-catalog"] }}
      />,
    );
    fireEvent.click(screen.getByTitle("Choose model"));
    await waitFor(() => expect(screen.getByText("chat routing chain")).toBeTruthy());
    // Both chain models render in the pinned group (so they appear twice in the
    // modal: once pinned, once under their provider); unknown ids are skipped.
    expect(screen.getAllByText("GPT-4o mini").length).toBe(2);
    expect(screen.getAllByText("GPT-4o").length).toBe(2);
    // Chain order preserved: the mini (chain primary) is the first row overall.
    const rows = screen.getAllByText(/GPT-4o/).map((el) => el.textContent);
    expect(rows[0]).toBe("GPT-4o mini");
  });

  it("renders no pinned group when the chain is empty or absent", async () => {
    render(<ModelPicker value="" onChange={() => {}} pinned={{ label: "chat routing chain", ids: [] }} />);
    fireEvent.click(screen.getByTitle("Choose model"));
    await waitFor(() => expect(screen.getByText("GPT-4o")).toBeTruthy());
    expect(screen.queryByText("chat routing chain")).toBeNull();
  });
});
