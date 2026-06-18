// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));
vi.mock("@/lib/events", () => ({
  useEvents: () => ({
    subscribe: () => () => {},
  }),
}));

import { RunDetailLoader } from "@/components/RunDetail";

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postAction.mockReset();
});

describe("RunDetailLoader raw events", () => {
  it("shows incident badges and compact summaries in the raw event list", async () => {
    getJSON.mockResolvedValue({
      events: [
        {
          seq: 1,
          kind: "info",
          subject: "agent.wake",
          ts_unix_ms: 10,
          correlation_id: "run-1",
          payload: {
            agent: "infra-lead",
            phase: "completed",
            reason: "incident owner woke",
          },
        },
        {
          seq: 3,
          kind: "llm.request",
          ts_unix_ms: 12,
          correlation_id: "run-1",
          payload: { iter: 0, model: "mock-model" },
        },
        {
          seq: 4,
          kind: "tool.invoked",
          ts_unix_ms: 13,
          correlation_id: "run-1",
          payload: { iter: 0, tool: "shell" },
        },
        {
          seq: 5,
          kind: "tool.result",
          ts_unix_ms: 14,
          correlation_id: "run-1",
          payload: { iter: 0, tool: "shell" },
        },
        {
          seq: 6,
          kind: "task.completed",
          ts_unix_ms: 15,
          correlation_id: "run-1",
          payload: { answer: "done" },
        },
      ],
    });
    render(<RunDetailLoader correlationId="run-1" status="completed" />);
    await waitFor(() =>
      expect(getJSON).toHaveBeenCalledWith("/api/journal", {
        correlation_id: "run-1",
        limit: "500",
      }),
    );
    expect(screen.getByText("phase timeline")).toBeTruthy();
    expect(screen.getByText("thinking")).toBeTruthy();
    expect(screen.getByText("using tool")).toBeTruthy();
    expect(screen.getByText("observed tool")).toBeTruthy();
    expect(screen.getAllByText("completed").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: /raw events/i }));
    expect(screen.getByText("operator")).toBeTruthy();
    expect(screen.getAllByText("completed").length).toBeGreaterThan(0);
    expect(
      screen.getByText(/infra-lead · incident owner woke · agent\.wake/),
    ).toBeTruthy();
  });
});
