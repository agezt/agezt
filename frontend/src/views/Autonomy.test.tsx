// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

import { PulseControl, cadenceLabel } from "@/views/Autonomy";
import { UIProvider } from "@/components/ui/feedback";

function withUI(node: ReactNode) {
  return <UIProvider>{node}</UIProvider>;
}

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postAction.mockReset();
  postAction.mockResolvedValue({ ok: true });
});

describe("PulseControl", () => {
  it("renders nothing meaningful when pulse is disabled", async () => {
    getJSON.mockResolvedValue({ enabled: false });
    render(withUI(<PulseControl />));
    await waitFor(() => expect(screen.getByText(/Pulse is disabled/)).toBeTruthy());
  });

  it("shows running status with beats + observers, a cadence selector and a Pause button", async () => {
    getJSON.mockResolvedValue({ enabled: true, running: true, paused: false, beats: 12, cadence_ms: 30000, observers: ["self:health", "system:disk", "probe:ci"] });
    render(withUI(<PulseControl />));
    await waitFor(() => expect(screen.getByText("running")).toBeTruthy());
    expect(screen.getByText(/12 beats · 3 observers/)).toBeTruthy();
    // Cadence moved into a live selector (M757); 30s is a preset → selected.
    expect((screen.getByLabelText("Heartbeat cadence") as HTMLSelectElement).value).toBe("30");
    expect(screen.getByRole("button", { name: /Pause/ })).toBeTruthy();
  });

  it("pauses via /api/pulse/pause and re-reads status", async () => {
    getJSON
      .mockResolvedValueOnce({ enabled: true, paused: false, beats: 5, cadence_ms: 60000 })
      .mockResolvedValue({ enabled: true, paused: true, beats: 5, cadence_ms: 60000 });
    render(withUI(<PulseControl />));
    await waitFor(() => expect(screen.getByRole("button", { name: /Pause/ })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /Pause/ }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/pulse/pause", {}));
    await waitFor(() => expect(screen.getByText("paused")).toBeTruthy());
  });

  it("resumes via /api/pulse/resume when paused", async () => {
    getJSON.mockResolvedValue({ enabled: true, paused: true, beats: 0, cadence_ms: 30000 });
    render(withUI(<PulseControl />));
    await waitFor(() => expect(screen.getByRole("button", { name: /Resume/ })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /Resume/ }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/pulse/resume", {}));
  });

  it("triggers an on-demand heartbeat via /api/pulse/beat (M756)", async () => {
    getJSON.mockResolvedValue({ enabled: true, paused: false, beats: 7, cadence_ms: 30000 });
    render(withUI(<PulseControl />));
    await waitFor(() => expect(screen.getByRole("button", { name: /Beat now/ })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /Beat now/ }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/pulse/beat", {}));
  });

  it("offers Beat now even while paused (explicit override)", async () => {
    getJSON.mockResolvedValue({ enabled: true, paused: true, beats: 0, cadence_ms: 30000 });
    render(withUI(<PulseControl />));
    await waitFor(() => expect(screen.getByRole("button", { name: /Beat now/ })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /Beat now/ }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/pulse/beat", {}));
  });

  it("changes the cadence live via /api/pulse/cadence (M757)", async () => {
    getJSON.mockResolvedValue({ enabled: true, paused: false, beats: 4, cadence_ms: 60000 });
    render(withUI(<PulseControl />));
    const sel = await screen.findByLabelText("Heartbeat cadence");
    // Current 60s maps to the "1m" preset being selected.
    expect((sel as HTMLSelectElement).value).toBe("60");
    fireEvent.change(sel, { target: { value: "300" } });
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/pulse/cadence", { seconds: "300" }));
  });

  it("shows a non-preset cadence as the current option", async () => {
    getJSON.mockResolvedValue({ enabled: true, paused: false, beats: 0, cadence_ms: 45000 }); // 45s, not a preset
    render(withUI(<PulseControl />));
    const sel = (await screen.findByLabelText("Heartbeat cadence")) as HTMLSelectElement;
    expect(sel.value).toBe(""); // the synthetic "current" option
    expect(screen.getByText(/45s \(current\)/)).toBeTruthy();
  });

  it("changes the proactivity dial live via /api/pulse/dial (M758)", async () => {
    getJSON.mockResolvedValue({ enabled: true, paused: false, beats: 1, cadence_ms: 60000, dial: "balanced" });
    render(withUI(<PulseControl />));
    const sel = (await screen.findByLabelText("Proactivity dial")) as HTMLSelectElement;
    expect(sel.value).toBe("balanced");
    fireEvent.change(sel, { target: { value: "chatty" } });
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/pulse/dial", { dial: "chatty" }));
  });

  it("defaults the dial selector to balanced when status omits it", async () => {
    getJSON.mockResolvedValue({ enabled: true, paused: false, beats: 0, cadence_ms: 60000 });
    render(withUI(<PulseControl />));
    const sel = (await screen.findByLabelText("Proactivity dial")) as HTMLSelectElement;
    expect(sel.value).toBe("balanced");
  });

  it("adds a disk watch via /api/pulse/watch (M767)", async () => {
    postAction.mockResolvedValue({ added: true, observer: "system:disk" });
    getJSON.mockResolvedValue({ enabled: true, paused: false, beats: 2, cadence_ms: 60000 });
    render(withUI(<PulseControl />));
    // The form is collapsed until the "a disk" toggle is clicked.
    expect(screen.queryByLabelText("Watch disk path")).toBeNull();
    fireEvent.click(await screen.findByRole("button", { name: /a disk/ }));
    fireEvent.change(screen.getByLabelText("Watch disk path"), { target: { value: "/data" } });
    fireEvent.change(screen.getByLabelText("Watch min percent free"), { target: { value: "15" } });
    fireEvent.click(screen.getByRole("button", { name: /Watch/ }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/pulse/watch", { path: "/data", min_pct: "15" }));
  });

  it("adds a command-probe watch via /api/pulse/probe (M768)", async () => {
    postAction.mockResolvedValue({ added: true, observer: "probe:ci" });
    getJSON.mockResolvedValue({ enabled: true, paused: false, beats: 2, cadence_ms: 60000 });
    render(withUI(<PulseControl />));
    fireEvent.click(await screen.findByRole("button", { name: /a command/ }));
    fireEvent.change(screen.getByLabelText("Probe name"), { target: { value: "ci" } });
    fireEvent.change(screen.getByLabelText("Probe command"), { target: { value: "make test" } });
    fireEvent.click(screen.getByRole("button", { name: /Watch/ }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/pulse/probe", { name: "ci", command: "make test" }));
  });

  it("lists observers and removes a runtime-added watch via /api/pulse/unwatch (M769)", async () => {
    getJSON.mockResolvedValue({
      enabled: true, paused: false, beats: 2, cadence_ms: 60000,
      observers: ["self:health", "probe:ci"], removable: ["probe:ci"],
    });
    render(withUI(<PulseControl />));
    // Both observers are listed; only the runtime-added one offers a remove control.
    await waitFor(() => expect(screen.getByText("self:health")).toBeTruthy());
    expect(screen.getByText("probe:ci")).toBeTruthy();
    expect(screen.queryByLabelText("Stop watching self:health")).toBeNull();
    fireEvent.click(screen.getByLabelText("Stop watching probe:ci"));
    // Confirm the modal, then it posts the unwatch.
    fireEvent.click(await screen.findByRole("button", { name: "Stop watching" }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/pulse/unwatch", { name: "probe:ci" }));
  });

  it("sets quiet hours via /api/pulse/quiet (M770)", async () => {
    getJSON.mockResolvedValue({ enabled: true, paused: false, beats: 1, cadence_ms: 60000, quiet: { enabled: false } });
    render(withUI(<PulseControl />));
    const input = await screen.findByLabelText("Quiet hours window");
    fireEvent.change(input, { target: { value: "22-7" } });
    fireEvent.click(screen.getByRole("button", { name: "Set" }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/pulse/quiet", { hours: "22-7" }));
  });

  it("shows the active quiet window and clears it (M770)", async () => {
    getJSON.mockResolvedValue({ enabled: true, paused: false, beats: 1, cadence_ms: 60000, quiet: { enabled: true, start: 22, end: 7, spec: "22-7" } });
    render(withUI(<PulseControl />));
    await waitFor(() => expect(screen.getByText("22:00–07:00")).toBeTruthy());
    fireEvent.click(screen.getByLabelText("Clear quiet hours"));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/pulse/quiet", { hours: "" }));
  });

  it("shows a Flush digest button only when items are held, and flushes them (M761)", async () => {
    postAction.mockResolvedValue({ flushed: 3 });
    getJSON.mockResolvedValue({ enabled: true, paused: false, beats: 5, cadence_ms: 60000, digest_pending: 3 });
    render(withUI(<PulseControl />));
    const btn = await screen.findByRole("button", { name: /Flush digest \(3\)/ });
    fireEvent.click(btn);
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/pulse/flush", {}));
  });

  it("hides the Flush digest button when the digest is empty", async () => {
    getJSON.mockResolvedValue({ enabled: true, paused: false, beats: 5, cadence_ms: 60000, digest_pending: 0 });
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
