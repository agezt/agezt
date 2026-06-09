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

import { NewScheduleForm, Schedules } from "@/views/Schedules";
import { UIProvider } from "@/components/ui/feedback";
import type { ReactNode } from "react";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

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
