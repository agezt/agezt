// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";

vi.mock("@/lib/events", () => ({
  useEvents: () => ({
    connected: true,
    events: [
      {
        id: "e1",
        kind: "info",
        subject: "agent.wake",
        ts_unix_ms: 10,
        payload: {
          agent: "infra-lead",
          phase: "completed",
          reason: "incident owner woke",
        },
      },
    ],
  }),
}));

import { EventFeed } from "@/components/EventFeed";

afterEach(cleanup);

describe("EventFeed", () => {
  it("shows incident-family provenance badges and compact summary in the live stream", () => {
    render(<EventFeed />);
    expect(screen.getByText("operator")).toBeTruthy();
    expect(screen.getByText("completed")).toBeTruthy();
    expect(
      screen.getByText(/infra-lead · incident owner woke · agent\.wake/),
    ).toBeTruthy();
  });
});
