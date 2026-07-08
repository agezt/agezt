// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...args: unknown[]) => getJSON(...args),
  postJSON: (...args: unknown[]) => postJSON(...args),
  postAction: (...args: unknown[]) => postAction(...args),
}));
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ events: [], connected: true, subscribe: () => () => {} }),
}));

import { AgentPage } from "@/views/AgentPage";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

afterEach(cleanup);

beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postAction.mockReset();
  postJSON.mockResolvedValue({ removed: true });
  postAction.mockResolvedValue({});
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
    if (path === "/api/memory") return Promise.resolve({ records: [] });
    if (path === "/api/skills") return Promise.resolve({ skills: [] });
    if (path === "/api/policy_log") return Promise.resolve({ decisions: [] });
    if (path === "/api/tool_log") return Promise.resolve({ invocations: [] });
    if (path === "/api/policy") return Promise.resolve({});
    if (path === "/api/edict_show") return Promise.resolve({ levels: {} });
    if (path === "/api/tools_catalog") return Promise.resolve({ tools: [] });
    if (path === "/api/agents/permissions") return Promise.resolve({});
    if (path === "/api/board") return Promise.resolve({ messages: [] });
    if (path === "/api/chains") return Promise.resolve({ chains: {} });
    if (path === "/api/routing") return Promise.resolve({ chains: {} });
    if (path === "/api/provider_log") return Promise.resolve({ events: [] });
    if (path === "/api/reaper/scan") return Promise.resolve({});
    if (path === "/api/agents/repair_status") return Promise.resolve({});
    if (path === "/api/agents/escalations") return Promise.resolve({ escalations: [] });
    return Promise.resolve({});
  });
});

describe("AgentPage", () => {
  it("renders the full AgentDetail command center with all tabs", async () => {
    render(withUI(<AgentPage slug="researcher" onNavigate={vi.fn()} />));

    // AgentDetail renders the slug as a font-mono heading.
    expect(await screen.findByText("researcher")).toBeTruthy();

    // The tab list should have all the primary tabs.
    expect(screen.getByRole("tab", { name: /Overview/ })).toBeTruthy();
    expect(screen.getByRole("tab", { name: /Activity/ })).toBeTruthy();
    expect(screen.getByRole("tab", { name: /Triggers/ })).toBeTruthy();
    expect(screen.getByRole("tab", { name: /Comms/ })).toBeTruthy();

    // The Wake button should be present (it's a direct_callable agent).
    expect(screen.getByRole("button", { name: /Wake researcher/i })).toBeTruthy();

    // The overview tab renders the agent command center with secondary tabs
    // (soul, model, memory, skills, repair, diagnostics, files).
    expect(screen.getByRole("tablist", { name: "researcher detail sections" })).toBeTruthy();
    expect(screen.getByRole("tab", { name: /Soul/ })).toBeTruthy();
    expect(screen.getByRole("tab", { name: /Repair/ })).toBeTruthy();
    expect(screen.getByRole("tab", { name: /Diagnostics/ })).toBeTruthy();
    expect(screen.getByRole("tab", { name: /Files/ })).toBeTruthy();
  });

  it("shows a friendly empty state for missing agents", async () => {
    render(<AgentPage slug="missing" onNavigate={vi.fn()} />);

    expect(await screen.findByText(/No agent/i)).toBeTruthy();
    expect(screen.getByText(/removed or never existed/i)).toBeTruthy();
  });
});
