# M513 — Mutation testing restapi: pin the mesh federation hop-limit guard

## Context
Twenty-fourth package in the mutation pass: `kernel/restapi` (the inbound native REST
surface — token auth, tenant routing, run submission, SSE streaming, mesh delegation).
Run with `GOMAXPROCS=3` (CPU-capped). go-mutesting score 0.614, 129 survivors; working
tree restored clean after the run.

## Triage — auth core verified solid first
The highest-stakes path is `authorized`. Hand-applied negative control confirmed it is
already pinned: `presented == "" → !=` killed, `ConstantTimeCompare(...) == 1 → != 1`
killed, the per-tenant `id != "" → id ==` killed (by `TestTenantAuth` /
`TestAuthRequired` / `TestEmptyTokenFailsClosed`). The `s.token != ""` admin-token guard
is a redundant defense (an empty server token can never match a non-empty presented token
via constant-time compare, and `presented == ""` already returns early), so mutating it
away is an equivalent mutant.

## The genuine gap (closed)
`handleRunsRoot` carries the mesh delegation **loop guard** (M209): a run forwarded from
a peer carries `X-Agezt-Mesh-Hop`, and

```
if hopIn > maxHops { … publish mesh.loop … 508 Loop Detected … return }
```

refuses one hop past this node's limit so a federated mesh can't recurse forever. This
guard had **no REST-layer test at all** — no test sent a hop header, exercised the 508
refusal, or checked the inclusive boundary. So two non-equivalent mutants survived:
- `hopIn > maxHops → >= maxHops`: a run arriving at *exactly* the limit is wrongly
  refused (the federation is cut one hop short).
- `hopIn > maxHops → < maxHops`: the refusal effectively never fires — the DoS/recursion
  guard is disabled and a delegation loop can run unbounded.

## Fix
Added `TestSubmitRun_MeshHopLimit`: with `AGEZT_MESH_MAX_HOPS=2` (deterministic via
`t.Setenv`), a hop of 3 is refused with `508 Loop Detected`, while a hop of exactly 2 is
accepted (200) and threads into the run context (`meshctx.Hop(ctx) == 2`).

## Negative control (manual, CPU-capped)
- `hopIn > maxHops → >= maxHops`: FAIL (hop 2 refused).
- `hopIn > maxHops → < maxHops`: FAIL (hop 3 accepted).
Restored byte-for-byte (`git diff --ignore-all-space` on restapi.go empty); passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — twenty-four packages (M490–M513)
redact, journal, edict, netguard, event, creds, warden, governor, scheduler, bus,
cadence, runtime, tenant, worldmodel, approval, memory, skill, standing, catalog, plugin,
webhook, channel, anomaly, restapi — plus the controlplane primary-token auth gate
verified solid. The token-auth core was already solid; the gap was an untested
federation-loop boundary in the inbound run path.
