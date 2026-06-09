// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

import { NewOrderForm } from "@/views/Standing";

afterEach(cleanup);
beforeEach(() => {
  postJSON.mockReset();
  postJSON.mockResolvedValue({ order: { id: "so-1" } });
});

describe("NewOrderForm", () => {
  it("disables Create until name and trigger value are set", () => {
    render(<NewOrderForm onCreated={() => {}} onError={() => {}} />);
    const create = screen.getByRole("button", { name: /Create order/ }) as HTMLButtonElement;
    expect(create.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Order name"), { target: { value: "Briefing" } });
    expect((screen.getByRole("button", { name: /Create order/ }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Trigger value"), { target: { value: "0 9 * * *" } });
    expect((screen.getByRole("button", { name: /Create order/ }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("submits a cron order with plan and initiative mode", async () => {
    const onCreated = vi.fn();
    render(<NewOrderForm onCreated={onCreated} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Order name"), { target: { value: "  Morning briefing  " } });
    fireEvent.change(screen.getByLabelText("Trigger value"), { target: { value: "0 9 * * *" } });
    fireEvent.change(screen.getByLabelText("Order plan"), { target: { value: "Summarize overnight activity." } });
    fireEvent.change(screen.getByLabelText("Initiative mode"), { target: { value: "act_or_ask" } });
    fireEvent.click(screen.getByRole("button", { name: /Create order/ }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/standing/add", {
        order: {
          name: "Morning briefing",
          triggers: [{ type: "cron", schedule: "0 9 * * *" }],
          plan: "Summarize overnight activity.",
          initiative: { mode: "act_or_ask" },
        },
      }),
    );
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith("Morning briefing"));
  });

  it("submits an event-triggered order (subject, not schedule)", async () => {
    render(<NewOrderForm onCreated={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Order name"), { target: { value: "Watch failures" } });
    fireEvent.click(screen.getByRole("button", { name: "event" }));
    fireEvent.change(screen.getByLabelText("Trigger value"), { target: { value: "run.failed" } });
    fireEvent.click(screen.getByRole("button", { name: /Create order/ }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/standing/add", {
        order: { name: "Watch failures", triggers: [{ type: "event", subject: "run.failed" }] },
      }),
    );
  });

  it("reports an error via onError when the create fails", async () => {
    postJSON.mockRejectedValueOnce(new Error("bad cron"));
    const onError = vi.fn();
    render(<NewOrderForm onCreated={() => {}} onError={onError} />);
    fireEvent.change(screen.getByLabelText("Order name"), { target: { value: "X" } });
    fireEvent.change(screen.getByLabelText("Trigger value"), { target: { value: "nope" } });
    fireEvent.click(screen.getByRole("button", { name: /Create order/ }));
    await waitFor(() => expect(onError).toHaveBeenCalledWith("bad cron"));
  });
});
