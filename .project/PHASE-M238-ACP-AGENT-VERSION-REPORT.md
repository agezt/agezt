# M238 — ACP server reports the real product version

## Why
The ACP server (`kernel/acp`) is how Claude Code / Codex / Gemini CLI and other
IDEs drive agezt over the Agent Client Protocol. Its `initialize` response
carries `agentInfo` — the agent's identity shown to the editor. The `version`
there was a hardcoded literal `"0.1.0"`, so an IDE connecting to a **v1.0.0**
daemon displayed "agezt 0.1.0" — a misleading, stale version.

This literal was deliberately skipped during the v1.0.0 cut (M220) on the
reasoning that it was "a separate version field." On review that was wrong:
`agentInfo.version` is precisely the *agent's (product's) version* reported to
the client. The genuinely separate field — the ACP wire `protocolVersion` — is
already a proper constant and is untouched.

## What
- **`kernel/acp/acp.go`** — `handleInitialize` now builds `agentInfo` from
  `internal/brand`: `name = brand.Binary` (`"agezt"`, the same value as before)
  and `version = brand.Version` (the real release, currently `1.0.0`). De-hardcoded
  so it tracks every future version bump automatically.

`skill.DefaultVersion` ("0.1.0") was intentionally **not** changed — that is a
genuinely separate field (the starting version assigned to a newly-authored
skill, like an `init` default), unrelated to agezt's own release.

## Files
- `kernel/acp/acp.go` — `brand` import + `agentInfo` from brand (edited).
- `kernel/acp/acp_test.go` — `TestInitialize` strengthened (new): asserts
  `agentInfo.version == brand.Version` (and `name == brand.Binary`), so the
  version can never silently drift from the release again.

## Verification
- `go test ./kernel/acp/` — green; full suite still green, 66 packages. No new
  test *function* was added (an assertion was added to the existing
  `TestInitialize`), so the README test tally is unchanged at 1779.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./kernel/acp/` clean.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.
- **Proof:** the test drives the real `initialize` dispatch and asserts the
  reply's `agentInfo.version` equals `brand.Version` — exactly what an IDE
  receives over ACP.

## Scope notes
- ACP `protocolVersion` (the wire-contract version) is and stays a constant —
  it must not track the product version.
