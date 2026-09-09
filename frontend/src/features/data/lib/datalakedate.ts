// SPDX-License-Identifier: MIT

// Pure helpers for datalake date fields.
//
// WHY THIS EXISTS
// The data lake stores record fields verbatim — `Lake.Insert` documents
// "Fields are stored verbatim", `Lake.Update` merges the patch as-is, and
// neither `handleDataInsert` (kernel/controlplane/datalake.go) nor the agent's
// `db` tool (plugins/tools/db) validates a value against the schema's declared
// field type. The console editor's `coerce()` handles number/money/bool/tags
// but falls through to raw text for `date`, and there is no `type="date"`
// input anywhere in the frontend. So a `date` field is free text that only
// CONVENTIONALLY looks like `YYYY-MM-DD`: `2026-9-5` is a legal stored value,
// whether an agent wrote it or an operator typed it.
//
// A consumer that compares such a string lexicographically is then wrong in
// both directions:
//   - `"2026-9-5".startsWith("2026-09")` is false  -> the row is silently
//     dropped from a money total;
//   - `"2026-9-5" > "2026-10-01"` because '9' > '1' -> September outranks
//     October, so a "recent" list is mis-ordered.
//
// The fix is the invariant the comparison silently required: canonicalize to
// fixed-width `YYYY-MM-DD` BEFORE comparing. With equal width and zero-padded
// fields, lexicographic order IS chronological order. This mirrors
// internal/strutil's purpose for truncation and the `canonicalDailyAt` fix in
// kernel/workflow — one shared rule instead of a per-call-site re-derivation.

/** `dayKey` normalizes a stored `date` value to `YYYY-MM-DD`, or `""` when it
 *  carries no readable year-month-day triple. `""` is deliberately the lowest
 *  possible sort key, so an unparseable row sinks to the end of a descending
 *  list and matches no month prefix instead of being guessed at.
 *
 *  Note the scope: month and day are RANGE-checked (1-12, 1-31) but not
 *  calendar-validated per month, so "2026-02-30" keeps a key rather than being
 *  rejected. That is enough for ordering and month bucketing, which is all any
 *  consumer here needs; nothing treats the result as a real instant. */
export function dayKey(v: unknown): string {
  const s = String(v ?? "").trim();
  // Year first, then month/day of 1-2 digits each. The trailing part is not
  // anchored, so a stored timestamp ("2026-09-05T10:00") still yields its day.
  const m = /^(\d{4})[-/](\d{1,2})[-/](\d{1,2})/.exec(s);
  if (!m) return "";
  const month = Number(m[2]);
  const day = Number(m[3]);
  if (month < 1 || month > 12 || day < 1 || day > 31) return "";
  return `${m[1]}-${String(month).padStart(2, "0")}-${String(day).padStart(2, "0")}`;
}

/** `monthKey` is the `YYYY-MM` prefix of `dayKey`, or `""` when there is none. */
export function monthKey(v: unknown): string {
  return dayKey(v).slice(0, 7);
}

/** `localDayKey` is `d`'s LOCAL calendar day as canonical `YYYY-MM-DD`, or `""`
 *  for an invalid Date.
 *
 *  Comparing a stored `date` against "now" needs the same frame the stored value
 *  was written in, and a bare `2026-10-01` is a LOCAL wall-calendar day — it
 *  carries no zone. `new Date().toISOString()` is UTC, so within a day of a
 *  month edge the two frames name different months (and different days): an
 *  operator at UTC+14 is already in October while UTC still says September, and
 *  at UTC-4 the operator is still in September while UTC says October. The old
 *  UTC derivation was therefore wrong in BOTH directions, which is exactly why
 *  a fixed offset is not a fix — only the local getters agree with the stored
 *  values. Local getters are also the in-repo idiom for a wall date, see
 *  `unixToLocalInput` in views/Schedules.tsx. */
export function localDayKey(d: Date): string {
  if (Number.isNaN(d.getTime())) return "";
  const p = (n: number) => String(n).padStart(2, "0");
  return `${String(d.getFullYear()).padStart(4, "0")}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** `localMonthKey` is the `YYYY-MM` prefix of `localDayKey`, same frame rule. */
export function localMonthKey(d: Date): string {
  return localDayKey(d).slice(0, 7);
}

/** `canonicalDate` is the WRITE-side rule, and it is deliberately stricter than
 *  `dayKey`. `dayKey` may truncate a timestamp to its day because it only feeds
 *  a comparison; rewriting stored data that way would destroy the time the
 *  operator typed. So this touches a value only when it is EXACTLY one whole
 *  date, and returns the input unchanged otherwise: it never truncates, never
 *  guesses at an out-of-range date, and never rejects. It mirrors
 *  `canonicalDate` in kernel/datalake/datalake.go, and the two test fixture
 *  tables (datalakedate.test.ts and datewrite_test.go) are kept identical so
 *  the two implementations cannot drift silently across the language seam. */
export function canonicalDate(s: string): string {
  const m = /^(\d{4})[-/](\d{1,2})[-/](\d{1,2})$/.exec(s.trim());
  if (!m) return s;
  const month = Number(m[2]);
  const day = Number(m[3]);
  if (month < 1 || month > 12 || day < 1 || day > 31) return s;
  return `${m[1]}-${String(month).padStart(2, "0")}-${String(day).padStart(2, "0")}`;
}
