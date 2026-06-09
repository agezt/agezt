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
