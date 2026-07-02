// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor, within } from "@testing-library/react";

const getJSON = vi.fn();
const postJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
}));

import {
  Workboard,
  workboardHashForTask,
  workboardOpenCount,
  workboardRetryText,
  workboardStatusCounts,
  workboardTaskFromHash,
} from "@/views/Workboard";

const task = {
  id: "wb1",
  title: "Ship Hermes parity",
  description: "Close durable task orchestration gaps.",
  status: "ready",
  priority: 2,
  tenant: "default",
  assignee: "builder",
  owner: "ops",
  updated_ms: 1_710_000_000_000,
  retry_policy: { max_attempts: 2, escalate_to: "lead" },
  failed_attempt_count: 1,
  max_attempts: 2,
  next_attempt: 2,
  dependencies: [{ id: "dep1" }],
  attempts: [
    { id: "attempt-1", agent: "builder", run_id: "run-123456789", status: "failed", summary: "missing artifact" },
  ],
  comments: [{ id: "c1", author: "planner", body: "needs review", created_ms: 1_710_000_001_000 }],
  links: [{ id: "l1", type: "run", target: "run-123456789" }],
  artifacts: ["artifact://diff/1"],
};

afterEach(cleanup);
beforeEach(() => {
  location.hash = "";
  getJSON.mockReset();
  postJSON.mockReset();
  postJSON.mockResolvedValue({});
});

describe("Workboard helpers", () => {
  it("builds and parses bookmarkable task hashes", () => {
    expect(workboardTaskFromHash("#workboard?task=wb1")).toBe("wb1");
    expect(workboardTaskFromHash("#/workboard?task=wb%2Flead")).toBe("wb/lead");
    expect(workboardTaskFromHash("#workboard")).toBe("");
    expect(workboardHashForTask(" wb/lead ")).toBe("#workboard?task=wb%2Flead");
    expect(workboardHashForTask("")).toBe("#workboard");
  });

  it("summarizes lane status and retry state", () => {
    const lanes: Array<{ counts: Record<string, number> }> = [
      { counts: { ready: 2, running: 1 } },
      { counts: { done: 3, blocked: 1 } },
    ];
    expect(workboardStatusCounts(lanes)).toEqual({ ready: 2, running: 1, done: 3, blocked: 1 });
    expect(workboardOpenCount(lanes)).toBe(4);
    expect(workboardRetryText(task)).toBe("1/2 failed · next 2 · escalate lead");
    expect(workboardRetryText({ id: "wb2" })).toBe("");
  });
});

