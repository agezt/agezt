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

import { NewOrderForm, EditOrderForm } from "@/views/Standing";

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

describe("EditOrderForm (M729)", () => {
  const order = {
    id: "so-42",
    name: "watch",
    plan: "old plan",
    initiative: { mode: "act_or_ask" },
    assure: 1,
    triggers: [{ type: "cron", schedule: "0 8 * * *" }],
  };

  it("prefills the current mutable fields", () => {
    render(<EditOrderForm order={order} onSaved={() => {}} onError={() => {}} />);
    expect((screen.getByLabelText("Edit order name") as HTMLInputElement).value).toBe("watch");
    expect((screen.getByLabelText("Edit order plan") as HTMLTextAreaElement).value).toBe("old plan");
    expect((screen.getByLabelText("Edit initiative mode") as HTMLSelectElement).value).toBe("act_or_ask");
    expect((screen.getByLabelText("Edit assure retries") as HTMLInputElement).value).toBe("1");
  });

  it("posts the full editable state to standing/edit with the id", async () => {
    const onSaved = vi.fn();
    render(<EditOrderForm order={order} onSaved={onSaved} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Edit order name"), { target: { value: "  renamed  " } });
    fireEvent.change(screen.getByLabelText("Edit order plan"), { target: { value: "new plan" } });
    fireEvent.change(screen.getByLabelText("Edit initiative mode"), { target: { value: "ask" } });
    fireEvent.change(screen.getByLabelText("Edit assure retries"), { target: { value: "3" } });
    fireEvent.click(screen.getByRole("button", { name: /Save changes/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/standing/edit", {
        id: "so-42",
        name: "renamed",
        plan: "new plan",
        mode: "ask",
        assure: 3,
      }),
    );
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith("renamed"));
  });

  it("disables Save when the name is cleared", () => {
    render(<EditOrderForm order={order} onSaved={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Edit order name"), { target: { value: "  " } });
    expect((screen.getByRole("button", { name: /Save changes/ }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("clamps a negative assure to 0", async () => {
    render(<EditOrderForm order={order} onSaved={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Edit assure retries"), { target: { value: "-5" } });
    fireEvent.click(screen.getByRole("button", { name: /Save changes/ }));
    await waitFor(() => {
      const call = postJSON.mock.calls.find((c) => c[0] === "/api/standing/edit");
      expect((call![1] as { assure: number }).assure).toBe(0);
    });
  });

  it("reports an edit failure via onError", async () => {
    postJSON.mockRejectedValueOnce(new Error("invalid mode"));
    const onError = vi.fn();
    render(<EditOrderForm order={order} onSaved={() => {}} onError={onError} />);
    fireEvent.click(screen.getByRole("button", { name: /Save changes/ }));
    await waitFor(() => expect(onError).toHaveBeenCalledWith("invalid mode"));
  });
});
