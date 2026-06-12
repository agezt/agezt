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

import {
  NewScheduleForm,
  Schedules,
  parseSchedulesJSON,
  untilLabel,
  scheduleCounts,
  DUE_SOON_MS,
} from "@/views/Schedules";
import { UIProvider } from "@/components/ui/feedback";
import type { ReactNode } from "react";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

describe("untilLabel (M917)", () => {
  const now = 1_000_000_000_000;
  it("renders a coarse countdown, and overdue/now near zero", () => {
    expect(untilLabel(now - 5000, now)).toBe("overdue");
    expect(untilLabel(now + 5000, now)).toBe("now");
    expect(untilLabel(now + 45_000, now)).toBe("in 45s");
    expect(untilLabel(now + 12 * 60_000, now)).toBe("in 12m");
    expect(untilLabel(now + 3 * 3_600_000, now)).toBe("in 3h");
    expect(untilLabel(now + 2 * 24 * 3_600_000, now)).toBe("in 2d");
  });
});

describe("scheduleCounts (M917)", () => {
  const now = 1_000_000_000_000;
  it("tallies enabled/paused and due-within-the-hour (enabled only)", () => {
    const items = [
      { enabled: true, next_run_unix: (now + 10 * 60_000) / 1000 }, // due soon
      { enabled: true, next_run_unix: (now + 5 * 3_600_000) / 1000 }, // later
      { enabled: false, next_run_unix: (now + 60_000) / 1000 }, // paused → not due-soon
      { enabled: true }, // continuous/no next → enabled but not due-soon
    ];
    expect(scheduleCounts(items, now)).toEqual({ total: 4, enabled: 3, paused: 1, dueSoon: 1 });
    expect(DUE_SOON_MS).toBe(3_600_000);
  });
});

afterEach(cleanup);
beforeEach(() => {
  postJSON.mockReset();
  postJSON.mockResolvedValue({ id: "sch-1" });
  getJSON.mockReset();
});

