// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";

const getJSON = vi.fn();
const postJSON = vi.fn();
const startConductorRun = vi.fn();
const applyConductorResult = vi.fn();

vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
}));

// The conductor store was merged into @/lib/conductor (C2). Mock the store
// surface but keep the real pure helpers (progressLabel, the fold/types the
// view also imports from the same module) via importOriginal.
vi.mock("@/lib/conductor", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/conductor")>();
  return {
    ...actual,
    useConductorStore: () => ({ runs: {}, activeCorr: null }),
    startConductorRun: (...a: unknown[]) => startConductorRun(...a),
    applyConductorResult: (...a: unknown[]) => applyConductorResult(...a),
    genConductorCorr: () => "corr-test",
  };
});

vi.mock("@/components/ModelPicker", () => ({
  ModelPicker: ({ activeModel }: { activeModel?: string }) => <button type="button">{activeModel || "auto"}</button>,
}));

import { UIProvider } from "@/components/ui/feedback";
import { Conductor } from "@/views/Conductor";

afterEach(cleanup);

beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  startConductorRun.mockReset();
  applyConductorResult.mockReset();
  getJSON.mockResolvedValue({ thinker: "gpt-think", worker: "gpt-work", verifier: "gpt-check" });
  postJSON.mockResolvedValue({ passed: true, answer: "done" });
});

describe("Conductor", () => {
  it("keeps task entry in a modal and starts the conductor from there", async () => {
    render(
      <UIProvider>
        <Conductor />
      </UIProvider>,
    );

    expect(screen.queryByLabelText("Conductor task")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /New task/ }));
    expect(screen.getByRole("dialog", { name: "New conductor task" })).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Conductor task"), { target: { value: "verify this implementation" } });
    fireEvent.click(within(screen.getByRole("group", { name: "Max rounds" })).getByRole("button", { name: "4" }));
    fireEvent.click(screen.getByRole("button", { name: /Conduct/ }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/conductor/ask", {
        task: "verify this implementation",
        thinker: "",
        worker: "",
        verifier: "",
        max_rounds: 4,
        plan: false,
        corr: "corr-test",
      }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "New conductor task" })).toBeNull());
  });
});
