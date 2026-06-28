// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import {
  render,
  screen,
  cleanup,
  fireEvent,
  waitFor,
  within,
} from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));
const subscribe = vi.fn(() => () => {});
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ events: [], connected: true, subscribe }),
}));

import { Autonomy, PulseControl, cadenceLabel } from "@/views/Autonomy";
import { UIProvider } from "@/components/ui/feedback";

function withUI(node: ReactNode) {
  return <UIProvider>{node}</UIProvider>;
}

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postAction.mockReset();
  subscribe.mockClear();
  postAction.mockResolvedValue({ ok: true });
});

describe("Autonomy", () => {
  it("renders doctor-category autonomy items from the curated feed", async () => {
    getJSON.mockImplementation((url: string) => {
      if (url === "/api/autonomy") {
        return Promise.resolve({
          items: [
            {
              seq: 7,
              kind: "info",
              subject: "doctor.auto_repair",
              category: "doctor",
              title: "a doctor run repaired an agent",
              detail: "builder · applied model, config_overrides",
              agent: "builder",
              root_agent: "builder",
              incident_id: "root-1",
              root_incident_id: "root-1",
              phase: "completed",
              mode: "degraded",
            },
          ],
          count: 1,
        });
      }
      if (url === "/api/pulse") return Promise.resolve({ enabled: false });
      return Promise.resolve({});
    });
    render(withUI(<Autonomy />));
    await waitFor(() =>
    expect(screen.getAllByText("a doctor run repaired an agent").length).toBeGreaterThan(0),
    );
    expect(screen.getAllByText("doctor").length).toBeGreaterThan(0);
    expect(screen.getAllByText("repaired").length).toBeGreaterThan(0);
    expect(
      screen.getByText("builder · applied model, config_overrides"),
    ).toBeTruthy();
  });

  it("renders repair incident trees with root + hop lineage", async () => {
    getJSON.mockImplementation((url: string) => {
      if (url === "/api/autonomy") {
        return Promise.resolve({
          items: [
            {
              seq: 9,
              kind: "info",
              subject: "agent.wake",
              category: "doctor",
              title: "an operator wake completed",
              detail: "infra-lead · operator wake completed",
              root_agent: "builder",
              incident_id: "child-1",
              root_incident_id: "root-1",
              parent_incident_id: "root-1",
              chain_depth: 1,
              phase: "completed",
              delegated_by: "lead",
              delegate_to: "infra-lead",
              ts_unix_ms: 50,
            },
            {
              seq: 8,
              kind: "info",
              category: "doctor",
              title: "a config repair failed",
              detail: "builder · repair failed",
              root_agent: "builder",
              incident_id: "root-1",
              root_incident_id: "root-1",
              chain_depth: 0,
              phase: "failed",
              ts_unix_ms: 40,
            },
          ],
          count: 1,
        });
      }
      if (url === "/api/pulse") return Promise.resolve({ enabled: false });
      return Promise.resolve({});
    });
    render(withUI(<Autonomy />));
    await waitFor(() =>
      expect(screen.getByText("Repair incident trees")).toBeTruthy(),
    );
    expect(screen.getAllByText("builder").length).toBeGreaterThan(0);
    expect(screen.getByText("infra-lead")).toBeTruthy();
    expect(screen.getAllByText("operator").length).toBeGreaterThan(0);
    expect(screen.getAllByText("completed").length).toBeGreaterThan(0);
    expect(screen.getAllByText("failed").length).toBeGreaterThan(0);
    expect(
      screen.getByText(
        /delegated by lead · root builder · hop 1 · to infra-lead/,
      ),
    ).toBeTruthy();
  });
});