describe("Workboard view", () => {
  it("renders lane detail, watch context, and posts task comments", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/workboard/lanes") {
        return Promise.resolve({
          count: 1,
          task_count: 1,
          lanes: [
            {
              assignee: "builder",
              label: "builder",
              count: 1,
              counts: { ready: 1 },
              tasks: [task],
            },
          ],
        });
      }
      if (path === "/api/workboard/watch") {
        return Promise.resolve({
          task,
          events: [
            { seq: 11, ts_unix_ms: 1_710_000_002_000, kind: "workboard.task.updated", payload: { action: "retry" } },
          ],
          blocked_dependencies: [{ id: "dep1", title: "Prerequisite", status: "triage" }],
        });
      }
      return Promise.reject(new Error(`unexpected ${path}`));
    });

    render(<Workboard />);

    await waitFor(() => expect(screen.getAllByText("Ship Hermes parity").length).toBeGreaterThan(0));
    expect(screen.getAllByText("1/2 failed · next 2 · escalate lead").length).toBeGreaterThan(0);
    expect(screen.getByText("Close durable task orchestration gaps.")).toBeTruthy();
    expect(screen.getByText("Dependencies")).toBeTruthy();
    await waitFor(() => expect(screen.getByText(/Prerequisite/)).toBeTruthy());
    expect(screen.getByText("Attempts")).toBeTruthy();
    expect(screen.getByText("missing artifact")).toBeTruthy();
    expect(screen.getByText("Links and Artifacts")).toBeTruthy();
    expect(screen.getByText("artifact://diff/1")).toBeTruthy();
    expect(screen.getByText("Events")).toBeTruthy();
    expect(screen.getByText("retry")).toBeTruthy();
    expect(within(screen.getByText("Comments").closest("section") as HTMLElement).getByText("needs review")).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Comment"), { target: { value: "looks good" } });
    fireEvent.click(screen.getByTitle("Comment"));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/workboard/comment",
        expect.objectContaining({
          id: "wb1",
          actor: "operator",
          author: "operator",
          body: "looks good",
        }),
      ),
    );
  });

  it("shows the proof panel, gates Complete, and posts a prove", async () => {
    const gated = {
      ...task,
      id: "wb-gated",
      status: "review",
      gated: true,
      proven: false,
      criteria_count: 2,
      criteria_met: 1,
      criteria: [
        { text: "tests pass", met: true, note: "green" },
        { text: "doc updated", met: false },
      ],
      proof: {
        verdict: { complete: false, gap: "doc still stale" },
        evidence: { corr: "run-1", artifacts: ["a1"], journal_from: 10, journal_to: 42 },
      },
    };
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/workboard/lanes") {
        return Promise.resolve({ count: 1, task_count: 1, lanes: [{ assignee: "builder", label: "builder", count: 1, counts: { review: 1 }, tasks: [gated] }] });
      }
      if (path === "/api/workboard/watch") {
        return Promise.resolve({ task: gated, events: [], blocked_dependencies: [] });
      }
      return Promise.reject(new Error(`unexpected ${path}`));
    });

    render(<Workboard />);

    await waitFor(() => expect(screen.getAllByText("Ship Hermes parity").length).toBeGreaterThan(0));
    // Proof panel content.
    expect(screen.getByText("unproven")).toBeTruthy();
    expect(screen.getByText("1/2 criteria met")).toBeTruthy();
    expect(screen.getByText("tests pass")).toBeTruthy();
    expect(screen.getByText(/doc still stale/)).toBeTruthy();
    // Complete is gated (disabled) while unproven.
    const completeBtn = screen.getByText("Complete").closest("button") as HTMLButtonElement;
    expect(completeBtn.disabled).toBe(true);
    // Prove posts to the prove endpoint.
    fireEvent.click(screen.getByText("Prove"));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/workboard/prove", expect.objectContaining({ id: "wb-gated" })),
    );
  });

  it("creates a task from the New task form with criteria and seat", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/seats") return Promise.resolve({ seats: [{ id: "default", name: "Default" }, { id: "reader", name: "Reader" }] });
      if (path === "/api/workboard/lanes") return Promise.resolve({ count: 0, task_count: 0, lanes: [] });
      return Promise.reject(new Error(`unexpected ${path}`));
    });

    render(<Workboard />);
    await waitFor(() => expect(screen.getByText("New task")).toBeTruthy());
    fireEvent.click(screen.getByText("New task"));

    fireEvent.change(screen.getByLabelText("Task title"), { target: { value: "ship it" } });
    fireEvent.change(screen.getByLabelText("Acceptance criteria"), { target: { value: "tests pass\ndoc updated" } });
    fireEvent.change(screen.getByLabelText("Task seat"), { target: { value: "reader" } });
    fireEvent.click(screen.getByText("Create"));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/workboard/create",
        expect.objectContaining({ title: "ship it", seat: "reader", criteria: ["tests pass", "doc updated"] }),
      ),
    );
  });

  it("shows the execution seat picker and posts a seat change", async () => {
    const seated = { ...task, id: "wb-seat", seat: "reader" };
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/seats") {
        return Promise.resolve({
          seats: [
            { id: "default", name: "Default", description: "inherit" },
            { id: "reader", name: "Reader", description: "read only" },
            { id: "isolated", name: "Isolated", description: "sandboxed", execution_profile: "warden" },
          ],
        });
      }
      if (path === "/api/workboard/lanes") {
        return Promise.resolve({ count: 1, task_count: 1, lanes: [{ assignee: "builder", label: "builder", count: 1, counts: { ready: 1 }, tasks: [seated] }] });
      }
      if (path === "/api/workboard/watch") {
        return Promise.resolve({ task: seated, events: [], blocked_dependencies: [] });
      }
      return Promise.reject(new Error(`unexpected ${path}`));
    });

    render(<Workboard />);
    await waitFor(() => expect(screen.getByLabelText("Execution seat")).toBeTruthy());
    const select = screen.getByLabelText("Execution seat") as HTMLSelectElement;
    expect(select.value).toBe("reader");
    fireEvent.change(select, { target: { value: "isolated" } });
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/workboard/seat", expect.objectContaining({ id: "wb-seat", seat: "isolated" })),
    );
  });
});
