// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ events: [], connected: true, subscribe: () => () => {} }),
}));

import { Wizards } from "@/views/Wizards";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postAction.mockReset();
  getJSON.mockResolvedValue({});
  postJSON.mockResolvedValue({});
  postAction.mockResolvedValue({});
});

describe("Wizards", () => {
  it("lists the available guided flows", () => {
    render(withUI(<Wizards />));
    expect(screen.getByText("Connect a provider")).toBeTruthy();
    expect(screen.getByText("Create an agent")).toBeTruthy();
    expect(screen.getByText("Schedule a task")).toBeTruthy();
    expect(screen.getByText("Create a cron trigger for an agent wake, workflow, system task, or tool call.")).toBeTruthy();
  });

  it("opens a wizard overlay on click and closes it", () => {
    render(withUI(<Wizards />));
    // Click the "Create an agent" card (the embedded form fetches nothing).
    fireEvent.click(screen.getByText("Create an agent"));
    const close = screen.getByLabelText("Close wizard");
    expect(close).toBeTruthy();
    fireEvent.click(close);
    expect(screen.queryByLabelText("Close wizard")).toBeNull();
  });

  it("channel wizard leads with popular channels and drills into the connect form", async () => {
    getJSON.mockResolvedValue({
      channels: [
        { kind: "telegram", display: "Telegram", description: "Bot API two-way chat.", fields: [{ env: "AGEZT_TELEGRAM_TOKEN", label: "Bot token", required: true }] },
        { kind: "email", display: "Email / SMTP", description: "Outbound over SMTP.", fields: [{ env: "AGEZT_EMAIL_SMTP_ADDR", label: "SMTP address" }] },
        { kind: "dingtalk", display: "DingTalk", description: "Enterprise robot.", fields: [] },
      ],
    });
    render(withUI(<Wizards />));
    fireEvent.click(screen.getByText("Connect a channel"));
    // Popular channels render as picker cards; the long tail stays behind search.
    expect(await screen.findByText("Telegram")).toBeTruthy();
    expect(screen.getByText("Email / SMTP")).toBeTruthy();
    expect(screen.queryByText("DingTalk")).toBeNull();
    expect(screen.getByText(/browse all 3 channels/)).toBeTruthy();
    // Search reaches the long tail.
    fireEvent.change(screen.getByLabelText("Search channels"), { target: { value: "ding" } });
    expect(screen.getByText("DingTalk")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Search channels"), { target: { value: "" } });
    // Drilling into a channel shows the guided connect form for its fields.
    fireEvent.click(screen.getByText("Telegram"));
    expect(await screen.findByText(/pick another channel/)).toBeTruthy();
    expect(screen.getByText("Bot token")).toBeTruthy();
  });
});