describe("NewScheduleForm", () => {
  it("disables Create until an intent is entered", () => {
    render(<NewScheduleForm onCreated={() => {}} onError={() => {}} />);
    const create = screen.getByRole("button", { name: /Create schedule/ }) as HTMLButtonElement;
    expect(create.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Schedule intent"), { target: { value: "Summarize runs" } });
    expect((screen.getByRole("button", { name: /Create schedule/ }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("posts an interval schedule (minutes → interval_sec)", async () => {
    render(<NewScheduleForm onCreated={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Schedule intent"), { target: { value: "ping" } });
    fireEvent.change(screen.getByLabelText("Interval amount"), { target: { value: "15" } });
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/schedule/add", { intent: "ping", interval_sec: 900 }));
  });

  it("posts a daily schedule (HH:MM → at_minutes, every day)", async () => {
    render(<NewScheduleForm onCreated={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Schedule intent"), { target: { value: "briefing" } });
    fireEvent.click(screen.getByRole("button", { name: "daily at…" }));
    fireEvent.change(screen.getByLabelText("Daily time"), { target: { value: "09:30" } });
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/add", { intent: "briefing", at_minutes: 570, days: 0 }),
    );
  });

  it("posts a one-shot schedule (datetime → once_at_unix)", async () => {
    render(<NewScheduleForm onCreated={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Schedule intent"), { target: { value: "deploy" } });
    fireEvent.click(screen.getByRole("button", { name: "once at…" }));
    fireEvent.change(screen.getByLabelText("Once date and time"), { target: { value: "2030-01-02T03:04" } });
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() => {
      const call = postJSON.mock.calls.find((c) => c[0] === "/api/schedule/add");
      expect(call).toBeTruthy();
      const args = call![1] as { intent: string; once_at_unix: number };
      expect(args.intent).toBe("deploy");
      expect(args.once_at_unix).toBe(Math.floor(Date.parse("2030-01-02T03:04") / 1000));
    });
  });

  it("surfaces a create error", async () => {
    postJSON.mockRejectedValueOnce(new Error("bad schedule"));
    const onError = vi.fn();
    render(<NewScheduleForm onCreated={() => {}} onError={onError} />);
    fireEvent.change(screen.getByLabelText("Schedule intent"), { target: { value: "x" } });
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() => expect(onError).toHaveBeenCalledWith("bad schedule"));
  });
});

describe("NewScheduleForm (edit mode, M728)", () => {
  it("prefills the intent and labels the action Save changes", () => {
    render(<NewScheduleForm editId="sch-7" initialIntent="old intent" onCreated={() => {}} onError={() => {}} />);
    expect((screen.getByLabelText("Schedule intent") as HTMLTextAreaElement).value).toBe("old intent");
    // The create label is gone; the edit label is shown.
    expect(screen.queryByRole("button", { name: /Create schedule/ })).toBeNull();
    expect(screen.getByRole("button", { name: /Save changes/ })).toBeTruthy();
  });

  it("posts to schedule/edit with the id and the new intent + cadence", async () => {
    render(<NewScheduleForm editId="sch-7" initialIntent="old intent" onCreated={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Schedule intent"), { target: { value: "new intent" } });
    fireEvent.click(screen.getByRole("button", { name: "daily at…" }));
    fireEvent.change(screen.getByLabelText("Daily time"), { target: { value: "06:15" } });
    fireEvent.click(screen.getByRole("button", { name: /Save changes/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/edit", {
        id: "sch-7",
        intent: "new intent",
        at_minutes: 375,
        days: 0,
      }),
    );
  });

  it("calls onCreated after a successful edit", async () => {
    const onCreated = vi.fn();
    render(<NewScheduleForm editId="sch-7" initialIntent="x" onCreated={onCreated} onError={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: /Save changes/ }));
    await waitFor(() => expect(onCreated).toHaveBeenCalled());
  });
});

describe("parseSchedulesJSON (M749)", () => {
  it("reads a bare array and a {schedules:[…]} wrapper", () => {
    const row = { id: "x", intent: "ping", mode: "", interval_sec: 900, source: "operator", enabled: true };
    expect(parseSchedulesJSON(JSON.stringify([row]))).toHaveLength(1);
    expect(parseSchedulesJSON(JSON.stringify({ version: 1, schedules: [row] }))).toHaveLength(1);
  });

  it("rebuilds interval args (mode '' → interval_sec), dropping kernel fields", () => {
    const out = parseSchedulesJSON(
      JSON.stringify([{ id: "x", source: "agent", enabled: false, fires: 4, intent: "ping", mode: "", interval_sec: 900 }]),
    );
    expect(out[0]).toEqual({ intent: "ping", interval_sec: 900 });
  });

  it("rebuilds daily args (at_minutes + days + tz)", () => {
    const out = parseSchedulesJSON(
      JSON.stringify([{ intent: "brief", mode: "daily", at_minutes: 570, days: 0, tz: "Europe/Istanbul" }]),
    );
    expect(out[0]).toEqual({ intent: "brief", at_minutes: 570, days: 0, tz: "Europe/Istanbul" });
  });

  it("rebuilds window args (at_minutes→window_start, end_minutes→window_end, interval_sec)", () => {
    const out = parseSchedulesJSON(
      JSON.stringify([{ intent: "watch", mode: "window", at_minutes: 540, end_minutes: 1020, interval_sec: 1800, days: 0 }]),
    );
    expect(out[0]).toEqual({ intent: "watch", window_start: 540, window_end: 1020, interval_sec: 1800, days: 0 });
  });

  it("rebuilds once args from once_at_unix, falling back to next_run_unix", () => {
    expect(parseSchedulesJSON(JSON.stringify([{ intent: "deploy", mode: "once", once_at_unix: 1893456000 }]))[0]).toEqual({
      intent: "deploy",
      once_at_unix: 1893456000,
    });
    expect(parseSchedulesJSON(JSON.stringify([{ intent: "deploy", mode: "once", next_run_unix: 1893456000 }]))[0]).toEqual({
      intent: "deploy",
      once_at_unix: 1893456000,
    });
  });

  it("keeps an explicit model and carries it across cadence kinds", () => {
    const out = parseSchedulesJSON(JSON.stringify([{ intent: "x", mode: "", interval_sec: 60, model: "deepseek-chat" }]));
    expect(out[0]).toEqual({ intent: "x", interval_sec: 60, model: "deepseek-chat" });
  });

  it("skips continuous (agent-managed, no add path), intentless and invalid-cadence entries", () => {
    const out = parseSchedulesJSON(
      JSON.stringify([
        { intent: "keep", mode: "", interval_sec: 60 },
        { intent: "alive", mode: "continuous", interval_sec: 60 },
        { mode: "", interval_sec: 60 }, // no intent
        { intent: "zero", mode: "", interval_sec: 0 }, // invalid interval
        { intent: "noonce", mode: "once" }, // once with no fire time
      ]),
    );
    expect(out).toHaveLength(1);
    expect(out[0]).toEqual({ intent: "keep", interval_sec: 60 });
  });

  it("throws on invalid JSON, a non-array shape, or nothing re-addable", () => {
    expect(() => parseSchedulesJSON("nope")).toThrow();
    expect(() => parseSchedulesJSON('{"foo":1}')).toThrow(/expected an array/);
    expect(() => parseSchedulesJSON("[{}]")).toThrow(/no re-addable schedules/);
  });
});

describe("Schedules fire-time preview (M744)", () => {
  const sched = {
    id: "sch-9",
    intent: "morning brief",
    cadence: "daily at 09:00",
    mode: "daily",
    enabled: true,
    next_run_unix: 1893456000,
  };

  it("toggles a forecast of next fire times from /api/schedule/test", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/schedules") return Promise.resolve({ schedules: [sched] });
      if (path === "/api/schedule/test")
        return Promise.resolve({ forecasts: [{ unix: 1893456000 }, { unix: 1893542400 }] });
      return Promise.resolve({});
    });
    render(withUI(<Schedules />));
    await waitFor(() => expect(screen.getByText("morning brief")).toBeTruthy());

    // Open the forecast.
    fireEvent.click(screen.getByRole("button", { name: "next fires" }));
    await waitFor(() =>
      expect(getJSON).toHaveBeenCalledWith("/api/schedule/test", { id: "sch-9", count: "5" }),
    );
    // Two forecast rows render (numbered).
    await waitFor(() => expect(screen.getByText("1.")).toBeTruthy());
    expect(screen.getByText("2.")).toBeTruthy();

    // Toggle hides it.
    expect(screen.getByRole("button", { name: "hide fires" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "hide fires" }));
    await waitFor(() => expect(screen.queryByText("1.")).toBeNull());
  });
});
