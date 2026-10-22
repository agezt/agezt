# calendar-tools recipes

The helper (`scripts/cal.py`) creates and parses `.ics` with stdlib only. For
recurrence (RRULE), timezones (VTIMEZONE), or VTODO/VALARM, use the `icalendar`
library directly.

## Create a meeting and email it as an invite

```sh
python scripts/cal.py '{"op":"create","out":"invite.ics","events":[
  {"summary":"Design sync","start":"2026-06-18T10:00:00","end":"2026-06-18T10:30:00",
   "tz":"UTC","location":"Meet","organizer":"me@corp.com","attendees":["you@corp.com"]}
]}'
```
Then send with **email-tools** (`"attachments":["invite.ics"]`). Most clients
render an attached `.ics` as an accept/decline invite.

## All-day event / reminder

```sh
python scripts/cal.py '{"op":"create","out":"holiday.ics","events":[
  {"summary":"Release freeze","start":"2026-07-01","all_day":true}
]}'
```

## Multiple events in one file

```sh
python scripts/cal.py '{"op":"create","out":"week.ics","events":[
  {"summary":"Standup","start":"2026-06-15T09:00:00","end":"2026-06-15T09:15:00"},
  {"summary":"Standup","start":"2026-06-16T09:00:00","end":"2026-06-16T09:15:00"}
]}'
```
(For *true* recurrence with one VEVENT + RRULE, use the `icalendar` library.)

## Read an invite you received

```sh
python scripts/cal.py '{"op":"parse","path":"invite.ics"}'
# → {events:[{uid,summary,start,end,location,description,attendees}]}
```
Store the parsed events in the Data Lake (a `calendar` collection) or hand them
to **data-analysis** to summarize a busy week.

## Time handling
- `start`/`end` are ISO: `2026-06-15T14:00:00`.
- Add `"tz":"UTC"` per event to stamp UTC (`...Z`); without it the time is
  floating (interpreted in the viewer's local zone).
- `all_day:true` uses a date only (`VALUE=DATE`).

## Recurrence (helper doesn't cover it)

```python
# pip install icalendar
from icalendar import Calendar, Event
from datetime import datetime
cal = Calendar(); ev = Event()
ev.add("summary", "Weekly 1:1"); ev.add("dtstart", datetime(2026,6,15,9,0))
ev.add("rrule", {"freq": "weekly", "count": 12})
cal.add_component(ev); open("recurring.ics","wb").write(cal.to_ical())
```
