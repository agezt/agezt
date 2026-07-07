// @vitest-environment jsdom
import { render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { cleanup } from "@testing-library/react";

const getJSON = vi.fn();
const liveEvents = vi.hoisted(() => ({ events: [] as any[] }));

vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postAction: vi.fn().mockResolvedValue({}),
}));

vi.mock("@/components/ui/feedback", () => ({
  useUI: () => ({ toast: vi.fn() }),
}));

vi.mock("@/lib/events", () => ({
  useEvents: () => ({
    events: liveEvents.events,
    connected: true,
    subscribe: () => () => {},
  }),
}));

import { Overseer, overseerShouldRefresh } from "@/views/Overseer";
import { buildLiveRunContexts, liveWakeLabel } from "@/lib/liveruncontext";

const withPage = (node: ReactNode) => <div>{node}</div>;

afterEach(cleanup);

beforeEach(() => {
  getJSON.mockReset();
  liveEvents.events = [];
});

describe("buildLiveRunContexts", () => {
  it("folds live events into per-run agent, tool, model, and wake context", () => {
    const got = buildLiveRunContexts([
      {
        kind: "tool.invoked",
        correlation_id: "run-1",
        actor: "ops",
        ts_unix_ms: 300,
        payload: { tool: "shell.exec" },
      },
      {
        kind: "llm.request",
        correlation_id: "run-1",
        actor: "ops",
        ts_unix_ms: 200,
        payload: { model: "gpt-5" },
      },
      {
        kind: "task.received",
        correlation_id: "run-1",
        actor: "ops",
        ts_unix_ms: 100,
        payload: { intent: "sync catalog", wake_source: "schedule", schedule_id: "sch-sync" },
      },
    ]);

    expect(got["run-1"]).toEqual({
      agent: "ops",
      phase: "using tool",
      detail: "shell.exec",
      tool: "shell.exec",
      model: "gpt-5",
      wakeSource: "schedule",
      scheduleId: "sch-sync",
      lastEventMs: 300,
    });
    expect(liveWakeLabel(got["run-1"])).toBe("schedule sch-sync");
  });

  it("refreshes on supervisor-significant repair and retry events", () => {
    expect(overseerShouldRefresh({ subject: "doctor.auto_repair", kind: "info" } as any)).toBe(true);
    expect(overseerShouldRefresh({ kind: "agent.retry" } as any)).toBe(true);
    expect(overseerShouldRefresh({ kind: "llm.token" } as any)).toBe(false);
  });
});

describe("Overseer", () => {
  it("shows active run live context from the event stream", async () => {
    liveEvents.events = [
      {
        subject: "doctor.auto_repair",
        kind: "info",
        correlation_id: "repair-1",
        actor: "guardian-doctor",
        ts_unix_ms: 400,
        payload: {
          agent: "builder",
          delegate_to: "lead",
          reason: "tool denied",
        },
      },
      {
        kind: "tool.invoked",
        correlation_id: "run-1",
        actor: "ops",
        ts_unix_ms: 300,
        payload: { tool: "shell.exec" },
      },
      {
        kind: "llm.request",
        correlation_id: "run-1",
        actor: "ops",
        ts_unix_ms: 200,
        payload: { model: "gpt-5" },
      },
      {
        kind: "task.received",
        correlation_id: "run-1",
        actor: "ops",
        ts_unix_ms: 100,
        payload: { intent: "sync catalog", wake_source: "schedule", schedule_id: "sch-sync" },
      },
    ];
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/runs")
        return Promise.resolve({
          runs: [{ correlation_id: "run-1", status: "running", intent: "sync catalog", started_unix_ms: 100 }],
        });
      if (path === "/api/agents")
        return Promise.resolve({ profiles: [{ slug: "ops", enabled: true }] });
      if (path === "/api/board/help") return Promise.resolve({ open_help: [] });
      return Promise.resolve({});
    });

    render(withPage(<Overseer />));

    await waitFor(() => expect(screen.getByText("sync catalog")).toBeTruthy());
    expect(screen.getAllByText("ops").length).toBeGreaterThan(0);
    expect(screen.getByText("using tool")).toBeTruthy();
    expect(screen.getByText("tool shell.exec")).toBeTruthy();
    expect(screen.getByText(/schedule sch-sync/)).toBeTruthy();
    expect(screen.getByText("gpt-5")).toBeTruthy();
    expect(screen.getByText("builder · to lead · tool denied · doctor.auto_repair")).toBeTruthy();
  });
});
