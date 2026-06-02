# M134 — Unauthenticated `/healthz` + `/readyz` probes (SPEC-14 §9)

## Why
Specced (SPEC-14 §9 "Health/readiness endpoints"), and a real deployment gap. The
REST API already had `GET /api/v1/health` — but it is **token-authed**, which is
wrong for deployment probes: systemd watchdogs, container/k8s liveness+readiness
probes, load balancers, and uptime monitors expect an **unauthenticated** health
endpoint and can't easily carry a Bearer token. Without one, the ROADMAP's "$5
VPS / container" deployment story has no standard way to health-check the daemon
over HTTP.

The liveness/readiness distinction matters operationally:
- **Liveness** — is the process up and serving? If not, restart it.
- **Readiness** — can it serve work *right now*? If not (e.g. halted via `agt
  halt`), pull it from the load-balancer rotation, but **don't restart it** — it's
  alive and deliberately paused.

## What
Two unauthenticated routes on the REST server, at the root (not under the
versioned, authed `/api/v1`):
- **`GET /healthz`** — always 200 `{"status":"ok"}` (HEAD supported). The server
  answering at all proves the process is alive. No version, model, or run data.
- **`GET /readyz`** — 200 `{"status":"ready"}` when serving; **503**
  `{"status":"not_ready","reason":"halted"}` when the injected readiness probe
  says so. The daemon wires the probe to `k.IsHalted()`.

Security: these expose only liveness/readiness — never version/model/run data
(that stays behind the authed `/api/v1/health`, unchanged). The REST server is
loopback-bound by operator choice; the probes add no sensitive surface. Readiness
is injected (`SetReadiness`) so `kernel/restapi` needs no halt-state coupling —
the same pattern as `SetTenantResolver`.

## Files
- `kernel/restapi/restapi.go` — `readiness` field + `SetReadiness`; `/healthz` +
  `/readyz` registered UNauthed in `Handler()`; `handleLive`, `handleReady`.
- `cmd/agezt/main.go` — `rest.SetReadiness(...)` wired to `k.IsHalted()`.
- `kernel/restapi/health_test.go` (new) — `/healthz` unauth 200 + no version/model
  leak + HEAD; `/readyz` 200→503 on halt with reason; `/api/v1/health` still 401
  without a token (regression guard).

## Live proof (offline mock, AGEZT_REST_ADDR set)
```
GET /healthz       (no token)            → 200 {"status":"ok"}
GET /readyz        (no token, running)   → 200 {"status":"ready"}
GET /api/v1/health (no token)            → 401 (authed endpoint unchanged)

# after `agt halt`:
GET /readyz        (no token, halted)    → 503 {"reason":"halted","status":"not_ready"}
GET /healthz       (no token, halted)    → 200 {"status":"ok"}   ← alive, just not ready
```

## Verification
- 55 packages `ok`, **FAIL 0**; **1429 tests**.
- `go vet` clean, `GOOS=linux go build ./...` ok, `go.mod` / `go.sum` unchanged.
- gofmt: all touched/new files clean under LF.
