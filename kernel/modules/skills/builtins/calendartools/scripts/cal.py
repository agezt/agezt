#!/usr/bin/env python3
"""calendar-tools helper — create and parse .ics (iCalendar) files. Stdlib only.

Usage:  python cal.py '<json-spec>'   (or pipe the JSON on stdin)
Ops:
  create {out, events:[{summary,start,end?,location?,description?,attendees?,
          organizer?,uid?,all_day?,tz?}]}            -> {out, count}
  parse  {path}                                       -> {events:[...], count}

Text fields are escaped and long lines folded per RFC 5545. parse is best-effort
over VEVENT blocks. For RRULE/VTIMEZONE, use the `icalendar` library directly.
"""
import datetime as dt
import json
import sys


def read_spec():
    if len(sys.argv) > 1 and sys.argv[1].strip():
        return json.loads(sys.argv[1])
    data = sys.stdin.read()
    if data.strip():
        return json.loads(data)
    raise ValueError("no spec: pass a JSON spec as argv[1] or on stdin")


def _esc(s):
    return (
        str(s)
        .replace("\\", "\\\\")
        .replace(";", "\\;")
        .replace(",", "\\,")
        .replace("\n", "\\n")
    )


def _fold(line):
    """RFC 5545 line folding at 75 octets (continuation lines start with a space)."""
    out = []
    while len(line.encode("utf-8")) > 75:
        # back off to <=75 bytes
        cut = 75
        while len(line[:cut].encode("utf-8")) > 75:
            cut -= 1
        out.append(line[:cut])
        line = " " + line[cut:]
    out.append(line)
    return "\r\n".join(out)


def _fmt_dt(value, all_day, tz):
    if all_day:
        d = dt.date.fromisoformat(value[:10])
        return ";VALUE=DATE:" + d.strftime("%Y%m%d")
    d = dt.datetime.fromisoformat(value)
    if tz and tz.upper() == "UTC":
        return ":" + d.strftime("%Y%m%dT%H%M%SZ")
    return ":" + d.strftime("%Y%m%dT%H%M%S")


def op_create(spec):
    events = spec.get("events") or []
    if not events:
        raise ValueError("create needs events[]")
    stamp = dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    lines = ["BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//agezt//calendar-tools//EN", "CALSCALE:GREGORIAN"]
    for i, ev in enumerate(events):
        if not ev.get("summary"):
            raise ValueError("each event needs a summary")
        if not ev.get("start"):
            raise ValueError("each event needs a start")
        all_day = bool(ev.get("all_day"))
        tz = ev.get("tz")
        uid = ev.get("uid") or f"agezt-{stamp}-{i}@agezt"
        lines.append("BEGIN:VEVENT")
        lines.append(_fold("UID:" + uid))
        lines.append("DTSTAMP:" + stamp)
        lines.append(_fold("DTSTART" + _fmt_dt(ev["start"], all_day, tz)))
        if ev.get("end"):
            lines.append(_fold("DTEND" + _fmt_dt(ev["end"], all_day, tz)))
        lines.append(_fold("SUMMARY:" + _esc(ev["summary"])))
        if ev.get("location"):
            lines.append(_fold("LOCATION:" + _esc(ev["location"])))
        if ev.get("description"):
            lines.append(_fold("DESCRIPTION:" + _esc(ev["description"])))
        if ev.get("organizer"):
            lines.append(_fold("ORGANIZER:mailto:" + str(ev["organizer"])))
        for a in ev.get("attendees") or []:
            lines.append(_fold("ATTENDEE;RSVP=TRUE:mailto:" + str(a)))
        lines.append("END:VEVENT")
    lines.append("END:VCALENDAR")
    out = spec.get("out", "event.ics")
    with open(out, "w", encoding="utf-8", newline="") as fh:
        fh.write("\r\n".join(lines) + "\r\n")
    return {"out": out, "count": len(events)}


def _unfold(text):
    # Join continuation lines (a line starting with space/tab continues the prior).
    raw = text.replace("\r\n", "\n").split("\n")
    out = []
    for line in raw:
        if line[:1] in (" ", "\t") and out:
            out[-1] += line[1:]
        else:
            out.append(line)
    return out


def _unesc(s):
    return s.replace("\\n", "\n").replace("\\,", ",").replace("\\;", ";").replace("\\\\", "\\")


def op_parse(spec):
    path = spec.get("path")
    if not path:
        raise ValueError("parse needs path")
    with open(path, "r", encoding="utf-8") as fh:
        lines = _unfold(fh.read())
    events, cur = [], None
    for line in lines:
        if line == "BEGIN:VEVENT":
            cur = {}
        elif line == "END:VEVENT":
            if cur is not None:
                events.append(cur)
            cur = None
        elif cur is not None and ":" in line:
            key, val = line.split(":", 1)
            name = key.split(";", 1)[0].upper()
            if name == "SUMMARY":
                cur["summary"] = _unesc(val)
            elif name == "DTSTART":
                cur["start"] = val
            elif name == "DTEND":
                cur["end"] = val
            elif name == "LOCATION":
                cur["location"] = _unesc(val)
            elif name == "DESCRIPTION":
                cur["description"] = _unesc(val)
            elif name == "UID":
                cur["uid"] = val
            elif name == "ATTENDEE":
                cur.setdefault("attendees", []).append(val.replace("mailto:", ""))
    return {"events": events, "count": len(events)}


OPS = {"create": op_create, "parse": op_parse}


def run(spec):
    op = spec.get("op")
    if op not in OPS:
        raise ValueError("spec.op must be one of: " + ", ".join(OPS))
    result = OPS[op](spec)
    result.update({"ok": True, "op": op})
    return result


def main():
    try:
        print(json.dumps(run(read_spec()), default=str))
    except Exception as e:  # noqa: BLE001
        print(json.dumps({"ok": False, "error": str(e)}))
        sys.exit(1)


if __name__ == "__main__":
    main()
