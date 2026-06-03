# M227 — `agt doctor` per-tenant peer pre-flight

## Why
M219 added per-tenant mesh peer sets (`AGEZT_TENANT_PEERS`): a tenant's
`remote_run` delegations route against its own peer set, falling back to the
global set. The daemon parses it at startup and **hard-fails** on a malformed
spec — but nothing in the CLI surfaced it. An operator configuring per-tenant
peers had:
- no confirmation the override loaded (vs. a typo'd tenant silently falling back
  to global), and
- no pre-flight for a malformed spec — discovered only when the daemon refuses
  to restart.

This is the same gap M225 closed for `AGEZT_PLUGINS` / `AGEZT_PLUGIN_PINS` /
`AGEZT_PLUGIN_TOOLS`, applied to the last hard-failing, CLI-invisible plugin/mesh
spec.

## What
A new `checkTenantPeers(spec)` doctor check (pure — takes the spec string, so
it's unit-testable without env mutation; the registration site reads
`AGEZT_TENANT_PEERS`). Surfaced only when the env var is set (advanced feature →
quiet otherwise). It:

- **FAILs** on a malformed spec, naming the parse error and that the daemon will
  refuse to start (matching the daemon's hard-fail);
- **WARNs** when a tenant is present in the spec but has an empty peer set. The
  parser (`peer.ParseTenantPeers`) silently drops such tenants — so the
  daemon ignores them and the tenant falls back to the global set with no other
  signal. doctor re-reads the raw JSON keys, diffs them against the parsed
  result, and names the dropped tenants — a silent misconfiguration nothing else
  reported;
- **OKs** a clean spec with a per-tenant peer-count summary
  (`alpha→2 peer(s), beta→1 peer(s)`). Peer URLs and tokens are never printed —
  only tenant names and counts.

## Files
- `cmd/agt/doctor.go` — `checkTenantPeers` + conditional registration (edited).
- `cmd/agt/doctor_tenantpeers_test.go` — 6 tests (new): unset is quiet, `{}` OK,
  valid multi-tenant OK (asserts counts and that no URL/token leaks), malformed
  set FAILs (bad JSON, bad inner peer spec), a silently-dropped tenant WARNs
  (alongside a loaded one), and an all-dropped spec WARNs (not a silent OK).

## Verification
- `go test ./cmd/agt/` — green; full suite **1742 → 1748** (+6), 66 packages.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./cmd/agt/` clean.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.
- **Live proof (end-to-end against a running daemon):**
  - valid → `[OK] tenant-peers : 2 tenant override(s): alpha→2 peer(s), beta→1 peer(s)`
  - dropped → `[WARN] tenant-peers : loaded 1 override(s) [alpha→1 peer(s)]; ignored (empty peer set): ghost`
  - malformed → `[FAIL] tenant-peers : AGEZT_TENANT_PEERS is malformed: … invalid JSON …`
    with the "daemon will refuse to start" hint.

## Scope notes
- doctor validates spec *shape* and reports counts; it does not probe the peer
  URLs for reachability (that's `checkMesh`'s job for the global set, and a
  per-tenant reachability sweep would multiply network probes). A future
  refinement could extend the mesh reachability probe to tenant peer sets.
- `agt status` was left unchanged; doctor is the validation surface (consistent
  with M225/M226).
