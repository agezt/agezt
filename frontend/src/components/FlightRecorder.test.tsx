// @vitest-environment jsdom
import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { FlightRecorder } from "@/components/FlightRecorder";

afterEach(cleanup);
beforeEach(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

describe("FlightRecorder", () => {
  it("shows incident badges on replay steps when present", () => {
    render(
      <FlightRecorder
        steps={[
          {
            seq: 1,
            ts: 10,
            kind: "info",
            iter: null,
            title: "infra-lead · incident owner woke · agent.wake",
            detail: "",
            tone: "other",
            incident: {
              subject: "agent.wake",
              phase: "completed",
              mode: undefined,
            },
            cumIn: 0,
            cumOut: 0,
            cumCostMc: 0,
            cumTools: 0,
          },
        ]}
      />,
    );
    expect(screen.getByText("operator")).toBeTruthy();
    expect(screen.getByText("completed")).toBeTruthy();
    expect(
      screen.getByText(/infra-lead · incident owner woke · agent\.wake/),
    ).toBeTruthy();
  });
});
