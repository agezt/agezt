---
name: calendar-tools
description: Create and read calendar events — write a standards-compliant .ics file (VEVENT) for a meeting or reminder, and parse an .ics back into structured events — when a task needs to schedule something, send a meeting invite, or read a calendar file
triggers: [calendar, ics, ical, event, meeting, invite, schedule, appointment, reminder, vevent]
tools: [code_exec, shell, artifacts]
---

# calendar-tools — make and read .ics calendar events

When a task needs to schedule something — send a meeting invite, drop a reminder
on a calendar, or read events out of an `.ics` file — use this. It writes and
parses the iCalendar (`.ics`) format using only the Python **standard library** —
no install. Runs through `code_exec` (python).

## No setup needed

Built on `datetime` + plain text. Use `skill op=files calendar-tools` to find the
bundle directory.

## The helper

`scripts/cal.py` takes a JSON spec with an `op` and prints JSON. Ops:

```sh
# Create an .ics with one or more events:
python scripts/cal.py '{"op":"create","out":"meeting.ics","events":[
  {"summary":"Sprint review","start":"2026-06-15T14:00:00","end":"2026-06-15T15:00:00",
   "location":"Zoom","description":"Demo + retro","attendees":["a@x.com","b@x.com"]}
]}'

# Parse an .ics file back into structured events:
python scripts/cal.py '{"op":"parse","path":"invite.ics"}'
```

### Spec fields
- `op` — `create` | `parse`.
- `out` (create) — output `.ics` path (default `event.ics`).
- `events` (create) — list of `{summary, start, end?, location?, description?,
  attendees?, organizer?, uid?, all_day?}`. `start`/`end` are ISO
  (`2026-06-15T14:00:00`); add `"tz":"UTC"` per event to stamp UTC (`Z`),
  otherwise the time is floating/local. `all_day:true` uses a date only.
- `path` (parse) — the `.ics` file to read.

### Output (JSON on stdout)
```
{ "ok": true, "op": "create", "out": "meeting.ics", "count": 1 }
{ "ok": true, "op": "parse", "events": [ {uid, summary, start, end, location, description} ], "count": 2 }
```

## Send an invite (with email-tools)

`create` an `.ics`, then attach it with the **email-tools** skill — most clients
render an attached `.ics` as an accept/decline invite:

```sh
python scripts/cal.py '{"op":"create","out":"invite.ics","events":[{...}]}'
# then email-tools send with "attachments":["invite.ics"]
```

## Notes

- Text fields are escaped (`,` `;` `\` newlines) and long lines are folded per
  RFC 5545, so the output validates in real calendar clients.
- `parse` is best-effort over `VEVENT` blocks (handles line unfolding) — good for
  reading invites; for recurring rules (`RRULE`), timezones (`VTIMEZONE`), or
  complex calendars, use the `icalendar` library directly in `code_exec`.

## Going further

The helper is a fast start, not a cage — for RRULE recurrence, VTODO/VALARM, or
timezone databases, `pip install icalendar` and use it directly. Save the `.ics`
with the `artifacts` tool so it shows in Files. See `reference/recipes.md`.
