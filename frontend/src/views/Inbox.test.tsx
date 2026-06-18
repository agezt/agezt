// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
const postAction = vi.fn();
const focusRun = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
  withToken: (path: string, extra?: Record<string, string>) =>
    `${path}?${new URLSearchParams(extra || {}).toString()}`,
}));
// Avoid the SSE EventSource (not in jsdom): stub the events hook.
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ events: [], connected: true, subscribe: () => () => {} }),
}));
vi.mock("@/lib/runfocus", () => ({ focusRun: (...a: unknown[]) => focusRun(...a) }));

import { SendMessageForm, Inbox, threadMatches } from "@/views/Inbox";
import { UIProvider } from "@/components/ui/feedback";

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postAction.mockReset();
  focusRun.mockReset();
  postAction.mockResolvedValue({ sent: true });
  location.hash = "";
});

describe("SendMessageForm (M747)", () => {
  it("disables Send until channel, recipient and text are all set", () => {
    render(<SendMessageForm onSent={() => {}} onError={() => {}} />);
    const btn = () => screen.getByRole("button", { name: /^Send$/ }) as HTMLButtonElement;
    expect(btn().disabled).toBe(true); // channel prefilled "telegram", but to+text empty
    fireEvent.change(screen.getByLabelText("Send recipient"), { target: { value: "12345" } });
    expect(btn().disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Send message text"), { target: { value: "hello" } });
    expect(btn().disabled).toBe(false);
  });

  it("posts channel (lower-cased) + to + text (trimmed) to /api/send", async () => {
    const onSent = vi.fn();
    render(<SendMessageForm onSent={onSent} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Send channel"), { target: { value: "Webhook" } });
    fireEvent.change(screen.getByLabelText("Send recipient"), { target: { value: "  ops  " } });
    fireEvent.change(screen.getByLabelText("Send message text"), { target: { value: "  deploy done  " } });
    fireEvent.click(screen.getByRole("button", { name: /^Send$/ }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/send", { channel: "webhook", to: "ops", text: "deploy done" }),
    );
    await waitFor(() => expect(onSent).toHaveBeenCalledWith("webhook", "ops"));
  });

  it("prefills from a thread (reply)", () => {
    render(<SendMessageForm initial={{ channel: "slack", to: "C123" }} onSent={() => {}} onError={() => {}} />);
    expect((screen.getByLabelText("Send channel") as HTMLInputElement).value).toBe("slack");
    expect((screen.getByLabelText("Send recipient") as HTMLInputElement).value).toBe("C123");
  });

  it("surfaces the daemon's 'no channels configured' error", async () => {
    postAction.mockRejectedValueOnce(new Error("no channels configured (set a channel token to enable send)"));
    const onError = vi.fn();
    render(<SendMessageForm initial={{ channel: "telegram", to: "1" }} onSent={() => {}} onError={onError} />);
    fireEvent.change(screen.getByLabelText("Send message text"), { target: { value: "hi" } });
    fireEvent.click(screen.getByRole("button", { name: /^Send$/ }));
    await waitFor(() => expect(onError).toHaveBeenCalledWith(expect.stringMatching(/no channels configured/)));
  });
});

describe("Inbox conversation search (M776)", () => {
  it("threadMatches matches channel kind/id and message sender/text (case-insensitive)", () => {
    const th = {
      correlation_id: "c1",
      channel_kind: "telegram",
      channel_id: "98765",
      messages: [{ direction: "in", sender: "Ada", text: "ship the release" }],
    };
    expect(threadMatches(th, "telegram")).toBe(true);
    expect(threadMatches(th, "98765")).toBe(true);
    expect(threadMatches(th, "ada")).toBe(true);
    expect(threadMatches(th, "release")).toBe(true);
    expect(threadMatches(th, "nope")).toBe(false);
    expect(threadMatches(th, "")).toBe(true);
  });

  it("filters conversations and shows a count once there are more than four", async () => {
    const mk = (id: string, kind: string, text: string) => ({
      correlation_id: id,
      channel_kind: kind,
      channel_id: id,
      messages: [{ direction: "in", sender: "x", text }],
    });
    getJSON.mockResolvedValue({
      threads: [
        mk("t1", "telegram", "ship the release"),
        mk("t2", "slack", "review the diff"),
        mk("t3", "discord", "standup notes"),
        mk("t4", "telegram", "lunch?"),
        mk("t5", "email", "invoice attached"),
      ],
    });
    render(
      <UIProvider>
        <Inbox />
      </UIProvider>,
    );
    const input = await screen.findByLabelText("Filter conversations");
    expect(screen.queryByText("1/5")).toBeNull();
    fireEvent.change(input, { target: { value: "slack" } });
    await waitFor(() => expect(screen.getByText("1/5")).toBeTruthy());
    fireEvent.change(input, { target: { value: "zzz" } });
    await waitFor(() => expect(screen.getByText(/no conversations match/)).toBeTruthy());
  });
});

describe("Inbox inline image thumbnails (M828)", () => {
  it("renders inbound image artifacts matched to their thread by correlation", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/inbox") {
        return Promise.resolve({
          threads: [
            { correlation_id: "c1", channel_kind: "telegram", channel_id: "99", messages: [{ direction: "in", sender: "Ada", text: "look at this" }] },
          ],
        });
      }
      if (path === "/api/artifacts") {
        return Promise.resolve({
          entries: [
            { id: "art-1", ref: "aaa", kind: "image", mime: "image/png", corr: "c1", caption: "a cat" },
            { id: "art-2", ref: "bbb", kind: "image", mime: "image/png", corr: "other-thread" },
          ],
        });
      }
      return Promise.resolve({});
    });

    render(
      <UIProvider>
        <Inbox />
      </UIProvider>,
    );

    // The thread's own image (corr c1) renders; the other thread's image does not.
    await waitFor(() => expect(screen.getByText("look at this")).toBeTruthy());
    const imgs = document.querySelectorAll("img");
    expect(imgs.length).toBe(1);
    expect(imgs[0].getAttribute("src")).toContain("/api/artifact/raw?ref=aaa");
  });
});

describe("Inbox run linking", () => {
  it("opens the governed run for a channel thread by correlation id", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/inbox") {
        return Promise.resolve({
          threads: [
            {
              correlation_id: "chan-corr-1",
              channel_kind: "webhook",
              channel_id: "room-1",
              messages: [{ direction: "in", sender: "ersin", text: "check the mailbox" }],
            },
          ],
        });
      }
      return Promise.resolve({ entries: [] });
    });

    render(
      <UIProvider>
        <Inbox />
      </UIProvider>,
    );

    fireEvent.click(await screen.findByRole("button", { name: /run/i }));
    expect(focusRun).toHaveBeenCalledWith("chan-corr-1");
    expect(location.hash).toBe("#runs");
  });
});
