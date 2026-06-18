// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));
const subscribe = vi.fn(() => () => {});
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ connected: true, subscribe }),
}));

import { Activity } from "@/views/Activity";
import { UIProvider } from "@/components/ui/feedback";

function withUI(node: ReactNode) {
  return <UIProvider>{node}</UIProvider>;
}

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postAction.mockReset();
  subscribe.mockClear();
});

describe("Activity", () => {
  it("shows doctor/operator provenance and phase badges in the autonomous doctor strip", async () => {
    getJSON.mockImplementation((url: string) => {
      if (url === "/api/runs") return Promise.resolve({ runs: [] });
      if (url === "/api/autonomy") {
        return Promise.resolve({
          items: [
            {
              seq: 2,
              kind: "info",
              subject: "agent.wake",
              category: "doctor",
              title: "an operator wake completed",
              detail: "infra-lead · operator wake completed",
              phase: "completed",
              ts_unix_ms: 20,
            },
            {
              seq: 1,
              kind: "info",
              subject: "doctor.auto_repair",
              category: "doctor",
              title: "a doctor run failed",
              detail: "builder · provider timeout",
              mode: "degraded",
              phase: "failed",
              ts_unix_ms: 10,
            },
          ],
        });
      }
      return Promise.resolve({});
    });

    render(withUI(<Activity />));
    await waitFor(() =>
      expect(screen.getByText("Autonomous doctor")).toBeTruthy(),
    );
    expect(screen.getAllByText("operator").length).toBeGreaterThan(0);
    expect(screen.getAllByText("doctor").length).toBeGreaterThan(0);
    expect(screen.getAllByText("completed").length).toBeGreaterThan(0);
    expect(screen.getAllByText("failed").length).toBeGreaterThan(0);
  });
});
