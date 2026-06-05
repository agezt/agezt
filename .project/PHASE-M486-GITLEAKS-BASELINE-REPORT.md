# M486 — gitleaks clean baseline (16 → 0), secret gate made enforceable

## Context
Part of the OBJECTIVE-GATE arc. `gitleaks detect` over the full history (554
commits) reported **16 leaks**. A secret-scan gate is only useful if it is clean:
16 standing hits mean a future *real* leak is indistinguishable from known noise.

## Triage — all 16 are deliberate test fixtures (no real secret)
| File | rule(s) | what it is |
|------|---------|-----------|
| `kernel/redact/redact_test.go` | jwt, github-pat | inputs proving the redactor scrubs JWTs / GitHub PATs |
| `kernel/redact/redact_m228_test.go` | generic-api-key, slack-app-token | provider keys + Slack tokens the redactor must mask |
| `kernel/redact/redact_m231_test.go` | generic-api-key | more redaction fixtures |
| `cmd/agezt/plugin_log_test.go` | generic-api-key | `sk-…` / `gsk_…` keys; test asserts they do **not** appear in log output |
| `kernel/creds/aws_test.go` | generic-api-key | placeholder AWS creds (`AKIA111111111111`, `secret1234567890`) for credential-file / IMDS parsing |

Every value is obviously synthetic. The redact package's entire job is to contain
and scrub such strings, so its tests *must* embed them; the creds test needs
parseable placeholder credentials. Confirmed by reading each match.

## Fix — scoped allowlist
Added `.gitleaks.toml`:
- `[extend] useDefault = true` keeps the full default ruleset active.
- `[allowlist] paths` allowlists **only** the three test paths above
  (`kernel/redact/.*_test\.go$`, `cmd/agezt/plugin_log_test\.go$`,
  `kernel/creds/aws_test\.go$`). Production code and every other test stay in scope.

Result: `gitleaks detect --no-banner --redact -s .` → **no leaks found** (exit 0).

## Negative control (git mode — the mode CI runs)
Removed the `kernel/creds/aws_test.go` path from the allowlist and re-ran git-mode
detect: **exactly its 2 findings reappeared** (and only those). Restored the path:
back to "no leaks found". Config restored byte-identical. This proves the allowlist
is path-scoped — it suppresses only the named fixtures, not the ruleset — so a
secret introduced in production code, or in any non-allowlisted test, still trips
the scan.

(Note: `--no-git` filesystem-scan mode on Windows emits backslash paths that the
forward-slash path regex does not match; git mode — used by CI and by the default
`gitleaks detect` — matches correctly, as the negative control demonstrates.)

## Verification / gate
- `gitleaks detect` exit 0 (was 16 leaks).
- `.gitleaks.toml` is configuration, not Go — `go vet` / build / test surface
  unchanged; `go.mod` / `go.sum` unchanged.
- The gate is now enforceable: any new secret outside the three test paths fails.
