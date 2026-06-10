// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ events: [], connected: true, subscribe: () => () => {} }),
}));

import {
  Workflows,
  CopilotPanel,
  RunsDrawer,
  runToStatus,
  toFlow,
  fromFlow,
  portsForNode,
  summarize,
  type Wf,
  type WfRun,
} from "@/views/Workflows";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

// React Flow needs ResizeObserver; jsdom has none. A no-op keeps the editor
// mountable in tests (we assert the top bar, not canvas geometry).
class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
(globalThis as { ResizeObserver?: unknown }).ResizeObserver ??= NoopResizeObserver;

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postAction.mockReset();
  postJSON.mockResolvedValue({});
  postAction.mockResolvedValue({});
});

describe("portsForNode", () => {
  it("derives output ports per node type", () => {
    expect(portsForNode("condition")).toEqual(["true", "false"]);
    expect(portsForNode("transform")).toEqual(["out"]);
    expect(portsForNode("tool")).toEqual(["out", "error"]); // failable
    expect(portsForNode("code")).toEqual(["out", "error"]);
    expect(
      portsForNode("switch", { cases: [{ equals: "a", port: "ops" }, { equals: "b", port: "dev" }] }),
    ).toEqual(["ops", "dev", "default"]);
  });
});

describe("summarize", () => {
  it("renders the canvas gist per type", () => {
    expect(summarize("trigger", { kind: "cron", interval_sec: 30 })).toBe("cron every 30s");
    expect(summarize("trigger", { kind: "event", subject: "memory.>" })).toBe("event on memory.>");
    expect(summarize("trigger")).toBe("manual");
    expect(summarize("http", { method: "GET", url: "https://x" })).toBe("GET https://x");
    expect(summarize("condition", { left: "{{a}}", op: "gt", right: "5" })).toBe("{{a}} gt 5");
    expect(summarize("merge", {})).toBe("any");
  });
});

describe("toFlow / fromFlow", () => {
  const wf: Wf = {
    name: "round-trip",
    description: "demo",
    nodes: [
      { id: "start", type: "trigger", x: 10, y: 20 },
      { id: "check", type: "condition", label: "Big?", config: { left: "{{trigger.payload.n}}", op: "gt", right: "5" }, x: 10, y: 160 },
      { id: "win", type: "transform", config: { template: "BIG" }, x: 0, y: 300 },
    ],
    edges: [
      { from: "start", to: "check" },
      { from: "check", to: "win", port: "true" },
    ],
  };

  it("round-trips reliability settings (M808) losslessly", () => {
    const withSettings: Wf = {
      name: "rel",
      nodes: [
        { id: "start", type: "trigger" },
        { id: "call", type: "tool", config: { tool: "http" }, timeout_sec: 30, retries: 2, retry_delay_sec: 5 },
      ],
      edges: [{ from: "start", to: "call" }],
    };
    const { nodes, edges } = toFlow(withSettings);
    const back = fromFlow("rel", "", nodes, edges);
    expect(back.nodes[1].timeout_sec).toBe(30);
    expect(back.nodes[1].retries).toBe(2);
    expect(back.nodes[1].retry_delay_sec).toBe(5);
    // Unset settings stay absent (no zero-noise in the saved graph).
    expect(back.nodes[0].timeout_sec).toBeUndefined();
    expect(back.nodes[0].retries).toBeUndefined();
  });

  it("round-trips a workflow through the canvas shapes losslessly", () => {
    const { nodes, edges } = toFlow(wf);
    expect(nodes).toHaveLength(3);
    expect(nodes[0].position).toEqual({ x: 10, y: 20 });
    // Default port rides as the "out" handle; named ports keep their name.
    expect(edges[0].sourceHandle).toBe("out");
    expect(edges[1].sourceHandle).toBe("true");

    const back = fromFlow(wf.name, wf.description || "", nodes, edges);
    expect(back.nodes.map((n) => n.id)).toEqual(["start", "check", "win"]);
    expect(back.nodes[1].label).toBe("Big?");
    expect(back.nodes[1].config).toEqual({ left: "{{trigger.payload.n}}", op: "gt", right: "5" });
    // "out" handle folds back to the kernel's default "" port (omitted).
    expect(back.edges?.[0].port).toBeUndefined();
    expect(back.edges?.[1].port).toBe("true");
  });
});

