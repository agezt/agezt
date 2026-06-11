# PHASE M895 — Built-in calendar-tools skill bundle

**Status:** shipped
**Milestone:** M895 (session range M889–M899; branched from `origin/main`,
concurrent local-main arc untouched).
**Theme:** Backlog **#34** — a fifteenth built-in skill bundle: create and parse
iCalendar `.ics` events, the scheduling complement to email-tools.

## What shipped

A built-in `calendar-tools` bundle, seeded active at startup by the existing
`plugins/builtinskills` seeder (`builtinBundles` + `go:embed` only). **Zero pip
deps** — stdlib `datetime` + text, so there's no `setup.sh`:

- `SKILL.md` — create/parse ops, time handling (ISO in; per-event `tz:"UTC"` or
  floating; `all_day`), the "send an invite via email-tools" flow, and the
  best-effort-parse / use-`icalendar`-for-RRULE caveat.
- `scripts/cal.py` — one JSON-spec helper, two ops: `create` (writes a valid
  VCALENDAR/VEVENT — UID/DTSTAMP/DTSTART/DTEND/SUMMARY/LOCATION/DESCRIPTION/
  ORGANIZER/ATTENDEE, with RFC 5545 text escaping and 75-octet line folding),
  `parse` (unfolds lines, extracts VEVENT fields back to structured events).
- `reference/recipes.md` — meeting invite + email-tools, all-day reminder, multi
  events, read a received invite, time handling, the `icalendar` RRULE pointer.

## Why this milestone is conflict-free

A new bundle touches **only** `plugins/builtinskills/`. The seeder auto-loads it.
It tests in isolation: `go test ./plugins/builtinskills/`. Branched from
`origin/main` (my M862–M894), concurrent local-main arc untouched.

## Verification

- **Isolated gate:** `go vet` clean; linux cross-build of `./plugins/builtinskills/`
  clean; `gofmt -l` empty. Package suite green — `TestSeedAll_InstallsCalendarTools`
  asserts the bundle seeds **active** and materializes `cal.py` / `recipes.md`;
  bundle-count assertions now cover fifteen bundles.
- **Functional smoke (stdlib, ran locally):** `create` an event with a comma in
  the summary ("Sprint, review") + UTC times + location → `parse` round-trips it
  back exactly (comma correctly escaped on write, unescaped on read; UID/DTSTART/
  DTEND/LOCATION all present).
- No new Go dep; no new env. `.gitattributes` already forces LF on
  `plugins/builtinskills/**`. Full `go build ./...` deliberately skipped.

## Notes
- Fifteen seeded bundles now ship. calendar-tools + email-tools = "schedule a
  meeting and send the invite"; parsed events can land in a Data Lake `calendar`
  collection or feed data-analysis. RRULE recurrence is deliberately out of scope
  (pointer to `icalendar`).
