// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor, within } from "@testing-library/react";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
}));

vi.mock("@/lib/events", () => ({
  useEvents: () => ({ subscribe: vi.fn(() => vi.fn()) }),
}));

vi.mock("@/components/FlightRecorder", () => ({
  FlightRecorder: () => <div data-testid="flight-recorder" />,
}));

import { Replay } from "@/views/Replay";

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  getJSON.mockImplementation((path: string, params?: Record<string, string>) => {
    if (path === "/api/runs") {
      return Promise.resolve({
        runs: [
          { correlation_id: "run-1", status: "running", intent: "Live deploy check" },
          { correlation_id: "run-2", status: "done", intent: "Past audit" },
        ],
      });
    }
    if (path === "/api/journal") return Promise.resolve({ events: [{ correlation_id: params?.correlation_id }] });
    return Promise.resolve({});
  });
});

describe("Replay", () => {
  it("selects runs from the compact run strip", async () => {
    render(<Replay />);
    const strip = await screen.findByRole("group", { name: "Replay run" });
    expect(within(strip).getByRole("button", { name: /Live deploy check/ }).getAttribute("aria-pressed")).toBe("true");

    fireEvent.click(within(strip).getByRole("button", { name: /Past audit/ }));
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/journal", { correlation_id: "run-2", limit: "1000" }));
  });
});