describe("Workflows list", () => {
  const twoFlows = {
    workflows: [
      {
        id: "01A", name: "triage", enabled: true, node_count: 10,
        trigger_kind: "event", trigger_detail: "on memory.>", description: "smoke pipeline",
      },
      { id: "01B", name: "heartbeat", enabled: false, node_count: 2, trigger_kind: "cron", trigger_detail: "every 30s" },
    ],
    count: 2,
  };

  it("renders workflows with trigger info and state", async () => {
    getJSON.mockResolvedValue(twoFlows);
    render(withUI(<Workflows />));
    await waitFor(() => expect(screen.getByText("triage")).toBeTruthy());
    expect(screen.getByText("heartbeat")).toBeTruthy();
    expect(screen.getByText("event (on memory.>)")).toBeTruthy();
    expect(screen.getByText("cron (every 30s)")).toBeTruthy();
    expect(screen.getByText("disabled")).toBeTruthy();
    expect(getJSON).toHaveBeenCalledWith("/api/workflows");
  });

  it("enable toggle posts the flipped flag", async () => {
    getJSON.mockResolvedValue(twoFlows);
    render(withUI(<Workflows />));
    await waitFor(() => expect(screen.getByText("heartbeat")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Enable heartbeat" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/workflows/enable", { ref: "heartbeat", enabled: "true" }),
    );
  });

  it("rejects an illegal new-workflow name without opening the canvas", async () => {
    getJSON.mockResolvedValue({ workflows: [], count: 0 });
    render(withUI(<Workflows />));
    await waitFor(() => expect(screen.getByText("No workflows yet")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /New workflow/ }));
    fireEvent.change(screen.getByLabelText("Workflow name"), { target: { value: "Bad Name" } });
    fireEvent.click(screen.getByRole("button", { name: "Create workflow" }));
    // Still in list mode (the canvas top bar never rendered).
    expect(screen.queryByRole("button", { name: "Run workflow" })).toBeNull();
  });

  it("shows the empty state when nothing is saved", async () => {
    getJSON.mockResolvedValue({ workflows: [], count: 0 });
    render(withUI(<Workflows />));
    await waitFor(() => expect(screen.getByText("No workflows yet")).toBeTruthy());
  });
});

describe("template gallery", () => {
  const gallery = {
    templates: [
      {
        name: "resilient-fetch",
        title: "Resilient fetch",
        description: "error-port rescue demo",
        category: "demo",
        node_count: 4,
        workflow: {
          name: "resilient-fetch",
          nodes: [
            { id: "start", type: "trigger", x: 40, y: 40 },
            { id: "fetch", type: "http", config: { method: "GET", url: "{{trigger.payload.url}}" }, x: 40, y: 190 },
          ],
          edges: [{ from: "start", to: "fetch" }],
        },
      },
    ],
    count: 1,
  };

  it("instantiates a template onto the canvas under the new name", async () => {
    getJSON.mockImplementation((path: string) =>
      path === "/api/workflows/templates"
        ? Promise.resolve(gallery)
        : Promise.resolve({ workflows: [], count: 0 }),
    );
    render(withUI(<Workflows />));
    await waitFor(() => expect(screen.getByText("No workflows yet")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /New workflow/ }));
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/workflows/templates"));
    fireEvent.change(screen.getByLabelText("Workflow name"), { target: { value: "my-fetch" } });
    fireEvent.change(screen.getByLabelText("Start from template"), { target: { value: "resilient-fetch" } });
    fireEvent.click(screen.getByRole("button", { name: "Create workflow" }));
    // The canvas opened on the template graph under the NEW name.
    await waitFor(() => expect(screen.getByText("my-fetch")).toBeTruthy());
    expect(screen.getByText("2 node(s) · 1 edge(s)")).toBeTruthy();
  });
});

