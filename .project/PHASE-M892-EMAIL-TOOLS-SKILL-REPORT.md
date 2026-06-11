# PHASE M892 — Built-in email-tools skill bundle

**Status:** shipped
**Milestone:** M892 (this session's range is M889–M899; branched from
`origin/main` to keep clear of the concurrent session's diverged local `main`).
**Theme:** Backlog **#34** — a twelfth built-in skill bundle: send (SMTP) and
read (IMAP) email, the delivery step of the reporting pipeline.

## What shipped

A built-in `email-tools` bundle, seeded active at startup by the existing
`plugins/builtinskills` seeder (`builtinBundles` + `go:embed` only). **Zero pip
dependencies** — stdlib `smtplib`/`imaplib`/`email`, so there's no `setup.sh`:

- `SKILL.md` — send/list/read ops, common provider host table, app-password
  guidance, the "recipient is outward-facing, double-check" note.
- `scripts/mail.py` — one JSON-spec helper, three ops: `send` (SMTP with
  STARTTLS/SSL, plain + optional HTML alternative, file attachments), `list`
  (IMAP-over-SSL, newest-first summaries with uid/from/subject/date, honours an
  IMAP `search` like `UNSEEN`), `read` (full message by uid → decoded text body).
  Headers are RFC2047-decoded; the password is never echoed back.
- `reference/recipes.md` — provider hosts, send-a-report-with-attachment, notify,
  check-unread-then-read, IMAP search syntax, HTML email, the reporting pipeline.

## Why this milestone is conflict-free

A new bundle touches **only** `plugins/builtinskills/`. The seeder auto-loads it.
It tests in isolation: `go test ./plugins/builtinskills/`. Branched from
`origin/main` (which carries my M862–M891), leaving the concurrent session's
local `main` (their M880–M901 arc, unpushed) untouched per the owner's call.

## Verification

- **Isolated gate:** `go vet` clean; linux cross-build of `./plugins/builtinskills/`
  clean; `gofmt -l` empty; `python -m py_compile mail.py` passes. Package suite
  green — `TestSeedAll_InstallsEmailTools` asserts the bundle seeds **active** and
  materializes `mail.py` / `recipes.md`; bundle-count assertions now cover twelve
  bundles.
- No new Go dep; no new env. `.gitattributes` already forces LF on
  `plugins/builtinskills/**`. Full `go build ./...` deliberately skipped (the
  concurrent kernel arc lives on local `main`, not this branch's base).

## Notes
- Twelve seeded bundles now ship. email-tools is the reporting pipeline's delivery
  step: generate a chart (data-analysis) / PDF (pdf-tools) / zip (archive-tools),
  then attach and send it. SMTP-send is outward-facing — the SKILL flags the
  recipient double-check.
