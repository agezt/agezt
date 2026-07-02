// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
const postJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
}));

import { OKR, okrAchievedCount, okrAveragePercent } from "@/views/OKR";

const objectives = [
  {
    id: "obj1",
    title: "Ship the proof loop",
    description: "PROOF > VIBES",
    owner: "founder",
    status: "active",
    percent: 50,
    achieved: false,
    key_result_count: 1,
    progress: {
      percent: 50,
      achieved: false,
      key_results: [
        { id: "kr1", title: "2 gated tasks proven", done: 1, total: 2, target: 2, percent: 50, achieved: false },
      ],
    },
  },
  {
    id: "obj2",
    title: "Close Hermes gap",
    status: "achieved",
    percent: 100,
    achieved: true,
    progress: { percent: 100, achieved: true, key_results: [] },
  },
];

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postJSON.mockResolvedValue({});
});

describe("OKR helpers", () => {
  it("summarizes achieved count and average progress", () => {
    expect(okrAchievedCount(objectives)).toBe(1);
    expect(okrAveragePercent(objectives)).toBe(75);
    expect(okrAveragePercent([])).toBe(0);
  });
});

describe("OKR view", () => {
  it("renders objectives with rollup and posts a new objective", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/okr") return Promise.resolve({ objectives, count: objectives.length });
      return Promise.reject(new Error(`unexpected ${path}`));
    });

    render(<OKR />);

    await waitFor(() => expect(screen.getByText("Ship the proof loop")).toBeTruthy());
    expect(screen.getAllByText("2 gated tasks proven").length).toBeGreaterThan(0);
    // Objective 2 is achieved.
    expect(screen.getByText("Close Hermes gap")).toBeTruthy();
    expect(screen.getAllByText("achieved").length).toBeGreaterThan(0);

    fireEvent.change(screen.getByLabelText("New objective title"), { target: { value: "New goal" } });
    fireEvent.click(screen.getByText("Objective"));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/okr/create", expect.objectContaining({ title: "New goal" })),
    );
  });

  it("posts add-key-result and link-task actions", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/okr") return Promise.resolve({ objectives: [objectives[0]], count: 1 });
      if (path === "/api/workboard") return Promise.resolve({ tasks: [{ id: "task-x", title: "do the thing", status: "ready" }] });
      return Promise.reject(new Error(`unexpected ${path}`));
    });

    render(<OKR />);
    await waitFor(() => expect(screen.getByText("Ship the proof loop")).toBeTruthy());

    fireEvent.change(screen.getByLabelText("Key result title"), { target: { value: "docs done" } });
    fireEvent.click(screen.getByTitle("Add key result"));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/okr/keyresult", expect.objectContaining({ id: "obj1", title: "docs done" })),
    );

    fireEvent.change(screen.getByLabelText("Key result to link"), { target: { value: "kr1" } });
    await waitFor(() => expect((screen.getByLabelText("Task to link") as HTMLSelectElement).disabled).toBe(false));
    fireEvent.change(screen.getByLabelText("Task to link"), { target: { value: "task-x" } });
    fireEvent.click(screen.getByTitle("Link a workboard task to a key result"));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/okr/link", expect.objectContaining({ id: "obj1", key_result: "kr1", task: "task-x" })),
    );
  });
});
