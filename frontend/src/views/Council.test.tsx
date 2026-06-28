// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
const postJSON = vi.fn();

vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
}));

vi.mock("@/components/ModelPicker", () => ({
  ModelPicker: ({ value, onChange }: { value: string; onChange: (id: string) => void }) => (
    <button type="button" onClick={() => onChange("gpt-4o")}>
      {value || "pick model"}
    </button>
  ),
}));

import { UIProvider } from "@/components/ui/feedback";
import { Council } from "@/views/Council";

afterEach(cleanup);

beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  getJSON.mockResolvedValue({
    members: [
      { seat: "Planner", model: "gpt-4o" },
      { seat: "Critic", model: "claude-opus" },
    ],
  });
  postJSON.mockResolvedValue({ saved: true });
});

describe("Council", () => {
  it("edits council members in a modal instead of an inline settings panel", async () => {
    render(
      <UIProvider>
        <Council />
      </UIProvider>,
    );

    await waitFor(() => expect(screen.getByText(/Planner:/)).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /Edit/ }));

    expect(screen.getByRole("dialog", { name: "Council seats" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Add seat/ }));
    expect(screen.getByText("3 seats")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Close council modal" }));
    expect(screen.queryByRole("dialog", { name: "Council seats" })).toBeNull();
  });

  it("convenes with the selected round count from the segmented control", async () => {
    render(
      <UIProvider>
        <Council />
      </UIProvider>,
    );

    await waitFor(() => expect(screen.getByText(/Planner:/)).toBeTruthy());
    fireEvent.change(screen.getByLabelText("Council question"), { target: { value: "Should we ship?" } });
    fireEvent.click(screen.getByRole("button", { name: "3" }));
    fireEvent.click(screen.getByRole("button", { name: /Convene/ }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/council/ask",
        expect.objectContaining({ question: "Should we ship?", rounds: 3 }),
      ),
    );
  });
});
