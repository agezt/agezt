# M216 — Reject a duplicate prefix in the plugin pin / tool-allowlist specs

## Why
While fixing the duplicate-peer-name silent overwrite (M215), the same class of bug
turned up in two **security-relevant** env-spec parsers:

- `ParsePinSpec` (`AGEZT_PLUGIN_PINS`) — maps a plugin prefix → the BLAKE3-256 hash its
  binary must match. Binary-integrity control.
- `ParseToolAllowlistSpec` (`AGEZT_PLUGIN_TOOLS`) — maps a plugin prefix → the list of
  tools it may advertise. Capability-restriction control.

Both did `out[prefix] = value` with no collision check, so a typo or a copy-paste
duplicate (`search=hashA,scrape=…,search=hashB`) silently kept the **last** value and
discarded the operator's intended one. For these controls that's a real hazard: a lost
pin could let a *different* binary pass (or reject the legitimate one), and a lost
allowlist could unexpectedly widen or narrow what a plugin is permitted to expose — all
with no signal, discovered only as confusing runtime behaviour.

## What
`kernel/plugin/pinspec.go`:
- `ParsePinSpec` — before storing, reject a prefix already present:
  `plugin: pin for "<prefix>" is defined more than once`.
- `ParseToolAllowlistSpec` — likewise:
  `plugin: tool-allowlist for "<prefix>" is defined more than once`.

Both are hard errors caught at daemon startup (the boot path validates these specs),
matching how every other malformed-spec case in this file already behaves — and the same
fix shape as M215 for `AGEZT_PEERS`.

## Tests (+2)
- `kernel/plugin/pin_test.go` — `TestParsePinSpec_RejectsDuplicatePrefix`:
  `search=<a>,scrape=<b>,search=<b>` → error naming `search` and "more than once".
- `kernel/plugin/allowlist_test.go` — `TestParseToolAllowlistSpec_RejectsDuplicatePrefix`:
  `search=a+b,scrape=c,search=d` → error naming `search` and "more than once".

The existing pin/allowlist tests (basic parse, bad format, empty, unused-prefix diff)
remain and pass.

## Verification
- `go test ./...` — 1682 passing (1680 + 2 new), 0 failing.
- `go vet ./kernel/plugin/` — clean.
- `gofmt -l` (CRLF-normalized) clean on touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only).
- Local commit only (no push); standard trailer.

## Files
- `kernel/plugin/pinspec.go` — duplicate-prefix guard in both parsers.
- `kernel/plugin/pin_test.go`, `kernel/plugin/allowlist_test.go` — duplicate-prefix tests.

## Theme
M215 (peers) + M216 (plugin pins/allowlist) harden the daemon's env-spec parsers against
silent duplicate-key overwrites — a quiet-misconfiguration class that's especially
dangerous for the security controls (binary pinning, tool allow-listing). The remaining
map-keyed env specs were checked: `AGEZT_SCHEDULE` jobs are an append-only list (no key,
no overwrite), so no equivalent bug there.
