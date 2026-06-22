// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AgentPage } from "@/views/AgentPage";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...args: unknown[]) => getJSON(...args),
}));
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ events: [], connected: true, subscribe: () => () => {} }),
}));

afterEach(cleanup);

beforeEach(() => {
  getJSON.mockReset();
  getJSON.mockImplementation((path: string) => {
    if (path === "/api/agents") {
      return Promise.resolve({
        profiles: [
          {
            id: "researcher",
            slug: "researcher",
            name: "Researcher",
            description: "Finds current facts and reports the useful parts.",
            soul: "You verify claims before answering. Keep results concise.",
            instructions: ["Check sources", "Report uncertainty"],
            model: "gpt-5",
            task_type: "research",
            enabled: true,
            trust_ceiling: "L3",
            memory_scope: "agent/researcher",
          },
        ],
      });
    }
    if (path === "/api/runs") {
      return Promise.resolve({
        runs: [
          {
            correlation_id: "run-1",
            agent: "researcher",
            status: "completed",
            started_unix_ms: Date.now() - 60_000,
          },
        ],
      });
    }
    if (path === "/api/standing") {
      return Promise.resolve({
        orders: [
          {
            id: "standing-1",
            name: "Morning scan",
            enabled: true,
            agent: "researcher",
            triggers: [{ type: "cron", schedule: "0 9 * * *" }],
          },
        ],
      });
    }
    if (path === "/api/schedules") {
      return Promise.resolve({
        schedules: [
          {
            id: "schedule-1",
            enabled: true,
            agent: "researcher",
            mode: "cadence",
            cadence: "every 15m",
            intent: "refresh facts",
          },
        ],
      });
    }
    if (path === "/api/workflows") return Promise.resolve({ workflows: [] });
    if (path === "/api/pulse") return Promise.resolve({ running: true });
    return Promise.resolve({});
  });
});

describe("AgentPage", () => {
  it("renders a simple identity page instead of the full command center", async () => {
    const onNavigate = vi.fn();
    render(<AgentPage slug="researcher" onNavigate={onNavigate} />);

    expect(await screen.findByRole("heading", { name: "researcher" })).toBeTruthy();
    expect(screen.getByText("Finds current facts and reports the useful parts.")).toBeTruthy();
    expect(screen.getByText("How this agent starts")).toBeTruthy();
    expect(screen.getByText("What to know")).toBeTruthy();
    expect(screen.getByText("0 9 * * *")).toBeTruthy();
    expect(screen.getByText("every 15m")).toBeTruthy();
    expect(screen.getByText("gpt-5")).toBeTruthy();
    expect(screen.getByText("L3")).toBeTruthy();
    expect(screen.getByRole("button", { name: /More identity details/i })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Wake researcher/i })).not.toBeTruthy();
  });

  it("shows a friendly empty state for missing agents", async () => {
    render(<AgentPage slug="missing" onNavigate={vi.fn()} />);

    expect(await screen.findByText("No agent “missing”")).toBeTruthy();
    expect(screen.getByText(/removed or never existed/i)).toBeTruthy();
  });
});
