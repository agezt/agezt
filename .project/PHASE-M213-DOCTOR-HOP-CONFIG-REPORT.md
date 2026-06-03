# M213 — `agt doctor` flags a misconfigured mesh hop limit

## Why
M211 made the mesh delegation hop limit tunable via `AGEZT_MESH_MAX_HOPS`, validating the
value and silently falling back to the default 8 when it is invalid (non-integer, `<1`, or
past the cap). That silent fallback is the right *runtime* behaviour — a typo must never
weaken the guard or crash the daemon — but it leaves an operator who fat-fingered the value
(`AGEZT_MESH_MAX_HOPS=l0` instead of `10`) believing they raised the limit when they did
not. A safety-relevant setting that's quietly ignored is exactly the kind of thing the
go-to diagnostic should catch.

## What
- **`kernel/meshctx`** — refactored the env read into `MaxHopsConfig() (effective int, raw
  string, validOverride bool)`, the single source of validation: `raw==""` → default,
  `validOverride=true`; a set-but-invalid value → default with `validOverride=false`.
  `MaxHopsFromEnv()` now delegates to it (behaviour unchanged). Exported
  `MaxConfigurableHops` (the cap) for messages.
- **`cmd/agt/doctor.go`** — new `checkMeshHopLimit()` check, appended to `runDoctorChecks`
  **only when `AGEZT_MESH_MAX_HOPS` is set** (so single-node / default operators see no extra
  line). A valid override → OK (`delegation hop limit = N (AGEZT_MESH_MAX_HOPS)`); an invalid
  one → WARN (`AGEZT_MESH_MAX_HOPS="…" is invalid and ignored; using default 8`) with a hint
  to set an integer in `[1, 64]`. WARN (not FAIL) because the daemon is still safe on the
  default — it only fails `--strict`.

## Tests (+2)
`cmd/agt/doctor_mesh_test.go`:
- `TestCheckMeshHopLimit_ValidOverride` — `AGEZT_MESH_MAX_HOPS=4` → OK, detail reports `= 4`.
- `TestCheckMeshHopLimit_InvalidWarns` — each of `abc`, `0`, `-3`, `9999` → WARN, detail says
  "invalid" and carries a hint.

The M211 `MaxHopsFromEnv` table and the M207/M208 mesh checks remain and pass (the refactor
is behaviour-preserving).

## Verification
- `go test ./...` — 1677 passing (1675 + 2 new), 0 failing.
- `go vet ./kernel/meshctx/ ./cmd/agt/` — clean.
- `gofmt -l` (CRLF-normalized) clean on touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only).
- Local commit only (no push); standard trailer.

## Files
- `kernel/meshctx/meshctx.go` — `MaxHopsConfig`, `MaxConfigurableHops`; `MaxHopsFromEnv`
  delegates.
- `cmd/agt/doctor.go` — `checkMeshHopLimit` + conditional wiring + import.
- `cmd/agt/doctor_mesh_test.go` — new hop-limit-config tests.

## Mesh thread (M8) so far
- **M200** bounded peer health read · **M201** `agt peers models` · **M202**
  `remote_run {model}` · **M203** auto-route · **M204** `agt peers route` · **M205**
  discovery cache · **M206** failover · **M207** `agt doctor` mesh check · **M208**
  `agt status` mesh config · **M209** loop guard · **M210** loop-refusal audit · **M211**
  tunable hop limit · **M212** tenant-scoped loop audit · **M213** doctor flags a bad hop
  config (this milestone).

The hop limit is now enforced (M209), audited (M210, per-tenant M212), tunable (M211), and
its misconfiguration is now visible (M213). Remaining deferred follow-ons (refused-loop count
in status/doctor, load/cost-aware routing, per-tenant peer sets) keep their own milestones.
