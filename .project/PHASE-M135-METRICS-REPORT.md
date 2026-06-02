# M135 — Prometheus `/metrics` endpoint (SPEC-14 §9)

## Why
The natural deployment companion to the M134 health probes, and the dependency-
free first step of SPEC-14 §9's observability story (full OTel is later-phase). A
daemon on a VPS is monitored with Prometheus + Grafana; without a `/metrics`
endpoint there's no standard way to graph or alert on its operational state
(spend creeping up, disk filling, runs piling up, approvals stuck, daemon halted).

## What
`GET /metrics` on the REST API, **token-authed** and rendered in Prometheus text
exposition format (`# HELP` / `# TYPE` / `agezt_<name> <value>`). Unlike the
unauthenticated `/healthz` + `/readyz` (M134), `/metrics` exposes spend and
activity volume — financially/operationally sensitive — so it requires the token;
Prometheus scrapes it with a `bearer_token` in the scrape config.

Gauges (all cheap in-memory or O(segments) reads — **no per-scrape journal fold**):
`agezt_up`, `agezt_halted`, `agezt_uptime_seconds`, `agezt_active_runs`,
`agezt_journal_head_seq`, `agezt_journal_bytes`, `agezt_memory_records`,
`agezt_world_entities`, `agezt_active_skills`, `agezt_schedules_total`,
`agezt_schedules_enabled`, `agezt_pending_approvals`,
`agezt_spend_today_microcents`, `agezt_budget_ceiling_microcents`,
`agezt_disk_free_bytes`, `agezt_disk_free_ratio`.

Decoupling: `kernel/restapi` defines a `Metric` struct + `SetMetrics(fn)` and only
formats; the daemon (which has the kernel + governor + disk probe) supplies the
gather closure — the same injection pattern as `SetReadiness` / `SetDiskFree` /
`SetTenantResolver`. Values render with `strconv.FormatFloat(_, 'f', -1, 64)` so
large gauges (spend) read as plain integers, not scientific notation.

## Files
- `kernel/restapi/restapi.go` — `Metric` type, `metrics` field, `SetMetrics`,
  `/metrics` route (authed), `handleMetrics` formatter; `strconv` import.
- `cmd/agezt/main.go` — `rest.SetMetrics(...)` gather closure over the kernel +
  governor snapshot + `pulse.DiskUsage` + a journal-dir size walk.
- `kernel/restapi/metrics_test.go` (new) — authed Prometheus format (prefix,
  HELP/TYPE, plain integers), 401 without a token, empty-but-200 with no source.

## Live proof (offline mock, AGEZT_REST_ADDR set)
```
GET /metrics (no token)  → 401
GET /metrics (Bearer …)  →
  agezt_up 1
  agezt_halted 0
  agezt_uptime_seconds 2.956168
  agezt_active_runs 0
  agezt_journal_head_seq 14
  agezt_schedules_total 1            ← reflects an added schedule
  agezt_schedules_enabled 1
  agezt_pending_approvals 0
  agezt_spend_today_microcents 0
  agezt_budget_ceiling_microcents 20000000000   ← $20 default ceiling
  agezt_journal_bytes 9126
  agezt_disk_free_bytes 1207425216512
  agezt_disk_free_ratio 0.6039145000620393
```

## Verification
- 55 packages `ok`, **FAIL 0**; **1431 tests**.
- `go vet` clean, `GOOS=linux go build ./...` ok, `go.mod` / `go.sum` unchanged.
- gofmt: all touched/new files clean under LF.
