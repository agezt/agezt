// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postAction = vi.fn();
const getVoiceReadiness = vi.fn();
const goToView = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
  authHeaders: (h?: HeadersInit) => new Headers(h),
}));
vi.mock("@/lib/voiceStatus", () => ({
  getVoiceReadiness: (...a: unknown[]) => getVoiceReadiness(...a),
}));
vi.mock("@/lib/nav", () => ({
  goToView: (...a: unknown[]) => goToView(...a),
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
  getVoiceReadiness.mockReset();
  getVoiceReadiness.mockResolvedValue({
    serverSTT: true,
    serverTTS: false,
    browserInput: true,
    browserTTS: true,
    canListen: true,
    canSpeak: true,
  });
  goToView.mockReset();
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
    // Server STT + browser TTS is a usable, explicitly degraded voice path.
    await waitFor(() => expect(screen.getByText("Voice ready")).toBeTruthy());
    // Presence meter: voice + will + profile = 3 of 3.
    expect(screen.getByText(/3 of 3/)).toBeTruthy();
  });

  it("reports a wired speech provider for both hearing and speaking", async () => {
    getJSON.mockImplementation((path: string) =>
      path === "/api/pulse" ? Promise.resolve(PULSE_ACT) : Promise.resolve(PROFILE_RECORDS),
    );
    getVoiceReadiness.mockResolvedValue({
      serverSTT: true,
      serverTTS: true,
      browserInput: true,
      browserTTS: true,
      canListen: true,
      canSpeak: true,
    });
    render(withUI(<Jarvis />));

    await waitFor(() => expect(screen.getByText("a speech provider")).toBeTruthy());
    expect(screen.getByText("a natural voice")).toBeTruthy();
    expect(screen.getByText(/Fully wired/)).toBeTruthy();
  });

  it("does not claim browser speech can replace a missing transcription provider", async () => {
    getJSON.mockImplementation((path: string) =>
      path === "/api/pulse" ? Promise.resolve(PULSE_ACT) : Promise.resolve({ records: [] }),
    );
    getVoiceReadiness.mockResolvedValue({
      serverSTT: false,
      serverTTS: false,
      browserInput: true,
      browserTTS: true,
      canListen: false,
      canSpeak: true,
    });
    render(withUI(<Jarvis />));

    await waitFor(() => expect(screen.getByText("Voice needs setup")).toBeTruthy());
    expect(screen.getByText("provider not configured")).toBeTruthy();
    expect(screen.getByText(/1 of 3/)).toBeTruthy();
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

  it("arms the disarmed initiative responder in one click", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/pulse") return Promise.resolve(PULSE_ACT);
      if (path === "/api/standing")
        return Promise.resolve({ orders: [{ id: "ord-1", slug: "guardian-initiative", name: "Guardian · Initiative", enabled: false }] });
      return Promise.resolve(PROFILE_RECORDS);
    });
    postAction.mockResolvedValue({ ok: true });
    render(withUI(<Jarvis />));

    await waitFor(() => expect(screen.getByText(/disarmed/)).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /arm/i }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/standing/enable", { id: "ord-1", enabled: "true" }),
    );
  });

  it("triggers an on-demand heartbeat with Think now", async () => {
    getJSON.mockImplementation((path: string) =>
      path === "/api/pulse" ? Promise.resolve(PULSE_ACT) : Promise.resolve(PROFILE_RECORDS),
    );
    postAction.mockResolvedValue({ triggered: true });
    render(withUI(<Jarvis />));

    await waitFor(() => expect(screen.getByText("Acting on its own")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /think now/i }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/pulse/beat", {}));
  });

  it("shows the recent-initiative feed with act/ask badges (M1003)", async () => {
    getJSON.mockImplementation((path: string, params?: Record<string, string>) => {
      if (path === "/api/pulse") return Promise.resolve(PULSE_ACT);
      if (path === "/api/journal" && params?.kind === "initiative.act")
        return Promise.resolve({
          events: [
            { id: "e1", subject: "pulse.initiative.act", ts_unix_ms: Date.now(), payload: { summary: "restarted stuck run", source: "self:health" } },
            { id: "e2", subject: "pulse.initiative.ask", ts_unix_ms: Date.now(), payload: { summary: "disk almost full", source: "probe:disk" } },
          ],
        });
      return Promise.resolve(PROFILE_RECORDS);
    });
    render(withUI(<Jarvis />));

    await waitFor(() => expect(screen.getByText("Recent initiative")).toBeTruthy());
    expect(screen.getByText("disk almost full")).toBeTruthy();
    expect(screen.getByText("restarted stuck run")).toBeTruthy();
    expect(screen.getByText("acted")).toBeTruthy();
    expect(screen.getByText("asked")).toBeTruthy();
  });

  it("navigates from all three pillar actions and refreshes on demand", async () => {
    getJSON.mockImplementation((path: string) =>
      path === "/api/pulse" ? Promise.resolve(PULSE_ACT) : Promise.resolve(PROFILE_RECORDS),
    );
    render(withUI(<Jarvis />));
    await waitFor(() => expect(screen.getByText("Acting on its own")).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: /start talking/i }));
    fireEvent.click(screen.getByRole("button", { name: /tune autonomy/i }));
    fireEvent.click(screen.getByRole("button", { name: /manage profile/i }));
    expect(goToView.mock.calls).toEqual([["voice"], ["autonomy"], ["memory"]]);

    const calls = getJSON.mock.calls.length;
    fireEvent.click(screen.getByTitle("Refresh"));
    await waitFor(() => expect(getJSON.mock.calls.length).toBeGreaterThan(calls));
  });

  it("degrades cleanly when every status source and speech readiness reject", async () => {
    getJSON.mockRejectedValue(new Error("offline"));
    getVoiceReadiness.mockRejectedValue(new Error("offline"));
    render(withUI(<Jarvis />));

    await waitFor(() => expect(getVoiceReadiness).toHaveBeenCalled());
    expect(screen.getAllByText("…").length).toBeGreaterThan(0);
    expect(screen.getByText("Checking voice…")).toBeTruthy();
    expect(screen.getByText(/0 of 3/)).toBeTruthy();
  });

  it("shows paused and fully unavailable voice states with responder fallbacks", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/pulse") return Promise.resolve({ paused: true, running: true });
      if (path === "/api/standing")
        return Promise.resolve({ orders: [{}, { name: "Initiative fallback", enabled: true }] });
      if (path === "/api/memory")
        return Promise.resolve({ records: [{ id: "p", content: "Known", type: "PREFERENCE" }] });
      return Promise.resolve({});
    });
    getVoiceReadiness.mockResolvedValue({
      serverSTT: false,
      serverTTS: false,
      browserInput: false,
      browserTTS: false,
      canListen: false,
      canSpeak: false,
    });
    render(withUI(<Jarvis />));

    await waitFor(() => expect(screen.getByText("Heartbeat paused")).toBeTruthy());
    expect(screen.getByText("browser microphone unavailable")).toBeTruthy();
    expect(screen.getByText("unavailable")).toBeTruthy();
    expect(screen.getByText(/Responder armed/)).toBeTruthy();
  });

  it("does nothing when the matched responder has no id", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/pulse") return Promise.resolve(PULSE_ACT);
      if (path === "/api/standing")
        return Promise.resolve({ orders: [{ name: "Initiative fallback", enabled: false }] });
      return Promise.resolve(PROFILE_RECORDS);
    });
    render(withUI(<Jarvis />));
    fireEvent.click(await screen.findByRole("button", { name: /arm/i }));
    expect(postAction).not.toHaveBeenCalled();
  });

  it("dismisses asks and covers summary fallbacks", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/pulse") return Promise.resolve({ ...PULSE_ACT, initiative: "ask" });
      if (path === "/api/pulse/asks")
        return Promise.resolve({
          asks: [
            { issue_key: "source-only", source: "probe:disk" },
            { issue_key: "key-only" },
          ],
        });
      return Promise.resolve(PROFILE_RECORDS);
    });
    postAction.mockResolvedValue({ resolved: true });
    render(withUI(<Jarvis />));

    await waitFor(() => expect(screen.getByText("probe:disk")).toBeTruthy());
    expect(screen.getByText("key-only")).toBeTruthy();
    fireEvent.click(screen.getAllByTitle("Dismiss")[0]);
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/pulse/asks/resolve", {
        issue_key: "source-only",
        approve: "false",
      }),
    );
  });

  it("explains when an approved ask has no armed action", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/pulse") return Promise.resolve({ ...PULSE_ACT, initiative: "ask" });
      if (path === "/api/pulse/asks")
        return Promise.resolve({ asks: [{ issue_key: "ask-1", summary: "Approval needed" }] });
      if (path === "/api/memory") return Promise.resolve({});
      return Promise.resolve({});
    });
    postAction.mockResolvedValue({});
    render(withUI(<Jarvis />));

    fireEvent.click(await screen.findByTitle(/approve/i));
    expect(await screen.findByText(/enable the Initiative responder/)).toBeTruthy();
  });

  it("reports failures from every Jarvis action", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/pulse") return Promise.resolve({ ...PULSE_ACT, initiative: "ask" });
      if (path === "/api/pulse/asks")
        return Promise.resolve({ asks: [{ issue_key: "ask-1", summary: "Needs approval" }] });
      if (path === "/api/standing")
        return Promise.resolve({ orders: [{ id: "ord-1", slug: "guardian-initiative", enabled: false }] });
      return Promise.resolve(PROFILE_RECORDS);
    });
    postAction.mockRejectedValue(new Error("action failed"));
    render(withUI(<Jarvis />));
    await screen.findByText("Needs approval");

    fireEvent.click(screen.getByRole("button", { name: /arm/i }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/standing/enable", expect.anything()));
    fireEvent.click(screen.getByTitle(/approve/i));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/pulse/asks/resolve", expect.anything()));
    fireEvent.click(screen.getByRole("button", { name: /think now/i }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/pulse/beat", {}));
    fireEvent.click(screen.getByRole("button", { name: /rebuild/i }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/profile/rebuild", {}));
    expect(await screen.findAllByText("action failed")).toHaveLength(4);
  });

  it("reports all profile rebuild outcomes", async () => {
    getJSON.mockImplementation((path: string) =>
      path === "/api/pulse" ? Promise.resolve(PULSE_ACT) : Promise.resolve(PROFILE_RECORDS),
    );
    postAction
      .mockResolvedValueOnce({ facets_written: 1 })
      .mockResolvedValueOnce({ facets_written: 0 })
      .mockResolvedValueOnce({});
    render(withUI(<Jarvis />));
    const rebuild = await screen.findByRole("button", { name: /rebuild/i });

    fireEvent.click(rebuild);
    await screen.findByText("profile rebuilt: 1 facet learned");
    fireEvent.click(rebuild);
    await screen.findByText("nothing to learn from yet — give it some memory first");
    fireEvent.click(rebuild);
    await waitFor(() => expect(postAction).toHaveBeenCalledTimes(3));
  });

  it("uses singular profile copy for one learned facet", async () => {
    getJSON.mockImplementation((path: string) =>
      path === "/api/pulse"
        ? Promise.resolve(PULSE_ACT)
        : Promise.resolve({ records: [{ id: "one", subject: "operator profile: timezone", content: "UTC" }] }),
    );
    render(withUI(<Jarvis />));
    expect(await screen.findByText("Knows 1 thing about you")).toBeTruthy();
  });

  it("formats every recent-event age and payload fallback", async () => {
    const now = Date.now();
    getJSON.mockImplementation((path: string, params?: Record<string, string>) => {
      if (path === "/api/pulse") return Promise.resolve(PULSE_ACT);
      if (path === "/api/journal" && params?.kind === "initiative.act")
        return Promise.resolve({
          events: [
            {},
            { id: "day", ts_unix_ms: now - 2 * 86400_000, payload: { issue_key: "issue fallback" } },
            { id: "hour", ts_unix_ms: now - 2 * 3600_000, payload: { source: "source fallback" } },
            { id: "minute", ts_unix_ms: now - 2 * 60_000, payload: { summary: "minute event" } },
            { id: "now", subject: "initiative.ask", ts_unix_ms: now, payload: { reason: "reason only" } },
          ],
        });
      return Promise.resolve(PROFILE_RECORDS);
    });
    render(withUI(<Jarvis />));

    await screen.findByText("minute event");
    expect(screen.getByText("just now")).toBeTruthy();
    expect(screen.getByText("2m")).toBeTruthy();
    expect(screen.getByText("2h")).toBeTruthy();
    expect(screen.getByText("2d")).toBeTruthy();
    expect(screen.getAllByText("an observation")).toHaveLength(2);
    expect(screen.getByText("issue fallback")).toBeTruthy();
    expect(screen.getAllByText("source fallback").length).toBeGreaterThan(0);
  });
});
