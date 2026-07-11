// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor, within } from "@testing-library/react";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

import {
  NewOrderForm,
  EditOrderForm,
  Standing,
  parseStandingJSON,
  standingAttentionCount,
  standingFrequencyIssue,
  standingResumeIssue,
  initiativeEnforcement,
} from "@/views/Standing";
import { UIProvider } from "@/components/ui/feedback";
import type { ReactNode } from "react";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

afterEach(cleanup);
beforeEach(() => {
  postJSON.mockReset();
  postJSON.mockResolvedValue({ order: { id: "so-1" } });
  postAction.mockReset();
  postAction.mockResolvedValue({});
  getJSON.mockReset();
});

describe("initiativeEnforcement", () => {
  it("describes the enforced trust ceiling per mode (M999)", () => {
    expect(initiativeEnforcement("ask")).toMatch(/approval/i);
    expect(initiativeEnforcement("act_or_ask")).toMatch(/ceiling/i);
    expect(initiativeEnforcement("inform_only")).toMatch(/no tools/i);
    expect(initiativeEnforcement("")).toMatch(/no tools/i); // default = inform only
  });
});

describe("standingFrequencyIssue", () => {
  it("flags event cooldowns and cron schedules that can wake agents too often", () => {
    expect(standingFrequencyIssue({ frequency_warning: "backend warning" })).toBe("backend warning");
    expect(standingFrequencyIssue({ triggers: [{ type: "event", subject: "run.failed" }], cooldown_sec: 60 })).toBe(
      "event cooldown 1m is below the default 15m guard",
    );
    expect(standingFrequencyIssue({ triggers: [{ type: "cron", schedule: "* * * * *" }] })).toBe(
      "cron trigger may wake this standing order every minute",
    );
    expect(standingFrequencyIssue({ triggers: [{ type: "event", subject: "run.failed" }], cooldown_sec: 3600 })).toBe("");
    expect(standingFrequencyIssue({ triggers: [{ type: "cron", schedule: "0 8 * * *" }] })).toBe("");
  });
});

