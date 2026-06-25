// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  authHeaders: () => new Headers(),
  withToken: (p: string) => p,
}));

// Keep the voice session inert in tests — we only verify the view's chrome and
// controls render and toggle, not the audio loop (covered in voiceSession.test).
const start = vi.fn();
const stop = vi.fn();
vi.mock("@/lib/voiceSession", () => ({
  VoiceSession: class {
    start = start;
    stop = stop;
  },
  createBrowserVoiceIO: () => ({}),
}));

import { Voice } from "@/views/Voice";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  getJSON.mockResolvedValue({ profiles: [{ slug: "researcher", name: "Researcher" }, { slug: "ops" }] });
  start.mockReset();
  stop.mockReset();
  localStorage.clear();
});

describe("Voice view", () => {
  it("renders the header, orb prompt, and start control", () => {
    render(withUI(<Voice />));
    expect(screen.getByRole("heading", { name: "Voice" })).toBeTruthy();
    expect(screen.getByText(/hands-free conversation/i)).toBeTruthy();
    expect(screen.getByRole("button", { name: /start talking/i })).toBeTruthy();
  });

  it("loads the roster into the agent picker", async () => {
    render(withUI(<Voice />));
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/agents"));
    await waitFor(() => expect(screen.getByText("Researcher")).toBeTruthy());
  });

  it("starts a session and swaps to a Stop control", () => {
    render(withUI(<Voice />));
    fireEvent.click(screen.getByRole("button", { name: /start talking/i }));
    expect(start).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: /stop/i })).toBeTruthy();
  });

  it("persists the wake-word toggle", () => {
    render(withUI(<Voice />));
    const sw = screen.getByRole("switch");
    expect(sw.getAttribute("aria-checked")).toBe("false");
    fireEvent.click(sw);
    expect(sw.getAttribute("aria-checked")).toBe("true");
    expect(localStorage.getItem("agezt.voice.wake")).toBe("1");
  });
});
