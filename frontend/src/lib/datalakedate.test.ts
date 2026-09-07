// SPDX-License-Identifier: MIT

// Pure-logic tests for the datalake date normalizer. Node environment is
// enough — these are string helpers with no DOM, matching the repo convention
// that lib/* holds the pure logic the views consume.
import { describe, expect, it } from "vitest";

import { canonicalDate, dayKey, localDayKey, localMonthKey, monthKey } from "@/lib/datalakedate";

describe("dayKey", () => {
  it("passes an already-canonical date through unchanged", () => {
    expect(dayKey("2026-09-05")).toBe("2026-09-05");
    expect(dayKey("2026-12-31")).toBe("2026-12-31");
  });

  it("pads a non-canonical single-digit month or day", () => {
    // Both halves of the original defect: "2026-9-5" fails a
    // startsWith("2026-09") month filter AND sorts after "2026-10-01".
    expect(dayKey("2026-9-5")).toBe("2026-09-05");
    expect(dayKey("2026-9-20")).toBe("2026-09-20");
    expect(dayKey("2026-10-1")).toBe("2026-10-01");
    expect(dayKey("2026-1-1")).toBe("2026-01-01");
  });

  it("tolerates surrounding whitespace and a slash separator", () => {
    expect(dayKey("  2026-9-5  ")).toBe("2026-09-05");
    expect(dayKey("2026/9/5")).toBe("2026-09-05");
  });

  it("keeps the day out of a stored timestamp", () => {
    expect(dayKey("2026-09-05T10:00:00Z")).toBe("2026-09-05");
    expect(dayKey("2026-9-5 08:30")).toBe("2026-09-05");
  });

  it("returns the empty string for anything without an unambiguous date", () => {
    // "" must sort below every real date and match no month prefix, so a
    // malformed value sinks instead of being guessed into a bucket.
    expect(dayKey("")).toBe("");
    expect(dayKey("   ")).toBe("");
    expect(dayKey(undefined)).toBe("");
    expect(dayKey(null)).toBe("");
    expect(dayKey("not a date")).toBe("");
    expect(dayKey("2026-09")).toBe("");
    expect(dayKey("9/5")).toBe("");
    expect(dayKey("2026-13-01")).toBe(""); // month out of range
    expect(dayKey("2026-00-01")).toBe(""); // month zero
    expect(dayKey("2026-09-32")).toBe(""); // day out of range
    expect(dayKey("2026-09-00")).toBe(""); // day zero
  });

  it("accepts the 31st and the boundary days", () => {
    expect(dayKey("2026-1-31")).toBe("2026-01-31");
    expect(dayKey("2026-12-1")).toBe("2026-12-01");
  });

  it("range-checks without calendar-validating, as its docstring states", () => {
    // dayKey's docstring scopes itself to ordering and month bucketing, NOT to
    // deciding whether an instant really exists. Pin that so a future
    // "hardening" that starts rejecting these cannot silently change what the
    // views treat as a readable date: 30 February keeps a key.
    expect(dayKey("2026-02-30")).toBe("2026-02-30");
    expect(dayKey("2026-04-31")).toBe("2026-04-31");
    // 31 is the accepted ceiling; 32 falls outside the range check.
    expect(dayKey("2026-04-32")).toBe("");
  });

  it("is total: whatever it returns is fixed-width, so string order is date order", () => {
    // The invariant the ExpenseView comparisons silently depended on. A raw
    // "2026-9-5" violates it ("9" > "1"), which is the whole bug class.
    const inputs = ["2026-10-01", "2026-9-5", "2026-09-05", "2026-1-1", "junk", ""];
    for (const a of inputs) {
      for (const b of inputs) {
        const ka = dayKey(a);
        const kb = dayKey(b);
        if (ka === "" || kb === "") continue;
        expect(ka.length).toBe(10);
        // Equal width + zero padding => lexicographic compare is chronological.
        expect(ka < kb).toBe(ka.localeCompare(kb) < 0);
      }
    }
  });
});

