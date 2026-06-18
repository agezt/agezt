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
vi.mock("@/lib/events", () => ({ useEvents: () => ({ events: [] }) }));
vi.mock("@/views/Files", () => ({ rawURL: () => "" }));

import { ChannelSessions } from "@/views/ChannelSessions";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postAction.mockReset();
  getJSON.mockImplementation((path: string) => {
    if (path === "/api/inbox")
      return Promise.resolve({
        threads: [
          {
            correlation_id: "c1",
            channel_kind: "telegram",
            channel_id: "123",
            last_ts_unix_ms: 1000,
            messages: [{ direction: "in", sender: "alice", text: "hi there", ts_unix_ms: 1000 }],
          },
        ],
      });
    return Promise.resolve({ entries: [] }); // /api/artifacts
  });
  postAction.mockResolvedValue({});
});

describe("ChannelSessions", () => {
  it("groups inbound channel traffic into a session and replies back to the channel", async () => {
    render(withUI(<ChannelSessions />));
    // The session shows up (titled by sender) in the sidebar section.
    const sessionBtn = await screen.findByText("alice");
    fireEvent.click(sessionBtn);
    // Pane opens with a reply composer (the two-way surface).
    const replyBox = await screen.findByLabelText("Reply");
    fireEvent.change(replyBox, { target: { value: "hello back" } });
    fireEvent.click(screen.getByRole("button", { name: /send/i }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/send", {
        channel: "telegram",
        to: "123",
        text: "hello back",
      }),
    );
  });
});
