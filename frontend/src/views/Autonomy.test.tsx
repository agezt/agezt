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

import { PulseControl } from "@/views/Autonomy";
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

  it("shows running status with beats + cadence and a Pause button", async () => {
    getJSON.mockResolvedValue({ enabled: true, running: true, paused: false, beats: 12, cadence_ms: 30000, observers: 3 });
    render(withUI(<PulseControl />));
    await waitFor(() => expect(screen.getByText("running")).toBeTruthy());
    expect(screen.getByText(/12 beats · every 30s · 3 observers/)).toBeTruthy();
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
});
