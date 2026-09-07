// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

import { Data, dataLakeActorAgent, dataLakeAgents, dataRecordAttribution, dataRecordWriter, filterDataRecordsByAgent } from "@/views/Data";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postAction.mockReset();
  getJSON.mockImplementation((path: string) => {
    if (path === "/api/data/collections") {
      return Promise.resolve({
        collections: [{ name: "notes", title: "Notes", fields: [{ name: "title" }, { name: "body" }], count: 3 }],
      });
    }
    if (path === "/api/data/records") {
      return Promise.resolve({
        records: [
          { id: "r1", fields: { title: "Ops note", body: "disk" }, created_by: "ops:corr-1", created_ms: 1000 },
          { id: "r2", fields: { title: "Research note", body: "paper" }, created_by: "researcher:corr-2", updated_by: "ops:corr-3", updated_ms: 2000 },
          { id: "r3", fields: { title: "Planner note", body: "roadmap" }, created_by: "planner" },
        ],
      });
    }
    return Promise.resolve({});
  });
});

describe("data lake provenance helpers", () => {
  const records = [
    { id: "r1", fields: {}, created_by: "ops:corr-1" },
    { id: "r2", fields: {}, created_by: "researcher:corr-2", updated_by: "ops:corr-3" },
    { id: "r3", fields: {}, created_by: "planner" },
  ];

  it("lists and filters records by creating/updating agent", () => {
    expect(dataLakeAgents(records)).toEqual(["ops", "planner", "researcher"]);
    expect(filterDataRecordsByAgent(records, "ops").map((r) => r.id)).toEqual(["r1", "r2"]);
    expect(filterDataRecordsByAgent(records, "").map((r) => r.id)).toEqual(["r1", "r2", "r3"]);
  });

  it("formats the visible writer and detailed attribution", () => {
    expect(dataLakeActorAgent("researcher:corr-2")).toBe("researcher");
    expect(dataRecordWriter({ created_by: "researcher", updated_by: "ops" })).toBe("ops");
    expect(dataRecordWriter({})).toBe("unknown");
    expect(dataRecordAttribution({ created_by: "researcher", created_ms: 1000, updated_by: "ops", updated_ms: 2000 })).toContain("created by researcher");
    expect(dataRecordAttribution({ created_by: "researcher", created_ms: 1000, updated_by: "ops", updated_ms: 2000 })).toContain("updated by ops");
  });
});

