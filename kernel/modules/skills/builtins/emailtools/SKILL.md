---
name: email-tools
description: Send and read email — send a message (with attachments) over SMTP, and list or read messages over IMAP — when a task needs to email a report or notification, or check an inbox, using an account's own SMTP/IMAP credentials
triggers: [email, mail, smtp, imap, inbox, send mail, notify, report, attachment, message]
tools: [code_exec, shell]
---

# email-tools — send and read email

When a task needs to email something — deliver a report, notify on completion,
or check an inbox for a reply — use this. It speaks SMTP (send) and IMAP (read)
with an account's own credentials, using only the Python **standard library** —
no install. Runs through `code_exec` (python).

## No setup needed

`smtplib`, `imaplib`, and `email` ship with Python. Use `skill op=files
email-tools` to find the bundle directory.

## The helper

`scripts/mail.py` takes a JSON spec with an `op` and prints JSON. Ops:

```sh
# Send a message (with optional attachments):
python scripts/mail.py '{"op":"send",
  "smtp":{"host":"smtp.example.com","port":587,"user":"me@example.com","pass":"$APP_PW","starttls":true},
  "from":"me@example.com","to":["you@example.com"],"subject":"Report","body":"See attached.",
  "attachments":["report.pdf"]}'

# List recent messages in a folder:
python scripts/mail.py '{"op":"list",
  "imap":{"host":"imap.example.com","user":"me@example.com","pass":"$APP_PW"},
  "folder":"INBOX","search":"UNSEEN","limit":20}'

# Read one message by uid:
python scripts/mail.py '{"op":"read",
  "imap":{"host":"imap.example.com","user":"me@example.com","pass":"$APP_PW"},
  "folder":"INBOX","uid":"1234"}'
```

### Spec fields
- `op` — `send` | `list` | `read`.
- `smtp` (send) — `{host, port=587, user, pass, starttls=true, ssl=false}`.
- `from`, `to` (string or list), `cc`, `subject`, `body` (plain), `html`
  (optional HTML alternative), `attachments` (file paths).
- `imap` (list/read) — `{host, port=993, user, pass}` (IMAP-over-SSL).
- `folder` (default `INBOX`), `search` (IMAP search, default `ALL`; e.g.
  `UNSEEN`, `FROM "x@y"`), `limit` (list, default 20), `uid` (read).

### Output (JSON on stdout)
```
{ "ok": true, "op": "send", "to": ["you@example.com"] }
{ "ok": true, "op": "list", "messages": [ {uid, from, subject, date} ], "count": 7 }
{ "ok": true, "op": "read", "from": "...", "subject": "...", "date": "...", "text": "..." }
```

## Credentials & safety

- Use an **app password** (Gmail/Outlook require one for SMTP/IMAP), not the main
  account password. Pass it via the spec, ideally from an env var you set first
  (`export APP_PW=...` then `$APP_PW` — expand it in the shell before the call).
- The helper never echoes the password back in its output.
- Sending email is outward-facing — double-check the recipient before a `send`.

## Going further

The helper is a fast start, not a cage — for OAuth2 (XOAUTH2), calendar invites,
or rich multipart, use `smtplib`/`imaplib`/`email` directly. Pair with
**pdf-tools**/**data-analysis** (generate a report, then attach + send it) and
**archive-tools** (zip outputs into one attachment). See `reference/recipes.md`.
