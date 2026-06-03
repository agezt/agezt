# M218 — Webhook HMAC secret containing `|` is no longer truncated

## Why
An outbound webhook sink is configured as `url|subject|secret` in `AGEZT_WEBHOOKS`. The
`secret` is the HMAC-SHA256 signing key whose value the receiver uses to verify the
`X-Agezt-Signature` of each delivery. `ParseSinks` split each entry with an **unbounded**
`strings.Split(entry, "|")` and took `parts[2]` as the secret. So a secret that itself
contained a `|` — perfectly legal for an arbitrary HMAC key / shared secret — was split
further: only the text up to the third `|` survived, and the rest was silently dropped.

The corrupted key is the worst kind of bug: nothing errors. The daemon signs every
delivery with the *truncated* key, the receiver verifies with the *full* key, every
signature mismatches, and the operator sees deliveries silently rejected (or, if the
receiver fails open, accepted but "untrusted") with no clue why.

## What
`kernel/webhook/webhook.go` (`ParseSinks`):
- Replace `strings.Split(entry, "|")` with `strings.SplitN(entry, "|", 3)`. The secret is
  the **last** field, so capping the split at 3 keeps everything after the second `|` —
  including any `|` characters — as the secret. The URL (`parts[0]`) and subject
  (`parts[1]`) fields are unchanged; a normal secret without a pipe is unaffected.

This pairs with M217 (subject-filter validation) — both fixes harden the same
`AGEZT_WEBHOOKS` parser against silent delivery failures.

## Tests (+1)
`kernel/webhook/webhook_test.go`:
- `TestParseSinks_SecretWithPipe` — `https://h/a|agent.>|se|cr|et` parses to one sink with
  secret `se|cr|et` (the full value after the second pipe) and subject `agent.>`.

The existing `TestParseSinks` (and the M217 filter tests) all use secrets without a pipe
and pass unchanged, confirming no regression for the common case.

## Verification
- `go test ./...` — 1685 passing (1684 + 1 new), 0 failing.
- `go vet ./kernel/webhook/` — clean.
- `gofmt -l` (CRLF-normalized) clean on touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only).
- Local commit only (no push); standard trailer.

## Files
- `kernel/webhook/webhook.go` — bounded split in `ParseSinks`.
- `kernel/webhook/webhook_test.go` — secret-with-pipe test.

## Theme
Config-hygiene thread (M215 peers dup-name, M216 plugin pins/allowlist dup-prefix, M217
webhook subject-filter validation, M218 webhook secret preservation): catch silent
misconfigurations / silent corruption at the parse boundary instead of letting them
surface as baffling runtime behaviour. The daemon's user-facing env-spec parsers have now
been swept for this class.
