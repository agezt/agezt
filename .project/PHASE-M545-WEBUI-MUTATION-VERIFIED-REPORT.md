# M545 — Mutation testing webui: security surface verified solid

## Context
`kernel/webui` is the only kernel package that had not been mutation-assessed. It
serves the operator dashboard and a token-authed JSON API/SSE stream over loopback
— a real auth surface (token gate, constant-time compare, per-route arg allowlist,
CSP/clickjacking headers). This milestone runs `go-mutesting` over it.
`GOMAXPROCS=3`.

## Result
`go-mutesting --test-recursive ./...`: score 0.578 (52 killed, 38 survived, 90
total). Every survivor was classified by extracting its diff and matching against
the security-relevant surface (token / authorized / URL.Path / HasPrefix /
TrimPrefix / args allowlist / Bearer / ConstantTimeCompare / nonce):
**zero security-relevant survivors.** The whole auth + arg-allowlist + path surface
is killed by the existing tests.

## The 38 survivors — all equivalent or cosmetic error-path
- **Tuning constants (equivalent):** the 5s per-route read timeout
  (`WithTimeout(...,5*time.Second)` → 4s/6s and `5*` → `5/`), the SSE buffer
  (`Subscribe(">",256)` → 255/257), the heartbeat ticker (20s → 19/21s), and the
  CSP-nonce length (`var b [16]byte` → 15/17). None of these values is asserted —
  they are tuning, not behavior; mutating them changes nothing observable.
- **Header `Set` removals that DetectContentType makes equivalent:** dropping the
  dashboard's `Content-Type: text/html` survives because `httptest`'s recorder
  falls back to `http.DetectContentType` on the `<!DOCTYPE html>` body and still
  reports `text/html` — the explicit Set is belt-and-suspenders. `Cache-Control`,
  `Connection`, `Allow`, and the JSON `Content-Type` likewise aren't asserted (the
  dashboard's `fetch().json()` ignores the response content-type).
- **Error-path / streaming teardown (cosmetic):** discarding the `BadGateway`
  error-body write in the proxies, and removing `return`/`continue`/`break` in the
  SSE relay loop (handleEvents). These are failure-path responses and stream
  shutdown, not security or primary behavior.

## Verification
- No code or test change (verified-solid verdict). `go-mutesting` mutates in a temp
  copy; working tree confirmed clean (`git status` empty) and `report.json` removed
  after the run.

## Kernel mutation coverage — complete
Every kernel package with non-test code has now been mutation-assessed (37 in the
detail table + webui here). Combined with the full `plugins/tools` sweep
(M535–M544), `plugins/external/mcpbridge` (M538), `plugins/channels` auth gates
(M540), `plugins/tools/peer` (M541), and the two primary providers (M539), the
highest-stakes mutation surface of the codebase is covered. Genuine gaps were
closed where found (incl. one real prod bug, M517); the remainder is verified
solid with only equivalent/cosmetic survivors.
