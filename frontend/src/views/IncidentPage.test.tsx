// @vitest-environment jsdom
// IncidentPage after the declutter law: facts render once as visuals (tone
// chips + one raw "details" KeyValue per surface/row); every operator action
// stays inline; the prose re-narration layer (repair-ops sentence, selected-node
// card, passport lines) is gone.
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import {
  render,
  screen,
  cleanup,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postAction = vi.fn();
const postJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
}));
const subscribe = vi.fn(() => () => {});
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ events: [], connected: true, subscribe }),
}));

import { IncidentPage } from "@/views/IncidentPage";
import { UIProvider } from "@/components/ui/feedback";

function withUI(node: ReactNode) {
  return <UIProvider>{node}</UIProvider>;
}

const failedItem = {
  seq: 8,
  kind: "info",
  subject: "doctor.auto_repair",
  category: "doctor",
  title: "a config repair failed",
  detail: "builder · repair failed",
  agent: "builder",
  root_agent: "builder",
  incident_id: "root-1",
  root_incident_id: "root-1",
  chain_depth: 0,
  phase: "failed",
  ts_unix_ms: 40,
  correlation_id: "corr-1",
};

const exhaustedItem = {
  seq: 9,
  kind: "info",
  subject: "doctor.auto_repair",
  category: "doctor",
  title: "routing chain exhausted",
  detail: "builder · forced chain exhausted",
  agent: "builder",
  root_agent: "builder",
  incident_id: "root-1",
  root_incident_id: "root-1",
  phase: "routing_force_exhausted_detected",
  routing_task_type: "chat",
  routing_task_model_chain: ["m1", "m2"],
  ts_unix_ms: 50,
};

function mockAPI({
  items = [failedItem] as unknown[],
  events = [] as unknown[],
  profiles = [{ slug: "builder", name: "Builder", enabled: true }] as unknown[],
} = {}) {
  getJSON.mockImplementation((url: string) => {
    if (url === "/api/autonomy") return Promise.resolve({ items });
    if (url === "/api/journal") return Promise.resolve({ events });
    if (url === "/api/agents") return Promise.resolve({ profiles });
    if (url === "/api/agents/escalations") return Promise.resolve({ escalations: [] });
    return Promise.resolve({});
  });
}

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postAction.mockReset();
  postJSON.mockReset();
  subscribe.mockClear();
  postAction.mockResolvedValue({ ok: true });
  postJSON.mockResolvedValue({ ok: true });
  location.hash = "";
});

describe("IncidentPage", () => {
  it("leads with a glance band (ops tone chip + phase badges) and folds ALL raw identifiers into one details KeyValue", async () => {
    mockAPI();
    render(withUI(<IncidentPage incidentId="root-1" onNavigate={() => {}} />));
    await screen.findAllByText("a config repair failed");
    // Glance layer: tone chips, no sentences.
    expect(screen.getAllByText("needs owner").length).toBeGreaterThan(0);
    expect(screen.getAllByText("failed").length).toBeGreaterThan(0);
    // The single raw escape hatch: identifiers live in the details KeyValue.
    expect(screen.getByText("root incident")).toBeTruthy();
    expect(screen.getAllByText("root-1").length).toBeGreaterThan(0);
    expect(screen.getByText("hop")).toBeTruthy();
    expect(screen.getByText("corr-1")).toBeTruthy();
    // Deleted prose layer stays deleted.
    expect(screen.queryByText("repair ops")).toBeNull();
    expect(screen.queryByText("Selected node")).toBeNull();
    expect(screen.queryByText(/incident root agent/)).toBeNull();
    expect(screen.queryByText(/current config issue/)).toBeNull();
  });

  it("keeps every operator action inline: nav, doctor rerun with incident lineage, pause, retire, wake", async () => {
    mockAPI();
    render(withUI(<IncidentPage incidentId="root-1" onNavigate={() => {}} />));
    await screen.findByRole("button", { name: /Root agent/ });
    expect(screen.getByRole("button", { name: /Pause/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /Retire/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /Wake root/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /Repair console/ })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Doctor rerun/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/agents/repair",
        expect.objectContaining({
          ref: "builder",
          incident_id: "root-1",
          root_incident_id: "root-1",
        }),
      ),
    );
    // Root agent nav deep-links to the agent page.
    fireEvent.click(screen.getByRole("button", { name: /Root agent/ }));
    expect(location.hash).toContain("agent/builder");
  });

  it("renders resolution history as chips + actions with the raw stored fields folded per row", async () => {
    mockAPI({
      events: [
        {
          id: "ev1",
          kind: "info",
          subject: "agent.resolve",
          ts_unix_ms: 60,
          payload: {
            phase: "completed",
            resolution: "delegated",
            resolution_summary: "handed to helper",
            delegate_to: "helper",
            incident_id: "root-1",
            root_incident_id: "root-1",
          },
        },
      ],
    });
    render(withUI(<IncidentPage incidentId="root-1" onNavigate={() => {}} />));
    await screen.findByText("Resolution history");
    // Glance chip: what the operator decided.
    expect((await screen.findAllByText("delegated")).length).toBeGreaterThan(0);
    // Act layer: delegate nav button stays.
    expect(screen.getByRole("button", { name: /Delegate/ })).toBeTruthy();
    // Raw stored fields fold into the row's details KeyValue.
    expect(screen.getByText("summary")).toBeTruthy();
    expect(screen.getByText("handed to helper")).toBeTruthy();
    expect(screen.getByText("helper")).toBeTruthy();
    // The old " · "-joined narration line is gone.
    expect(screen.queryByText(/handed to helper · delegate helper/)).toBeNull();
  });

  it("gates exhausted-routing incidents behind the amber policy chip set and resolves via delegate with validation", async () => {
    mockAPI({
      items: [failedItem, exhaustedItem],
      profiles: [
        { slug: "builder", name: "Builder", enabled: true },
        { slug: "helper", name: "Helper", enabled: true },
      ],
    });
    render(withUI(<IncidentPage incidentId="root-1" onNavigate={() => {}} />));
    await screen.findByText("Forced-chain exhaustion");
    // Allowed resolutions render as chips, not sentences.
    for (const allowed of ["paused", "retired", "force_chain"]) {
      expect(screen.getAllByText(allowed).length).toBeGreaterThan(0);
    }
    // The policy summary sentence was demoted to a tooltip.
    expect(screen.queryByText(/owner-forced chain exhausted/)).toBeNull();
    // Exhausted chain surfaces once, in the surface details KeyValue.
    expect(screen.getByText("m1 → m2")).toBeTruthy();
    // Resolve flow: open the modal, invalid target explains itself, valid target posts.
    fireEvent.click(screen.getByRole("button", { name: /Resolve incident/ }));
    const input = await screen.findByPlaceholderText("delegate target slug");
    fireEvent.change(input, { target: { value: "builder" } });
    expect(
      await screen.findByText("delegate target cannot be the root agent"),
    ).toBeTruthy();
    fireEvent.change(input, { target: { value: "helper" } });
    expect(
      await screen.findByText("delegates incident ownership to helper"),
    ).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Delegate now/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/agents/resolve",
        expect.objectContaining({
          ref: "builder",
          resolution: "delegated",
          delegate_to: "helper",
          root_incident_id: "root-1",
        }),
      ),
    );
  });

  it("shows the empty state when no tree matches the incident id", async () => {
    mockAPI({ items: [] });
    render(withUI(<IncidentPage incidentId="ghost-9" onNavigate={() => {}} />));
    expect(await screen.findByText(/No incident/)).toBeTruthy();
  });
});