describe("run history", () => {
  const failedRun: WfRun = {
    correlation_id: "run-001",
    status: "failed",
    started_ms: 1000,
    finished_ms: 3500,
    error: "node call: boom",
    node_events: [
      { node: "start", ok: true },
      { node: "rescue", ok: false, handled: true, port: "error" },
      { node: "call", ok: false },
    ],
  };

  it("runToStatus mirrors the live ok/handled rule", () => {
    expect(runToStatus(failedRun)).toEqual({ start: "done", rescue: "done", call: "failed" });
    expect(runToStatus({ correlation_id: "x", status: "running" })).toEqual({});
  });

  it("RunsDrawer lists runs from the journal fold and replays on click", async () => {
    getJSON.mockResolvedValue({ runs: [failedRun], count: 1 });
    const onReplay = vi.fn();
    render(withUI(<RunsDrawer name="wire-flow" onReplay={onReplay} onError={vi.fn()} />));
    await waitFor(() => expect(screen.getByText("failed")).toBeTruthy());
    expect(getJSON).toHaveBeenCalledWith("/api/workflows/runs", { ref: "wire-flow" });
    expect(screen.getByText(/3 node\(s\)/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Replay run run-001" }));
    expect(onReplay).toHaveBeenCalledWith(failedRun);
  });

  it("RunsDrawer shows the empty state when nothing ran yet", async () => {
    getJSON.mockResolvedValue({ runs: [], count: 0 });
    render(withUI(<RunsDrawer name="x" onReplay={vi.fn()} onError={vi.fn()} />));
    await waitFor(() => expect(screen.getByText(/No runs yet/)).toBeTruthy());
  });
});

describe("CopilotPanel", () => {
  it("posts the description and hands the drafted graph back", async () => {
    const drafted: Wf = {
      name: "status-check",
      description: "drafted",
      nodes: [
        { id: "start", type: "trigger", config: { kind: "cron", daily_at: "09:00" } },
        { id: "fetch", type: "http", config: { method: "GET", url: "https://x" } },
      ],
      edges: [{ from: "start", to: "fetch" }],
    };
    postJSON.mockResolvedValue({ workflow: drafted });
    const onDraft = vi.fn();
    const onError = vi.fn();
    render(withUI(<CopilotPanel name="status-check" onDraft={onDraft} onError={onError} />));
    fireEvent.change(screen.getByLabelText("Copilot description"), {
      target: { value: "check the status page every morning" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Draft with copilot" }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/workflows/draft", {
        description: "check the status page every morning",
        name: "status-check",
      }),
    );
    await waitFor(() => expect(onDraft).toHaveBeenCalledWith(drafted));
    expect(onError).not.toHaveBeenCalled();
  });

  it("refuses an empty description without calling the daemon", () => {
    const onDraft = vi.fn();
    const onError = vi.fn();
    render(withUI(<CopilotPanel name="x" onDraft={onDraft} onError={onError} />));
    fireEvent.click(screen.getByRole("button", { name: "Draft with copilot" }));
    expect(postJSON).not.toHaveBeenCalled();
    expect(onError).toHaveBeenCalledWith("describe the workflow first");
    expect(onDraft).not.toHaveBeenCalled();
  });

  it("offers Refine when the canvas holds a real graph and posts it with the instruction", async () => {
    const graph: Wf = {
      name: "greeter",
      nodes: [
        { id: "start", type: "trigger" },
        { id: "greet", type: "transform", config: { template: "hi" } },
      ],
      edges: [{ from: "start", to: "greet" }],
    };
    const revised: Wf = { ...graph, nodes: [...graph.nodes, { id: "gate", type: "approval" }] };
    postJSON.mockResolvedValue({ workflow: revised });
    const onDraft = vi.fn();
    render(withUI(<CopilotPanel name="greeter" graph={graph} onDraft={onDraft} onError={vi.fn()} />));
    fireEvent.change(screen.getByLabelText("Copilot description"), {
      target: { value: "add an approval gate" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Refine with copilot" }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/workflows/refine", {
        workflow: graph,
        instruction: "add an approval gate",
      }),
    );
    await waitFor(() => expect(onDraft).toHaveBeenCalledWith(revised));
  });

  it("hides Refine on a trigger-only canvas", () => {
    const bare: Wf = { name: "x", nodes: [{ id: "start", type: "trigger" }] };
    render(withUI(<CopilotPanel name="x" graph={bare} onDraft={vi.fn()} onError={vi.fn()} />));
    expect(screen.queryByRole("button", { name: "Refine with copilot" })).toBeNull();
    expect(screen.getByRole("button", { name: "Draft with copilot" })).toBeTruthy();
  });

  it("surfaces a copilot failure as an error, not a draft", async () => {
    postJSON.mockRejectedValue(new Error("workflow draft: the graph has a cycle"));
    const onDraft = vi.fn();
    const onError = vi.fn();
    render(withUI(<CopilotPanel name="x" onDraft={onDraft} onError={onError} />));
    fireEvent.change(screen.getByLabelText("Copilot description"), { target: { value: "do a thing" } });
    fireEvent.click(screen.getByRole("button", { name: "Draft with copilot" }));
    await waitFor(() => expect(onError).toHaveBeenCalledWith("workflow draft: the graph has a cycle"));
    expect(onDraft).not.toHaveBeenCalled();
  });
});
