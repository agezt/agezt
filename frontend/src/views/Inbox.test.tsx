// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

import { SendMessageForm } from "@/views/Inbox";

afterEach(cleanup);
beforeEach(() => {
  postAction.mockReset();
  postAction.mockResolvedValue({ sent: true });
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
