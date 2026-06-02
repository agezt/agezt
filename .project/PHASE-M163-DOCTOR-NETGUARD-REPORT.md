# M163 — Egress-guard health check in `agt doctor`

## Why
Continuing to fold the security subsystems into the single-pane diagnostic (M162
added schedules), the egress guard (netguard, M16/M109) had no `agt doctor` check.

A `netguard.blocked` event means a tool (http / browser.read) tried to open a
connection to an internal / metadata address — `169.254.169.254` (cloud
credentials), `10.x`, `127.x`, link-local — and the guard refused it. That is one
of the highest-signal security events the system produces: it's the fingerprint of
an SSRF attempt, a prompt-injection exfiltration, or (more mundanely) a legitimate
host the operator forgot to allowlist. Until now it was only visible via `agt
netguard log` — invisible unless someone thought to look.

## What
`cmd/agt/doctor.go`:
- `checkNetguard(ctx, client)` — calls `CmdNetguardLog` (M109) scoped to the last
  24h (`since_ms`) with a generous `limit`; a call failure is an informational OK
  (never a false FAIL), mirroring `checkWebhooks`/`checkSchedules`.
- `netguardCheckFromLog(res)` — the pure, testable verdict:
  - no blocks → OK "no egress blocked in the last 24h".
  - any blocks → **WARN** "N egress connection(s) blocked in the last 24h", with the
    hint naming the most recent target (`tool→ip`, e.g. `http→169.254.169.254`;
    blocks arrive newest-first). Wording makes clear the guard *prevented* them and
    points to `agt netguard log`.
- `netguardLookbackMS` const (24h) documents the window.
- Wired into `runDoctorChecks` after `checkExposure` (both are network-security).

### Design choices
- **WARN, not FAIL.** A block is the guard working correctly — the run was
  protected. It's surfaced because it warrants operator review (allowlist a host /
  investigate an attack), not because anything is broken.
- **24h window, self-clearing.** Unlike webhooks (which has a failure *rate* over a
  total), netguard has no natural denominator, so an all-time count would WARN
  forever after a single reviewed block. Scoping to the last 24h makes the check
  actionable and self-clearing once egress is clean for a day. `sinceCutoff`
  interprets `since_ms` as a relative age (`now - since_ms`), so the window tracks
  wall-clock automatically.

Reuses the existing `CmdNetguardLog` response (`{blocks:[{ts_unix_ms,ip,reason,
tool}], count}`) — no control-plane change.

## Tests (+1, all passing)
`TestNetguardCheckFromLog`: no-blocks OK; missing-key OK (best-effort); blocks
present → WARN with the count in the detail and the most-recent `tool→ip` in the
hint; an ip-only block (no tool) still names the ip.

## Live proof
- **OK path (binary):** a clean mock daemon → `agt doctor` shows
  `[OK  ] netguard : no egress blocked in the last 24h` — proving the wiring, the
  real `CmdNetguardLog` call, and the 24h-window arg end to end.
- **WARN path:** the offline mock's fixed shell+final script never drives the http
  tool to an internal address, so a *real* block can't be triggered through the
  offline daemon. The WARN verdict is instead proven by (a) the unit test above
  against the real response shape, and (b) the existing `TestNetguardLog`
  (kernel/controlplane), which publishes `netguard.blocked` events exactly as the
  guard's OnBlock does and confirms `CmdNetguardLog` returns them newest-first in
  precisely the `{blocks, count}` shape `netguardCheckFromLog` consumes. (Same
  approach as M154, where a hard-to-trigger live state was covered by a rigorous
  unit test plus a lighter live confirmation.)

## Verification
- `go.mod` / `go.sum` unchanged; no new protocol command or env var.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on both touched files.
- `go test ./... -count=1` — **FAIL 0**, **1525 tests** (was 1524; +1), 61 packages.

## Result
A tool reaching for an internal/metadata address now surfaces in `agt doctor`
instead of sitting silently in the journal — the highest-signal SSRF/exfiltration
event joins the single-pane health view, with a hint pointing straight at what was
blocked.
