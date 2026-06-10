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

import { Workflows, toFlow, fromFlow, portsForNode, summarize, type Wf } from "@/views/Workflows";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

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
