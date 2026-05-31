# Phase Report — Milestone M15 (Secret redaction at the journal boundary)

> Status: **Phases 1–3 shipped** · Date: 2026-05-31
> SPEC-06 / ROADMAP "redaction must work before Initiative can act
> autonomously." Phase 1: the redaction substrate (`kernel/redact`) + the bus
> seam that scrubs every durably-published event before it is hashed and written.
> Phase 2: the daemon wiring — on by default, seeded from the creds vault,
> installed on the primary and every tenant bus. Streaming-token redaction is a
> named follow-up.

## Why this milestone

The journal is append-only and **hash-chained**: whatever is written there is
permanent and tamper-evident. That is exactly what makes it dangerous for
secrets. A secret can reach an event payload through many innocent paths — an API
key echoed in a shell tool's stdout, a token pasted into a prompt, a credential
returned in an HTTP tool's response body, an `Authorization` header in a debug
dump. Once such a payload is published, the secret is in the permanent record
forever, replayable by anyone who can read the journal.

Before M15 the only redaction in the system was `creds.MaskValue`, used purely
for **CLI display** (it keeps the first four characters). Nothing scrubbed the
**persisted** event stream. M15 closes that hole at the one place every durable
event must pass through: the bus.

## What shipped — `kernel/redact`

A pure, deterministic `Redactor` (stdlib only: `regexp`, `sort`, `strings`,
`sync`) that scrubs on two signals, replacing each hit with `[REDACTED]`:

- **Literals.** Exact secret values the daemon knows (the configured provider
  keys). Scrubbed wherever they appear — mid-string, nested in JSON. Values
  shorter than 8 chars are ignored (too likely to be ordinary substrings);
  the set is sorted longest-first so a secret that is a prefix of another can't
  be left partially exposed. `SetSecrets` updates the set (e.g. after a creds
  rotation) without rebuilding the redactor.
- **Patterns.** High-confidence secret *formats* that catch secrets the daemon
  was never told about: OpenAI/Anthropic `sk-…` (the dash class covers
  `sk-ant-…`/`sk-proj-…`), AWS `AKIA…`, GitHub `gh[pousr]_…`, Slack `xox[baprs]-…`,
  Google `AIza…`, `Bearer <token>`, and PEM `PRIVATE KEY` blocks. Each pattern is
  deliberately specific (a long, structured token shape) so a full-match
  replacement can't corrupt ordinary prose.

Redaction is a pure function of (input, literal set), so a redacted payload
**hashes stably** and replay is unaffected — the journal already holds the
redacted form; nothing re-redacts on read.

## The bus seam

`Bus.SetRedactor` installs a redactor (a narrow `Redactor` interface, so `bus`
takes no dependency on `redact`). `Bus.Publish` runs `redactSpecLocked` on the
spec **before** `journal.Append`: the payload is re-marshaled to redacted JSON
(replacing it with a `json.RawMessage`, so the downstream marshal is a no-op and
the JSON stays valid) and each tag value is scrubbed. The caller's payload/tags
are never mutated (payload is replaced wholesale; tags are copied). Nil redactor
(the default) is a complete no-op — existing single-binary behavior is byte-for-
byte unchanged.

`PublishStreaming` (ephemeral, display-only LLM token chunks, never journaled) is
**not** redacted in Phase 1: those events are not part of the permanent record,
and scrubbing token-by-token is unreliable across chunk boundaries. Named as a
follow-up.

## Proven

- **Unit (`kernel/redact`):** every pattern redacts its secret and leaves a
  placeholder; PEM blocks scrub their body while preserving surrounding text;
  literals scrub exact values; short/empty literals are ignored; ordinary prose
  (and below-threshold near-misses like `sk-too-short`) is untouched;
  `RedactBytes` returns the same slice when nothing matched and keeps redacted
  JSON valid (non-secret fields intact).
- **Integration (`kernel/bus`):** with a redactor set, a published `tool.result`
  carrying `sk-…` in `stdout` and a literal tenant key — plus a secret in a tag —
  comes back redacted, the **journaled** event is redacted AND its hash verifies
  over the redacted bytes, and the non-secret field survives. Without a redactor,
  the payload is journaled verbatim (default unchanged).

9 new tests; suite **1139** green, `go vet` clean, `GOOS=linux` builds,
`go.mod` unchanged.

## Phase 2 — daemon wiring

The redactor is now live in the daemon, on by default:

- **Built at startup**, seeded with the vault's secret *values* (`credSecrets` →
  `SetSecrets`) plus the built-in patterns. `AGEZT_REDACT=off` disables it (the
  bus then runs with a nil redactor — the unchanged path). A banner line reports
  the state (`redaction: enabled (N vault secrets + built-in patterns)`).
- **Installed on every bus before any run:** the primary kernel bus right after
  `gov.SetBus`, and each tenant kernel bus inside the tenant `OpenFunc`. So a
  multi-tenant deployment scrubs every tenant's journal too, with one redactor.
- **Refreshed on reload:** the existing `OnReload` hook re-seeds the literal set
  after a creds rotation, so a newly-added/rotated key is scrubbed as a literal
  from then on (the patterns already covered it regardless).

**Proven live (mock provider, fresh base dir):** the daemon boots with
`redaction: enabled (0 vault secrets + built-in patterns)`; an `agt run` whose
shell tool echoes `MYKEY=sk-abcdefghijklmnopqrstuvwxyz0123` produces a journal
where the **raw secret is absent** and `[REDACTED]` is present. Crucially, a scan
of the *entire base dir* — not just `journal/` but `state/`, `memory/`,
`worldmodel/` — finds the raw secret **nowhere**: because those stores are
event-sourced projections fed by the bus, redacting at the bus chokepoint
protects every downstream store at once.

## Phase 3 — operator-extensible literals

Real deployments have secrets the provider vault never holds and the built-in
patterns can't recognise — internal API tokens, DB passwords, service
credentials. `AGEZT_REDACT_EXTRA` (a `;`-separated list, trimmed, empties
dropped) adds those as literal secrets alongside the vault values, seeded at
startup and re-seeded on reload. The banner reports the count
(`…, 2 via AGEZT_REDACT_EXTRA`).

**Proven live:** with `AGEZT_REDACT_EXTRA="HUNTER2-internal-db-pw;
corp-token-9f8e7d6c5b4a"`, a run that echoes both internal secrets (neither
matches a pattern nor lives in the vault) leaves **neither** anywhere in the base
dir.

## Deferred — later phases (named)

- **Streaming-token redaction** for the live display path (defense-in-depth for
  secrets fully contained in one chunk; cross-chunk secrets need stream
  buffering, which would change streaming semantics — hence deferred).
- **Custom regex rules** (operator-supplied patterns, validated/compiled at
  startup) for site-specific secret *shapes* — the literal half shipped in
  Phase 3; arbitrary regexes need careful compile-error handling to avoid an
  over-redacting or daemon-failing footgun.
- **Per-tenant secret sets** (today all tenants share the primary redactor's
  literals + patterns; a tenant with its own vault would seed its own literals).