describe("Data view writer filter", () => {
  it("filters visible records by agent provenance", async () => {
    render(withUI(<Data />));
    await waitFor(() => expect(screen.getByText("Ops note")).toBeTruthy());
    expect(screen.getByText("Research note")).toBeTruthy();
    expect(screen.getByText("Planner note")).toBeTruthy();

    fireEvent.click(within(screen.getByRole("group", { name: "Filter records by agent" })).getByRole("button", { name: "ops" }));
    expect(screen.getByText("Ops note")).toBeTruthy();
    expect(screen.getByText("Research note")).toBeTruthy();
    expect(screen.queryByText("Planner note")).toBeNull();
  });

  // serveExpense drives the ExpenseView branch (activeCol.view === "expense").
  // A datalake `date` is free text: Lake.Insert stores fields verbatim,
  // Lake.Update merges the patch verbatim, handleDataInsert and the `db` tool
  // pass the raw value straight through, and the console editor's coerce() has
  // no `date` branch (number/money/bool/tags only). So "2026-9-5" is a legal
  // stored date, and ExpenseView must not depend on how it is spelled.
  function serveExpense(records: unknown[]) {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/data/collections") {
        return Promise.resolve({
          collections: [
            {
              name: "expenses",
              title: "Expenses",
              view: "expense",
              fields: [
                { name: "date", type: "date", label: "Date" },
                { name: "item", type: "text", label: "Item" },
                { name: "amount", type: "money", label: "Amount" },
                { name: "category", type: "text", label: "Category" },
              ],
            },
          ],
        });
      }
      if (path === "/api/data/records") return Promise.resolve({ records });
      return Promise.resolve({});
    });
  }

  // money() mirrors ExpenseView's own fmtMoney, so the expected text is built by
  // the SAME formatter the component uses. This runner's ICU renders 15 as
  // "15,00", so a literal "15.00" would assert the locale, not the code.
  const money = (n: number) => n.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  // Scoped to one metric card, so a money value repeated in the category bars
  // can never satisfy the assertion by accident.
  const metricText = (label: string) => screen.getByText(label).closest("div.rounded-xl")?.textContent ?? "";

  // useZone pins an IANA zone for the tests in the enclosing describe and puts
  // the previous value back. TZ is PROCESS-wide state, so leaking it would let
  // one test decide another runner's calendar frame — and `process.env.TZ =
  // undefined` is not a reset, Node coerces it to the literal "undefined".
  // Verified on this platform: Node re-reads TZ on every Date call, so the zone
  // can change per test rather than per process.
  // The headers render "Upcoming · N" / "Past · N" with a PLAIN integer, so
  // these assertions never touch a locale-formatted string at all. Hoisted to
  // this scope because both the pinned-today suite and the UTC-boundary suite
  // below need it — a second copy would be a second rule.
  const sectionCount = (heading: string) => {
    const el = screen.getByText(new RegExp(`^${heading}`));
    const n = Number((el.textContent ?? "").replace(/[^\d]/g, ""));
    return Number.isFinite(n) ? n : NaN;
  };

  // serveView installs one collection carrying the given custom view, so a
  // boundary test can drive ExpenseView or CalendarView without re-spelling the
  // API shape each time.
  function serveView(view: string, records: unknown[]) {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/data/collections") {
        return Promise.resolve({
          collections: [
            {
              name: "rec",
              title: "Rec",
              view,
              // ExpenseView's row prints `item`; CalendarView's row prints
              // `title`. Both are declared so a label is always on screen and a
              // missing row can never masquerade as a passing assertion.
              fields: [
                { name: "date", type: "date", label: "Date" },
                { name: "item", type: "text", label: "Item" },
                { name: "title", type: "text", label: "Title" },
                { name: "amount", type: "money", label: "Amount" },
              ],
            },
          ],
        });
      }
      if (path === "/api/data/records") return Promise.resolve({ records });
      return Promise.resolve({});
    });
  }

  function useZone(tz: string) {
    let prev: string | undefined;
    beforeEach(() => {
      prev = process.env.TZ;
      process.env.TZ = tz;
    });
    afterEach(() => {
      if (prev === undefined) delete process.env.TZ;
      else process.env.TZ = prev;
    });
  }

  // Pin the clock here too: the month assertion only discriminates when the
  // current month carries a leading zero. Derived from the real clock, a run in
  // October, November or December produced "2026-10-20", which satisfies
  // startsWith("2026-10") — so the test would have passed against UNFIXED code
  // depending on the date CI ran. Fixed literals under a pinned Date remove
  // that blind spot entirely.
  describe("expense month total and recency", () => {
    beforeEach(() => {
      vi.useFakeTimers({ toFake: ["Date"] });
      vi.setSystemTime(new Date("2026-09-06T12:00:00Z")); // today = 2026-09-06
    });
    afterEach(() => {
      vi.useRealTimers();
    });

    it("counts a non-canonical current-month date in the This-month total", async () => {
      serveExpense([
        { id: "a", fields: { date: "2026-09-05", item: "canon", amount: 10 } },
        { id: "b", fields: { date: "2026-9-20", item: "noncanon", amount: 5 } },
        { id: "c", fields: { date: "2026-8-15", item: "older", amount: 100 } },
      ]);
      render(withUI(<Data />));
      await screen.findByText("canon");
      // Guards the whole test: if the expense branch did not render, getByText
      // throws here instead of the money assertions passing vacuously.
      expect(screen.getByText("This month")).toBeTruthy();

      expect(metricText("Total")).toContain(money(115));
      // Pre-fix: "2026-9-20".startsWith("2026-09") is false, row b was dropped,
      // and This month silently read 10 instead of 15.
      expect(metricText("This month")).toContain(money(15));
    });

    it("orders the recent list chronologically, not by raw string", async () => {
      // Raw localeCompare DESC ranks "2026-9-5" above "2026-10-01" because '9'
      // (0x39) beats '1' at index 5, so September outranks October. Oct-01 is
      // the later record and must be listed first.
      serveExpense([
        { id: "s", fields: { date: "2026-9-5", item: "SepRec", amount: 1 } },
        { id: "o", fields: { date: "2026-10-01", item: "OctRec", amount: 2 } },
      ]);
      render(withUI(<Data />));
      const sep = await screen.findByText("SepRec");
      const oct = screen.getByText("OctRec");
      expect(oct.compareDocumentPosition(sep) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });
  });

  // CalendarView classifies with `date >= today` and sorts with localeCompare,
  // both on the RAW stored string. These pin the clock so the fixtures are
  // discriminating on every run, not only on days whose digits happen to agree.
  describe("calendar upcoming/past split", () => {
    beforeEach(() => {
      vi.useFakeTimers({ toFake: ["Date"] });
      vi.setSystemTime(new Date("2026-10-05T12:00:00Z")); // today = 2026-10-05
    });
    afterEach(() => {
      vi.useRealTimers();
    });

    // serveCalendar drives the activeCol.view === "calendar" branch.
    function serveCalendar(records: unknown[]) {
      getJSON.mockImplementation((path: string) => {
        if (path === "/api/data/collections") {
          return Promise.resolve({
            collections: [
              {
                name: "calendar",
                title: "Calendar",
                view: "calendar",
                fields: [
                  { name: "date", type: "date", label: "Date" },
                  { name: "title", type: "text", label: "Title" },
                ],
              },
            ],
          });
        }
        if (path === "/api/data/records") return Promise.resolve({ records });
        return Promise.resolve({});
      });
    }

    it("sinks an unreadable date to Past instead of listing it as Upcoming", async () => {
      // This pins a DELIBERATE behaviour change, not the original bug: the
      // pre-fix comparison was `"xyz" >= "2026-10-05"`, which is true because
      // 'x' (0x78) beats '2', so an event with a garbage date was presented as
      // upcoming. dayKey maps it to "" — below every real day — so it now falls
      // into Past. An unreadable date is not evidence of something scheduled.
      serveCalendar([
        { id: "g", fields: { date: "xyz", title: "garbage-date" } },
        { id: "u", fields: { date: "2026-10-20", title: "real-upcoming" } },
      ]);
      render(withUI(<Data />));
      await screen.findByText("garbage-date");

      expect(sectionCount("Upcoming")).toBe(1);
      expect(sectionCount("Past")).toBe(1);
    });

    it("does not classify a past non-canonical date as upcoming", async () => {
      // "2026-9-20" is Sep 20, ten days BEFORE the pinned today. Raw string
      // compare puts it >= "2026-10-05" because '9' (0x39) beats '1' at index 5,
      // so a past event was listed as upcoming.
      serveCalendar([
        { id: "p", fields: { date: "2026-9-20", title: "past-sep" } },
        { id: "u", fields: { date: "2026-10-20", title: "future-oct" } },
      ]);
      render(withUI(<Data />));
      await screen.findByText("past-sep");

      expect(sectionCount("Upcoming")).toBe(1);
      expect(sectionCount("Past")).toBe(1);
      expect(screen.queryByText("no-such-row")).toBeNull(); // guards vacuous pass
    });

    it("sorts the upcoming list soonest-first, not by raw string", async () => {
      // Both are upcoming. Raw ASC localeCompare ranks "2026-10-20" before
      // "2026-10-9" ('2' < '9' at index 8), so the 20th printed above the 9th.
      serveCalendar([
        { id: "late", fields: { date: "2026-10-20", title: "late-oct" } },
        { id: "mid", fields: { date: "2026-10-9", title: "mid-oct" } },
      ]);
      render(withUI(<Data />));
      const late = await screen.findByText("late-oct");
      const mid = screen.getByText("mid-oct");
      expect(sectionCount("Upcoming")).toBe(2);
      expect(mid.compareDocumentPosition(late) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });
  });

  // A stored `date` is a bare LOCAL calendar day ("2026-10-01" means that date on
  // the operator's wall), but both views derived "now" with `toISOString()`,
  // which is UTC. Near a month edge the two frames name different months/days,
  // so the answer depended on the operator's offset. Each case fixes ONE
  // absolute instant and varies only the zone, and every stored date here is
  // already canonical `YYYY-MM-DD` — so padding cannot explain a failure and the
  // timezone is the single axis under test.
  describe("local-vs-UTC month boundary", () => {
    // UTC+14: local wall clock is AHEAD of UTC, so UTC still names September
    // while the operator is already in October.
    describe("east of UTC (Pacific/Kiritimati)", () => {
      useZone("Pacific/Kiritimati");
      beforeEach(() => {
        vi.useFakeTimers({ toFake: ["Date"] });
        vi.setSystemTime(new Date("2026-09-30T12:00:00Z")); // local: 2026-10-01
      });
      afterEach(() => {
        vi.useRealTimers();
      });

      it("totals the operator's month, not UTC's", async () => {
        // ExpenseView prints `item`, so the label lives on that field (a `title`
        // here would render as "—" and the test would fail on the harness).
        serveView("expense", [
          { id: "oct", fields: { date: "2026-10-01", item: "oct-expense", amount: 40 } },
          { id: "sep", fields: { date: "2026-09-30", item: "sep-expense", amount: 5 } },
        ]);
        render(withUI(<Data />));
        await screen.findByText("oct-expense");
        expect(screen.getByText("This month")).toBeTruthy(); // guards a vacuous pass

        // Pre-fix: ym came from toISOString() = "2026-09", so the total picked
        // the SEP record (5) and dropped the operator's whole current month.
        expect(metricText("Total")).toContain(money(45));
        expect(metricText("This month")).toContain(money(40));
      });

      it("treats yesterday-in-UTC as already past", async () => {
        serveView("calendar", [
          { id: "y", fields: { date: "2026-09-30", title: "local-yesterday" } },
          { id: "f", fields: { date: "2026-10-05", title: "local-future" } },
        ]);
        render(withUI(<Data />));
        await screen.findByText("local-yesterday");

        // Pre-fix: today = "2026-09-30", so "2026-09-30" >= today was true and a
        // day that the operator already lived through listed as Upcoming.
        expect(sectionCount("Upcoming")).toBe(1);
        expect(sectionCount("Past")).toBe(1);
      });
    });

    // UTC-4 (DST): local wall clock is BEHIND UTC, so UTC already names October
    // while the operator is still in September — the opposite failure direction.
    describe("west of UTC (America/New_York)", () => {
      useZone("America/New_York");
      beforeEach(() => {
        vi.useFakeTimers({ toFake: ["Date"] });
        vi.setSystemTime(new Date("2026-10-01T03:00:00Z")); // local: 2026-09-30
      });
      afterEach(() => {
        vi.useRealTimers();
      });

      it("does not pull next month's records into this month", async () => {
        serveView("expense", [
          { id: "oct", fields: { date: "2026-10-01", item: "oct-expense", amount: 40 } },
          { id: "sep", fields: { date: "2026-09-30", item: "sep-expense", amount: 5 } },
        ]);
        render(withUI(<Data />));
        await screen.findByText("oct-expense");
        expect(screen.getByText("This month")).toBeTruthy();

        // Pre-fix: ym = "2026-10" counted the FUTURE record as spent this month.
        expect(metricText("Total")).toContain(money(45));
        expect(metricText("This month")).toContain(money(5));
      });

      it("lists today's own event as upcoming, not past", async () => {
        serveView("calendar", [
          { id: "t", fields: { date: "2026-09-30", title: "local-today" } },
          { id: "p", fields: { date: "2026-09-20", title: "local-past" } },
        ]);
        render(withUI(<Data />));
        await screen.findByText("local-today");

        // Pre-fix: today = "2026-10-01" (UTC), so the operator's OWN today sorted
        // below it and today's event was buried in Past.
        expect(sectionCount("Upcoming")).toBe(1);
        expect(sectionCount("Past")).toBe(1);
      });
    });
  });

  it("adds a record through the modal editor", async () => {
    render(withUI(<Data />));
    await waitFor(() => expect(screen.getByText("Ops note")).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: /add/i }));
    const dialog = await screen.findByRole("dialog", { name: /add notes record/i });
    fireEvent.change(within(dialog).getByLabelText("title"), { target: { value: "New note" } });
    fireEvent.change(within(dialog).getByLabelText("body"), { target: { value: "ship it" } });
    fireEvent.click(within(dialog).getByRole("button", { name: /add/i }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/data/insert", {
        collection: "notes",
        record: { title: "New note", body: "ship it" },
      }),
    );
  });

  it("canonicalizes a date-typed field on submit, not just on read", async () => {
    // The read side (dayKey/monthKey) tolerates any spelling; this test pins
    // the WRITE side: a date typed freehand into the editor must leave the
    // console already canonical, so an offline reader or the optimistic row
    // never sees "2026-9-5".
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/data/collections") {
        return Promise.resolve({
          collections: [{ name: "log", title: "Log", fields: [{ name: "date", type: "date" }, { name: "note" }] }],
        });
      }
      if (path === "/api/data/records") {
        return Promise.resolve({ records: [{ id: "r1", fields: { date: "2026-09-01", note: "seed" } }] });
      }
      return Promise.resolve({});
    });
    render(withUI(<Data />));
    fireEvent.click(await screen.findByRole("button", { name: /add/i }));
    const dialog = await screen.findByRole("dialog", { name: /add log record/i });
    // The field's label renders the name PLUS a type badge ("date date") when
    // the schema declares a type, so an exact-match query would miss it. The
    // existing editor test's fields carry no type, which is why plain
    // getByLabelText("title") works there.
    fireEvent.change(within(dialog).getByLabelText(/^date/), { target: { value: "2026-9-5" } });
    fireEvent.change(within(dialog).getByLabelText("note"), { target: { value: "typed by hand" } });
    fireEvent.click(within(dialog).getByRole("button", { name: /add/i }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/data/insert", {
        collection: "log",
        record: { date: "2026-09-05", note: "typed by hand" },
      }),
    );
  });
});
