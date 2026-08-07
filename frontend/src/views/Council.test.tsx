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

  it("lists past convenings from the journal and replays one on click", async () => {
    const convened = {
      id: "ev-1",
      seq: 10,
      kind: "council.convened",
      correlation_id: "wc-old",
      ts_unix_ms: Date.now() - 3_600_000,
      payload: { question: "Postgres or SQLite?", seats: ["Planner", "Critic"], rounds: 2 },
    };
    getJSON.mockImplementation((path: string, params?: Record<string, string>) => {
      if (path === "/api/council/members")
        return Promise.resolve({ members: [{ seat: "Planner", model: "gpt-4o" }, { seat: "Critic", model: "claude-opus" }] });
      if (path === "/api/journal" && params?.kind === "council.convened")
        return Promise.resolve({ events: [convened] });
      if (path === "/api/journal" && params?.correlation_id === "wc-old")
        return Promise.resolve({
          events: [
            convened,
            {
              id: "ev-2",
              seq: 11,
              kind: "council.consensus",
              correlation_id: "wc-old",
              payload: { consensus: "SQLite for a single node.", has_dissent: false },
            },
          ],
        });
      return Promise.resolve({});
    });

    render(
      <UIProvider>
        <Council />
      </UIProvider>,
    );

    // The history strip shows the past question with its seat/round shape.
    await waitFor(() => expect(screen.getByText("Postgres or SQLite?")).toBeTruthy());
    expect(screen.getByText(/2 seats · 2r/)).toBeTruthy();

    // Replaying folds its journaled events back into the live view.
    fireEvent.click(screen.getByTitle("Postgres or SQLite?"));
    await waitFor(() =>
      expect(getJSON).toHaveBeenCalledWith("/api/journal", expect.objectContaining({ correlation_id: "wc-old" })),
    );
    await waitFor(() => expect(screen.getByText("SQLite for a single node.")).toBeTruthy());
  });

  it("nudges for a second seat when the council is a monologue", async () => {
    getJSON.mockImplementation((path: string) =>
      path === "/api/council/members"
        ? Promise.resolve({ members: [{ seat: "Elder Alpha", model: "gpt-4o" }] })
        : Promise.resolve({}),
    );
    render(
      <UIProvider>
        <Council />
      </UIProvider>,
    );
    await waitFor(() => expect(screen.getByText(/One seat means one opinion/)).toBeTruthy());
    // The nudge opens the same seats modal as the Edit action.
    fireEvent.click(screen.getByRole("button", { name: /Add a seat/ }));
    expect(screen.getByRole("dialog", { name: "Council seats" })).toBeTruthy();
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