describe("monthKey", () => {
  it("yields the YYYY-MM prefix that a month filter compares against", () => {
    expect(monthKey("2026-9-5")).toBe("2026-09");
    expect(monthKey("2026-09-05")).toBe("2026-09");
    expect(monthKey("2026-12-1")).toBe("2026-12");
  });

  it("yields the empty string for a date it cannot read", () => {
    expect(monthKey(undefined)).toBe("");
    expect(monthKey("2026-09")).toBe("");
    expect(monthKey("2026-13-05")).toBe("");
  });

  it("never reports a different month for two spellings of the same day", () => {
    expect(monthKey("2026-9-5")).toBe(monthKey("2026-09-05"));
  });
});

describe("localDayKey / localMonthKey", () => {
  // A stored `date` is a bare LOCAL calendar day, so "now" must be resolved in
  // that same frame: toISOString() reports UTC and disagrees with the wall
  // calendar within a day of a month edge. TZ is restored in a finally block —
  // leaking it would decide the calendar frame of every later test in this
  // worker, and assigning undefined makes Node read the literal "undefined".
  it("resolves the operator's local day where UTC names a different one", () => {
    const prev = process.env.TZ;
    process.env.TZ = "Pacific/Kiritimati"; // UTC+14: local runs ahead of UTC
    try {
      const d = new Date("2026-09-30T12:00:00Z");
      expect(d.toISOString().slice(0, 10)).toBe("2026-09-30"); // the old derivation
      expect(localDayKey(d)).toBe("2026-10-01");
      expect(localMonthKey(d)).toBe("2026-10");
      // The invariant the views depend on: the clock key and a stored value's
      // key now live in the same space, so a month filter can actually match.
      expect(monthKey("2026-10-01")).toBe(localMonthKey(d));
    } finally {
      if (prev === undefined) delete process.env.TZ;
      else process.env.TZ = prev;
    }
  });

  it("reads local fields, so a local-midnight Date keeps its own day", () => {
    // Built from LOCAL components, so these hold in whichever zone the runner
    // uses; a UTC-based implementation would answer 30 Sep anywhere east of UTC.
    expect(localDayKey(new Date(2026, 9, 1, 0, 0, 0))).toBe("2026-10-01");
    expect(localMonthKey(new Date(2026, 9, 1, 23, 59, 59))).toBe("2026-10");
    expect(localDayKey(new Date(2026, 0, 5))).toBe("2026-01-05");
  });

  it("pads to fixed width and treats an invalid Date as unreadable", () => {
    expect(localDayKey(new Date("nope"))).toBe("");
    expect(localMonthKey(new Date("nope"))).toBe("");
    // Same "" contract as dayKey: the lowest key, so it sinks rather than guessing.
    expect(localDayKey(new Date(2026, 8, 6))).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });
});

// The WRITE-side rule. It is deliberately stricter than dayKey: dayKey may
// truncate a timestamp to its day because it only feeds a comparison, but
// rewriting stored data the same way would destroy the time the operator typed.
// canonicalDate therefore touches a value only when it is EXACTLY one whole
// date. The fixture table mirrors kernel/datalake's datewrite_test.go case for
// case, so the two implementations of this rule cannot drift silently.
describe("canonicalDate", () => {
  it("pads a whole date to fixed width, with either separator", () => {
    expect(canonicalDate("2026-9-5")).toBe("2026-09-05");
    expect(canonicalDate("2026/9/5")).toBe("2026-09-05");
    expect(canonicalDate("  2026-9-5  ")).toBe("2026-09-05");
    expect(canonicalDate("2026-09-05")).toBe("2026-09-05"); // idempotent
  });

  it("rewrites nothing that is not exactly one whole date", () => {
    // Truncating this would destroy the time component.
    expect(canonicalDate("2026-09-05T10:30")).toBe("2026-09-05T10:30");
    expect(canonicalDate("2026-9-5 extra")).toBe("2026-9-5 extra");
    // Out-of-range is never guessed at.
    expect(canonicalDate("2026-13-05")).toBe("2026-13-05");
    expect(canonicalDate("2026-00-05")).toBe("2026-00-05");
    expect(canonicalDate("2026-09-32")).toBe("2026-09-32");
    expect(canonicalDate("tomorrow")).toBe("tomorrow");
    expect(canonicalDate("")).toBe("");
  });
});
