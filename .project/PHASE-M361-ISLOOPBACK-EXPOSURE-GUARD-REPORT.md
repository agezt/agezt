# M361 — Lock in isLoopback (the daemon-exposure warning primitive)

## Why
Priority-A security test coverage. The daemon binds several optional servers from
operator-set addresses — web UI (`AGEZT_WEB_ADDR`), control plane, REST API
(`AGEZT_REST_ADDR`), OpenAI-compatible API. Each binds only when configured, and
the startup banner / `agt doctor` warns "reachable beyond localhost" when the bind
is not loopback, so an operator who exposes the agent to the network does so as a
visible, explicit choice. That warning is gated entirely on `isLoopback(addr)`
(cmd/agezt/main.go).

`isLoopback` had **no test**. A regression that classified `0.0.0.0:port`, an
empty host (`:port`, which also binds every interface), or a LAN/public IP as
loopback would silently suppress the warning — and an operator could expose a
token-authed-but-network-reachable agent daemon without any signal. This is a
security primitive whose correctness must be pinned.

## What
Test-only. **`cmd/agezt/main_test.go`** — `TestIsLoopback_ClassifiesExposureCorrectly`,
a table test over the security-critical cases:
- **loopback (warning must stay quiet):** `127.0.0.1:8800`, `localhost:8800`,
  `[::1]:8800`, bare `127.0.0.1`, `127.0.0.53:8800` (all of 127/8), `::1`.
- **exposed (warning must fire):** `0.0.0.0:8800` (the classic mistake — every
  interface), `:8800` (empty host = every interface), bare `0.0.0.0`,
  `192.168.1.5:8800` (LAN), `10.0.0.1:8800` (private), `203.0.113.7:8800`
  (public), `example.com:8800` (hostname — conservatively non-loopback), and `""`.

## Verification
- `go test ./cmd/agezt -run IsLoopback -v` — passes; every case classified
  correctly.
- `gofmt -l` clean; `go vet ./cmd/agezt/` clean; `GOOS=linux go build ./...` exit
  0. Full suite **2097** passing (was 2096; +1), `go test ./...` exit 0.
  `go.mod`/`go.sum` unchanged. No CHANGELOG (test-only; behaviour unchanged).

## Scope notes
- No production change — `isLoopback` was already correct (empty host → false,
  `0.0.0.0` → `net.IP.IsLoopback()` false, all of 127/8 and `::1`/`localhost`
  → true). This pins that contract so the exposure warning can't be silently
  broken.
- Defaults remain operator-driven: the servers don't force loopback (an operator
  may bind publicly on purpose), but a non-loopback bind is always surfaced as a
  warning — which is exactly the behaviour this test protects.
