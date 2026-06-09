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

import { NewOrderForm, EditOrderForm, Standing, parseStandingJSON } from "@/views/Standing";
import { UIProvider } from "@/components/ui/feedback";
import type { ReactNode } from "react";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

afterEach(cleanup);
beforeEach(() => {
  postJSON.mockReset();
  postJSON.mockResolvedValue({ order: { id: "so-1" } });
  getJSON.mockReset();
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

describe("Standing order history (M746)", () => {
  const order = {
    id: "so-9",
    name: "Morning briefing",
    enabled: true,
    triggers: [{ type: "cron", schedule: "0 8 * * *" }],
    initiative: { mode: "ask" },
  };

  it("toggles the order's life story from /api/standing/why", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/standing") return Promise.resolve({ orders: [order] });
      if (path === "/api/standing/why")
        return Promise.resolve({
          events: [
            { seq: 1, kind: "standing.created", ts_unix_ms: 1893456000000, payload: {} },
            { seq: 2, kind: "standing.updated", ts_unix_ms: 1893456100000, payload: { action: "paused" } },
          ],
        });
      return Promise.resolve({});
    });
    render(withUI(<Standing />));
    await waitFor(() => expect(screen.getByText("Morning briefing")).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: "history" }));
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/standing/why", { id: "so-9" }));
    // Event kinds render (stripped of the "standing." prefix) + the action label.
    await waitFor(() => expect(screen.getByText("created")).toBeTruthy());
    expect(screen.getByText("updated")).toBeTruthy();
    expect(screen.getByText("paused")).toBeTruthy();

    // Toggle hides it.
    fireEvent.click(screen.getByRole("button", { name: "hide history" }));
    await waitFor(() => expect(screen.queryByText("created")).toBeNull());
  });
});

describe("Standing order Run now (M765)", () => {
  const order = { id: "so-7", name: "Nightly digest", enabled: true, triggers: [{ type: "cron", schedule: "0 8 * * *" }] };

  it("fires the order on demand via /api/standing/fire", async () => {
    getJSON.mockImplementation((path: string) =>
      path === "/api/standing" ? Promise.resolve({ orders: [order] }) : Promise.resolve({}),
    );
    render(withUI(<Standing />));
    await waitFor(() => expect(screen.getByText("Nightly digest")).toBeTruthy());
    fireEvent.click(screen.getByTitle(/Run now/));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/standing/fire", { id: "so-7" }));
  });
});

describe("parseStandingJSON (M748)", () => {
  const order = (name: string) => ({ name, triggers: [{ type: "cron", schedule: "0 8 * * *" }], plan: "x" });

  it("reads a bare array, a {standing:[…]} and a {orders:[…]} wrapper", () => {
    expect(parseStandingJSON(JSON.stringify([order("a")]))).toHaveLength(1);
    expect(parseStandingJSON(JSON.stringify({ standing: [order("b")] }))).toHaveLength(1);
    expect(parseStandingJSON(JSON.stringify({ version: 1, orders: [order("c")] }))).toHaveLength(1);
  });

  it("strips kernel-assigned id/timestamps but keeps the declarative shape", () => {
    const out = parseStandingJSON(
      JSON.stringify([{ ...order("watch"), id: "x", enabled: true, created_ms: 1, updated_ms: 2, initiative: { mode: "ask" } }]),
    );
    expect(out[0]).toEqual({ name: "watch", triggers: [{ type: "cron", schedule: "0 8 * * *" }], plan: "x", initiative: { mode: "ask" } });
    expect(out[0]).not.toHaveProperty("id");
    expect(out[0]).not.toHaveProperty("created_ms");
  });

  it("drops entries missing a name or triggers", () => {
    const out = parseStandingJSON(JSON.stringify([order("ok"), { name: "no-trigger" }, { triggers: [{ type: "cron", schedule: "x" }] }]));
    expect(out).toHaveLength(1);
    expect(out[0].name).toBe("ok");
  });

  it("throws on invalid JSON, a non-array shape, or nothing valid", () => {
    expect(() => parseStandingJSON("nope")).toThrow();
    expect(() => parseStandingJSON('{"foo":1}')).toThrow(/expected an array/);
    expect(() => parseStandingJSON("[{}]")).toThrow(/no valid orders/);
  });
});
