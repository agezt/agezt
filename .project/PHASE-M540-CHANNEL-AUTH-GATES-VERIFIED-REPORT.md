# M540 — Verify inbound channel authorization gates (all 5 channels)

## Context
The channel adapters are the surfaces where *external* users reach the agent — the
highest-stakes authorization boundary after the control plane. Each delegates to the
verified `kernel/channel.Allowlist` (M511) and applies it fail-closed before the agent
handler runs. This milestone verifies that gating by negative control across every inbound
channel. `GOMAXPROCS=3`.

## All five gates verified solid
Each channel computes `allowed := c.allow.Allows(<id>)` and refuses a non-allowlisted
sender before the agent runs. Mutating the gate `if !allowed { … return }` → `if allowed`
(which would let a non-allowlisted sender drive the agent and refuse the allowlisted one)
was applied per channel and run against each suite — **all killed**:

| Channel | Gate | Result |
|---|---|---|
| telegram | `if !allowed` (+ photo-fetch `allowed &&` guard) | **killed** (both) |
| discord | `if !allowed` (interaction webhook, after Ed25519 verify) | **killed** |
| slack | `if !allowed` (events API, after signing-secret verify) | **killed** |
| webhook | `if !allowed` (after HMAC verify) | **killed** |
| email | `if !c.allow.Allows(to)` (recipient allowlist, fail-closed send) | **killed** |

Telegram additionally gates the inbound-photo fetch on `allowed &&` so an unauthorized
sender's file reference is never dereferenced (`TestInbound_PhotoNotFetchedForRejectedSender`)
— also killed.

So on every external entry point, a non-allowlisted principal cannot drive the agent, and
the allowlisted one is not wrongly refused. The signature/secret verification on the HTTP
channels (discord Ed25519, slack signing secret, webhook HMAC) is separately fuzzed
(`FuzzVerify`, M533).

## Verification / gate
- No code change; `go test ./plugins/channels/...` passes (`GOMAXPROCS=3`).
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Authorization surface — complete
Every authorization boundary in Agezt is now verified by negative control:
control-plane primary token (M529) + tenant allowlist (M530), REST/OpenAI token auth
(M513), and now all five inbound channel allowlists (M540) — plus the channel signature
verifiers fuzzed (M533). No principal can act beyond its grant on any surface.