describe("Standing empty state", () => {
  it("describes standing orders as wake rules, not identities", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/standing") return Promise.resolve({ orders: [] });
      if (path === "/api/agents") return Promise.resolve({ profiles: [] });
      return Promise.resolve({});
    });

    render(withUI(<Standing />));

    await waitFor(() => expect(screen.getByText("No standing orders yet")).toBeTruthy());
    expect(screen.getByText(/durable wake rules, not agent identities/i)).toBeTruthy();
  });
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
    fireEvent.click(within(screen.getByLabelText("Initiative mode")).getByRole("button", { name: "act/ask" }));
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
    fireEvent.change(screen.getByLabelText("Event cooldown seconds"), { target: { value: "3600" } });
    fireEvent.click(screen.getByRole("button", { name: /Create order/ }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/standing/add", {
        order: { name: "Watch failures", triggers: [{ type: "event", subject: "run.failed" }], cooldown_sec: 3600 },
      }),
    );
  });

  it("builds a mailbox wake order for the selected agent", async () => {
    getJSON.mockImplementation((path: string) =>
      path === "/api/agents"
        ? Promise.resolve({ profiles: [{ slug: "ops", enabled: true }] })
        : Promise.resolve({}),
    );
    render(<NewOrderForm onCreated={() => {}} onError={() => {}} />);
    fireEvent.click(screen.getByLabelText("Pick conversation agent"));
    fireEvent.click(await screen.findByLabelText("Use agent ops"));
    fireEvent.click(screen.getByRole("button", { name: "DM" }));
    fireEvent.click(screen.getByRole("button", { name: /Create order/ }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/standing/add", {
        order: expect.objectContaining({
          name: "ops mailbox",
          agent: "ops",
          triggers: [{ type: "event", subject: "board.dm.ops" }],
          plan: expect.stringContaining("Read the triggering board message"),
        }),
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
    cooldown_sec: 900,
    triggers: [{ type: "cron", schedule: "0 8 * * *" }],
  };

  it("prefills the current mutable fields", () => {
    render(<EditOrderForm order={order} onSaved={() => {}} onError={() => {}} />);
    expect((screen.getByLabelText("Edit order name") as HTMLInputElement).value).toBe("watch");
    expect((screen.getByLabelText("Edit order plan") as HTMLTextAreaElement).value).toBe("old plan");
    expect(within(screen.getByLabelText("Edit initiative mode")).getByRole("button", { name: "act/ask" }).getAttribute("aria-pressed")).toBe("true");
    expect((screen.getByLabelText("Edit assure retries") as HTMLInputElement).value).toBe("1");
    expect((screen.getByLabelText("Edit event cooldown seconds") as HTMLInputElement).value).toBe("900");
  });

  it("posts the full editable state to standing/edit with the id", async () => {
    const onSaved = vi.fn();
    render(<EditOrderForm order={order} onSaved={onSaved} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Edit order name"), { target: { value: "  renamed  " } });
    fireEvent.change(screen.getByLabelText("Edit order plan"), { target: { value: "new plan" } });
    fireEvent.click(within(screen.getByLabelText("Edit initiative mode")).getByRole("button", { name: "ask" }));
    fireEvent.change(screen.getByLabelText("Edit assure retries"), { target: { value: "3" } });
    fireEvent.change(screen.getByLabelText("Edit event cooldown seconds"), { target: { value: "1800" } });
    fireEvent.click(screen.getByRole("button", { name: /Save changes/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/standing/edit", {
        id: "so-42",
        name: "renamed",
        plan: "new plan",
        agent: "", // M790: present-and-empty clears the agent
        mode: "ask",
        assure: 3,
        cooldown_sec: 1800,
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
    expect(screen.getAllByText("paused").length).toBeGreaterThan(0);

    // Toggle hides it.
    fireEvent.click(screen.getByRole("button", { name: "hide history" }));
    await waitFor(() => expect(screen.queryByText("created")).toBeNull());
  });

  it("surfaces standing orders that can wake agents too frequently", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/standing")
        return Promise.resolve({
          orders: [
            {
              id: "so-chatty",
              name: "Chatty watcher",
              enabled: true,
              cooldown_sec: 60,
              frequency_warning: "event cooldown is below the default 15m guard",
              triggers: [{ type: "event", subject: "run.failed" }],
            },
          ],
        });
      if (path === "/api/agents") return Promise.resolve({ profiles: [] });
      return Promise.resolve({});
    });

    render(withUI(<Standing />));
    await waitFor(() => expect(screen.getByText("Chatty watcher")).toBeTruthy());
    expect(screen.getByText("frequent")).toBeTruthy();
    // Once in the attention banner, once as the raw stored field in the details fold.
    expect(screen.getAllByText("event cooldown is below the default 15m guard").length).toBeGreaterThan(0);
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

describe("Standing order agent state", () => {
  it("counts standing orders that need target or cadence attention", () => {
    const agents = new Map([["ops", { slug: "ops", enabled: true }]]);

    expect(
      standingAttentionCount(
        [
          { id: "ok", agent: "ops", triggers: [{ type: "event", subject: "board.ops" }], cooldown_sec: 3600 },
          { id: "blocked", agent: "ops", target_status: "blocked", target_error: "standing agent ops is retired" },
          { id: "fast", triggers: [{ type: "event", subject: "run.failed" }], cooldown_sec: 60 },
        ],
        agents,
      ),
    ).toBe(2);
  });

  it("prefers backend target errors over local agent inference", () => {
    const agents = new Map([["ops", { slug: "ops", enabled: true }]]);

    expect(
      standingResumeIssue(
        {
          id: "so-blocked",
          agent: "ops",
          target_status: "blocked",
          target_error: "standing agent ops is retired",
        },
        agents,
      ),
    ).toBe("standing agent ops is retired");
    expect(standingResumeIssue({ id: "so-ready", agent: "ops" }, agents)).toBe("");
  });

  it("shows a paused bound agent and prevents unsafe re-arm", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/standing") {
        return Promise.resolve({
          orders: [
            {
              id: "so-paused",
              name: "Ops mailbox",
              enabled: false,
              agent: "ops",
              triggers: [{ type: "event", subject: "board.dm.ops" }],
            },
          ],
        });
      }
      if (path === "/api/agents") {
        return Promise.resolve({ profiles: [{ slug: "ops", enabled: false }] });
      }
      return Promise.resolve({});
    });

    render(withUI(<Standing />));
    await waitFor(() => expect(screen.getByText("Ops mailbox")).toBeTruthy());
    expect(screen.getByText("runs as ops")).toBeTruthy();
    expect(screen.getByText("is paused")).toBeTruthy();
    // Raw stored fields live once, inside the card's single "details" fold.
    expect(screen.getByText("details")).toBeTruthy();
    expect(screen.getByText("so-paused")).toBeTruthy();
    expect(screen.getByText(JSON.stringify([{ type: "event", subject: "board.dm.ops" }]))).toBeTruthy();
    const resume = screen.getAllByTitle("agent ops is paused")[0] as HTMLButtonElement;
    expect(resume.disabled).toBe(true);
    fireEvent.click(resume);
    expect(postAction).not.toHaveBeenCalledWith(
      "/api/standing/enable",
      expect.objectContaining({ id: "so-paused", enabled: "true" }),
    );
  });

  it("blocks re-arm for kind-only sub-agents bound to standing orders", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/standing") {
        return Promise.resolve({
          orders: [
            {
              id: "so-child",
              name: "Child mailbox",
              enabled: false,
              agent: "planner-child",
              triggers: [{ type: "event", subject: "board.dm.planner-child" }],
            },
          ],
        });
      }
      if (path === "/api/agents") {
        return Promise.resolve({ profiles: [{ slug: "planner-child", enabled: true, kind: "subagent" }] });
      }
      return Promise.resolve({});
    });

    render(withUI(<Standing />));
    await waitFor(() => expect(screen.getByText("Child mailbox")).toBeTruthy());
    expect(screen.getByText("is a managed sub-agent")).toBeTruthy();
    const resume = screen.getAllByTitle("agent planner-child is a managed sub-agent")[0] as HTMLButtonElement;
    expect(resume.disabled).toBe(true);
  });

  it("filters and pauses only enabled standing orders that need attention", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/standing") {
        return Promise.resolve({
          orders: [
            {
              id: "so-ok",
              name: "Healthy wake",
              enabled: true,
              agent: "ops",
              triggers: [{ type: "event", subject: "board.ops" }],
              cooldown_sec: 3600,
            },
            {
              id: "so-blocked",
              name: "Blocked wake",
              enabled: true,
              agent: "ops",
              triggers: [{ type: "event", subject: "board.ops" }],
              target_status: "blocked",
              target_error: "standing agent ops is retired",
            },
            {
              id: "so-fast",
              name: "Fast wake",
              enabled: true,
              triggers: [{ type: "event", subject: "run.failed" }],
              cooldown_sec: 60,
            },
            {
              id: "so-paused",
              name: "Paused bad wake",
              enabled: false,
              target_status: "blocked",
              target_error: "standing agent old is retired",
            },
          ],
        });
      }
      if (path === "/api/agents") {
        return Promise.resolve({ profiles: [{ slug: "ops", enabled: true }] });
      }
      return Promise.resolve({});
    });

    render(withUI(<Standing />));
    await waitFor(() => expect(screen.getByText("Healthy wake")).toBeTruthy());
    expect(screen.getByRole("button", { name: /Attention3/ })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Attention3/ }));
    expect(screen.queryByText("Healthy wake")).toBeNull();
    expect(screen.getByText("Blocked wake")).toBeTruthy();
    expect(screen.getByText("Fast wake")).toBeTruthy();
    expect(screen.getByText("Paused bad wake")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: /Pause attention/ }));
    fireEvent.click(screen.getAllByRole("button", { name: /Pause attention/ }).at(-1)!);

    await waitFor(() => {
      expect(postAction).toHaveBeenCalledWith("/api/standing/enable", { id: "so-blocked", enabled: "false" });
      expect(postAction).toHaveBeenCalledWith("/api/standing/enable", { id: "so-fast", enabled: "false" });
    });
    expect(postAction).not.toHaveBeenCalledWith("/api/standing/enable", { id: "so-ok", enabled: "false" });
    expect(postAction).not.toHaveBeenCalledWith("/api/standing/enable", { id: "so-paused", enabled: "false" });
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
