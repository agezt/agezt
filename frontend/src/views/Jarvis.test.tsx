// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
  authHeaders: (h?: HeadersInit) => new Headers(h),
}));

import { Jarvis } from "@/views/Jarvis";
import { UIProvider } from "@/components/ui/feedback";

function withUI(node: ReactNode) {
  return <UIProvider>{node}</UIProvider>;
}

const PULSE_ACT = {
  running: true,
  paused: false,
  beats: 42,
  observers: ["self:health"],
  cadence_ms: 60000,
  dial: "balanced",
  initiative: "act",
};
const PROFILE_RECORDS = {
  records: [
    { id: "p1", subject: "operator profile: expertise", content: "Go and React.", type: "PREFERENCE" },
    { id: "p2", subject: "operator profile: communication style", content: "Terse, direct.", type: "PREFERENCE" },
    { id: "m1", subject: "kubernetes", content: "frankfurt", type: "FACT" },
  ],
};

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postAction.mockReset();
  // Voice probe: 501 → server TTS not configured → browser voice.
  vi.stubGlobal("fetch", vi.fn(async () => ({ status: 501, ok: false }) as Response));
});

describe("Jarvis presence view", () => {
  it("lights up all three pillars and reports the live count", async () => {
    getJSON.mockImplementation((path: string) =>
      path === "/api/pulse"
        ? Promise.resolve(PULSE_ACT)
        : Promise.resolve(PROFILE_RECORDS),
    );
    render(withUI(<Jarvis />));

    // Initiative pillar reads the live mode from /api/pulse.
    await waitFor(() => expect(screen.getByText("Acting on its own")).toBeTruthy());
    // Profile pillar counts only the "operator profile:" records (2 of 3).
    expect(screen.getByText(/Knows 2 things about you/)).toBeTruthy();
    expect(screen.getByText(/expertise/)).toBeTruthy();
    // Voice pillar fell back to the browser voice after the 501 probe.
    await waitFor(() => expect(screen.getByText("Browser voice ready")).toBeTruthy());
    // Presence meter: voice + will + profile = 3 of 3.
    expect(screen.getByText(/3 of 3/)).toBeTruthy();
  });

  it("shows the dormant headlines and a lower count when pillars are off", async () => {
    getJSON.mockImplementation((path: string) =>
      path === "/api/pulse"
        ? Promise.resolve({ ...PULSE_ACT, initiative: "off", beats: 0 })
        : Promise.resolve({ records: [] }),
    );
    render(withUI(<Jarvis />));

    await waitFor(() => expect(screen.getByText("Observing only")).toBeTruthy());
    expect(screen.getByText("Still learning you")).toBeTruthy();
    // Only voice is live (browser fallback) → 1 of 3.
    await waitFor(() => expect(screen.getByText(/1 of 3/)).toBeTruthy());
  });

  it("rebuilds the operator profile on demand", async () => {
    getJSON.mockImplementation((path: string) =>
      path === "/api/pulse" ? Promise.resolve(PULSE_ACT) : Promise.resolve(PROFILE_RECORDS),
    );
    postAction.mockResolvedValue({ facets_written: 2, input_records: 5 });
    render(withUI(<Jarvis />));

    await waitFor(() => expect(screen.getByText(/Knows 2 things/)).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /rebuild/i }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/profile/rebuild", {}));
  });

  it("lists pending asks and approves one through the act path (M1001)", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/pulse") return Promise.resolve({ ...PULSE_ACT, initiative: "ask" });
      if (path === "/api/pulse/asks")
        return Promise.resolve({ asks: [{ issue_key: "ci-1", summary: "CI failed on main", source: "probe:ci" }] });
      return Promise.resolve(PROFILE_RECORDS);
    });
    postAction.mockResolvedValue({ resolved: true, approved: true, acted: true });
    render(withUI(<Jarvis />));

    await waitFor(() => expect(screen.getByText("CI failed on main")).toBeTruthy());
    expect(screen.getByText(/1 waiting on you/)).toBeTruthy();

    fireEvent.click(screen.getByTitle(/approve/i));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/pulse/asks/resolve", { issue_key: "ci-1", approve: "true" }),
    );
  });
});