describe("PulseControl", () => {
  it("renders nothing meaningful when pulse is disabled", async () => {
    getJSON.mockResolvedValue({ enabled: false });
    render(withUI(<PulseControl />));
    await waitFor(() =>
      expect(screen.getByText(/Pulse is disabled/)).toBeTruthy(),
    );
  });

  it("shows running status with beats + observers, a cadence selector and a Pause button", async () => {
    getJSON.mockResolvedValue({
      enabled: true,
      running: true,
      paused: false,
      beats: 12,
      cadence_ms: 30000,
      observers: ["self:health", "system:disk", "probe:ci"],
    });
    render(withUI(<PulseControl />));
    await waitFor(() => expect(screen.getByText("running")).toBeTruthy());
    expect(screen.getByText(/12 beats · 3 observers/)).toBeTruthy();
    // Cadence is a segmented live control (M757); 30s is the pressed preset.
    expect(within(screen.getByLabelText("Heartbeat cadence")).getByRole("button", { name: "30s" }).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByRole("button", { name: /Pause/ })).toBeTruthy();
  });

  it("pauses via /api/pulse/pause and re-reads status", async () => {
    getJSON
      .mockResolvedValueOnce({
        enabled: true,
        paused: false,
        beats: 5,
        cadence_ms: 60000,
      })
      .mockResolvedValue({
        enabled: true,
        paused: true,
        beats: 5,
        cadence_ms: 60000,
      });
    render(withUI(<PulseControl />));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: /Pause/ })).toBeTruthy(),
    );
    fireEvent.click(screen.getByRole("button", { name: /Pause/ }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/pulse/pause", {}),
    );
    await waitFor(() => expect(screen.getByText("paused")).toBeTruthy());
  });

  it("resumes via /api/pulse/resume when paused", async () => {
    getJSON.mockResolvedValue({
      enabled: true,
      paused: true,
      beats: 0,
      cadence_ms: 30000,
    });
    render(withUI(<PulseControl />));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: /Resume/ })).toBeTruthy(),
    );
    fireEvent.click(screen.getByRole("button", { name: /Resume/ }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/pulse/resume", {}),
    );
  });

  it("triggers an on-demand heartbeat via /api/pulse/beat (M756)", async () => {
    getJSON.mockResolvedValue({
      enabled: true,
      paused: false,
      beats: 7,
      cadence_ms: 30000,
    });
    render(withUI(<PulseControl />));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: /Beat now/ })).toBeTruthy(),
    );
    fireEvent.click(screen.getByRole("button", { name: /Beat now/ }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/pulse/beat", {}),
    );
  });

  it("offers Beat now even while paused (explicit override)", async () => {
    getJSON.mockResolvedValue({
      enabled: true,
      paused: true,
      beats: 0,
      cadence_ms: 30000,
    });
    render(withUI(<PulseControl />));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: /Beat now/ })).toBeTruthy(),
    );
    fireEvent.click(screen.getByRole("button", { name: /Beat now/ }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/pulse/beat", {}),
    );
  });

  it("changes the cadence live via /api/pulse/cadence (M757)", async () => {
    getJSON.mockResolvedValue({
      enabled: true,
      paused: false,
      beats: 4,
      cadence_ms: 60000,
    });
    render(withUI(<PulseControl />));
    const group = await screen.findByLabelText("Heartbeat cadence");
    // Current 60s maps to the "1m" preset being selected.
    expect(within(group).getByRole("button", { name: "1m" }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(within(group).getByRole("button", { name: "5m" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/pulse/cadence", {
        seconds: "300",
      }),
    );
  });

  it("shows a non-preset cadence as the current option", async () => {
    getJSON.mockResolvedValue({
      enabled: true,
      paused: false,
      beats: 0,
      cadence_ms: 45000,
    }); // 45s, not a preset
    render(withUI(<PulseControl />));
    const group = await screen.findByLabelText("Heartbeat cadence");
    expect(within(group).getByText("45s")).toBeTruthy();
    expect(within(group).getAllByRole("button").every((btn) => btn.getAttribute("aria-pressed") === "false")).toBe(true);
  });

  it("changes the proactivity dial live via /api/pulse/dial (M758)", async () => {
    getJSON.mockResolvedValue({
      enabled: true,
      paused: false,
      beats: 1,
      cadence_ms: 60000,
      dial: "balanced",
    });
    render(withUI(<PulseControl />));
    const group = await screen.findByLabelText("Proactivity dial");
    expect(within(group).getByRole("button", { name: /balanced/ }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(within(group).getByRole("button", { name: /chatty/ }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/pulse/dial", {
        dial: "chatty",
      }),
    );
  });

  it("defaults the dial selector to balanced when status omits it", async () => {
    getJSON.mockResolvedValue({
      enabled: true,
      paused: false,
      beats: 0,
      cadence_ms: 60000,
    });
    render(withUI(<PulseControl />));
    const group = await screen.findByLabelText("Proactivity dial");
    expect(within(group).getByRole("button", { name: /balanced/ }).getAttribute("aria-pressed")).toBe("true");
  });

  it("adds a disk watch via /api/pulse/watch (M767)", async () => {
    postAction.mockResolvedValue({ added: true, observer: "system:disk" });
    getJSON.mockResolvedValue({
      enabled: true,
      paused: false,
      beats: 2,
      cadence_ms: 60000,
    });
    render(withUI(<PulseControl />));
    // The form stays off the main card until the disk modal is opened.
    expect(screen.queryByLabelText("Watch disk path")).toBeNull();
    fireEvent.click(await screen.findByRole("button", { name: /a disk/ }));
    expect(screen.getByRole("dialog", { name: "Watch disk" })).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Watch disk path"), {
      target: { value: "/data" },
    });
    fireEvent.change(screen.getByLabelText("Watch min percent free"), {
      target: { value: "15" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Watch/ }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/pulse/watch", {
        path: "/data",
        min_pct: "15",
      }),
    );
  });

  it("adds a command-probe watch via /api/pulse/probe (M768)", async () => {
    postAction.mockResolvedValue({ added: true, observer: "probe:ci" });
    getJSON.mockResolvedValue({
      enabled: true,
      paused: false,
      beats: 2,
      cadence_ms: 60000,
    });
    render(withUI(<PulseControl />));
    fireEvent.click(await screen.findByRole("button", { name: /a command/ }));
    expect(screen.getByRole("dialog", { name: "Watch command" })).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Probe name"), {
      target: { value: "ci" },
    });
    fireEvent.change(screen.getByLabelText("Probe command"), {
      target: { value: "make test" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Watch/ }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/pulse/probe", {
        name: "ci",
        command: "make test",
      }),
    );
  });

  it("lists observers and removes a runtime-added watch via /api/pulse/unwatch (M769)", async () => {
    getJSON.mockResolvedValue({
      enabled: true,
      paused: false,
      beats: 2,
      cadence_ms: 60000,
      observers: ["self:health", "probe:ci"],
      removable: ["probe:ci"],
    });
    render(withUI(<PulseControl />));
    // Both observers are listed; only the runtime-added one offers a remove control.
    await waitFor(() => expect(screen.getByText("self:health")).toBeTruthy());
    expect(screen.getByText("probe:ci")).toBeTruthy();
    expect(screen.queryByLabelText("Stop watching self:health")).toBeNull();
    fireEvent.click(screen.getByLabelText("Stop watching probe:ci"));
    // Confirm the modal, then it posts the unwatch.
    fireEvent.click(
      await screen.findByRole("button", { name: "Stop watching" }),
    );
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/pulse/unwatch", {
        name: "probe:ci",
      }),
    );
  });

  it("sets quiet hours via /api/pulse/quiet (M770)", async () => {
    getJSON.mockResolvedValue({
      enabled: true,
      paused: false,
      beats: 1,
      cadence_ms: 60000,
      quiet: { enabled: false },
    });
    render(withUI(<PulseControl />));
    fireEvent.click(await screen.findByRole("button", { name: "Set" }));
    const dialog = screen.getByRole("dialog", { name: "Quiet hours" });
    expect(dialog).toBeTruthy();
    const input = within(dialog).getByLabelText("Quiet hours window");
    fireEvent.change(input, { target: { value: "22-7" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Set" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/pulse/quiet", {
        hours: "22-7",
      }),
    );
  });

  it("shows the active quiet window and clears it (M770)", async () => {
    getJSON.mockResolvedValue({
      enabled: true,
      paused: false,
      beats: 1,
      cadence_ms: 60000,
      quiet: { enabled: true, start: 22, end: 7, spec: "22-7" },
    });
    render(withUI(<PulseControl />));
    await waitFor(() => expect(screen.getByText("22:00–07:00")).toBeTruthy());
    fireEvent.click(screen.getByLabelText("Clear quiet hours"));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/pulse/quiet", {
        hours: "",
      }),
    );
  });

  it("shows a Flush digest button only when items are held, and flushes them (M761)", async () => {
    postAction.mockResolvedValue({ flushed: 3 });
    getJSON.mockResolvedValue({
      enabled: true,
      paused: false,
      beats: 5,
      cadence_ms: 60000,
      digest_pending: 3,
    });
    render(withUI(<PulseControl />));
    const btn = await screen.findByRole("button", {
      name: /Flush digest \(3\)/,
    });
    fireEvent.click(btn);
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/pulse/flush", {}),
    );
  });

  it("hides the Flush digest button when the digest is empty", async () => {
    getJSON.mockResolvedValue({
      enabled: true,
      paused: false,
      beats: 5,
      cadence_ms: 60000,
      digest_pending: 0,
    });
    render(withUI(<PulseControl />));
    await screen.findByLabelText("Proactivity dial");
    expect(screen.queryByRole("button", { name: /Flush digest/ })).toBeNull();
  });
});

describe("cadenceLabel (M757)", () => {
  it("formats seconds as compact intervals", () => {
    expect(cadenceLabel(10)).toBe("10s");
    expect(cadenceLabel(300)).toBe("5m");
    expect(cadenceLabel(3600)).toBe("1h");
  });
});
