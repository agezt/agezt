# M207 — `agt doctor` mesh-health check

## Why
The mesh thread (M200–M206) gave the federation real capability — discovery, model
pinning, auto-routing, an inspector, a discovery cache, and failover. But mesh health
was only visible if you ran `agt peers` on purpose, or implicitly when a `remote_run`
failed. An operator running a multi-node Agezt wants a broken peer to show up in their
**standard pre-flight** — `agt doctor` — alongside daemon, journal, provider, budget,
schedules, channels, and the rest. A federated system isn't healthy just because the
local node is; `doctor` should say so.

## What
`cmd/agt/doctor.go`:
- **New `checkMesh()` check**, wired into `runDoctorChecks` between `checkChannels` and
  `checkHalt`. It:
  - parses `AGEZT_PEERS` — a malformed spec is a **WARN** (not a crash) with a fix hint;
  - reports **OK** "no peers configured (single-node)" when none are set;
  - otherwise probes each peer's `/api/v1/health` via the existing `checkPeer` (M200,
    same package), in name order;
  - reports **OK** "N peer(s) reachable: …" when all are up, or **WARN**
    "M/N peer(s) unreachable: …" naming the down peers with a remediation hint
    (`check the peer URLs/tokens … ` + `agt peers` for detail).
- A down peer is a **WARN**, not a FAIL: the local node is fully functional and the
  mesh is merely degraded — matching how `doctor` treats other "degraded, not broken"
  conditions, and so it only fails `--strict`.
- The check is **independent of the local daemon** (each peer is reached over its own
  REST surface), and **tokens are never printed**.
- Imports added: `sort` and the `plugins/tools/peer` package (for `ParsePeers`); the
  bounded-read health probe is reused from M200.

## Tests (+4)
`cmd/agt/doctor_mesh_test.go`:
- `TestCheckMesh_NoPeers` — empty `AGEZT_PEERS` → OK "single-node".
- `TestCheckMesh_AllReachable` — two healthy `httptest` peers → OK naming them.
- `TestCheckMesh_SomeUnreachable` — one healthy + one 500ing peer → WARN
  "1/2 peer(s) unreachable" naming the down peer, with a non-empty hint.
- `TestCheckMesh_MalformedSpec` — a bad `AGEZT_PEERS` → WARN "malformed", not a panic.

All prior doctor tests pass unchanged; the new check slots into the existing
`doctorCheck` machinery (status tri-state, JSON shape, exit-code rollup).

## Verification
- `go test ./...` — 1651 passing (1647 + 4 new), 0 failing.
- `go vet ./cmd/agt/` — clean.
- `gofmt -l` (CRLF-normalized) clean on touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib + existing internal `peer` package).
- Local commit only (no push); standard trailer.

## Files
- `cmd/agt/doctor.go` — `checkMesh()` + wiring + two imports.
- `cmd/agt/doctor_mesh_test.go` — new mesh-check tests.

## Mesh thread (M8) so far
- **M200** bounded peer health read · **M201** `agt peers models` · **M202**
  `remote_run {model}` · **M203** auto-route by model · **M204** `agt peers route`
  inspector · **M205** discovery cache · **M206** auto-route failover ·
  **M207** `agt doctor` mesh check (this milestone).

Discovery, dispatch, routing, failover, and now mesh health is part of the operator's
standard diagnostic. A natural follow-on — surfacing mesh health in `agt status` too,
and load/cost-aware routing — remains deferred to keep this milestone single-purpose.
