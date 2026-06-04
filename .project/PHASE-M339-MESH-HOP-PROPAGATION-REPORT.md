# M339 — Mesh hop propagation into the run context (lock-in)

## Why
Priority-A coverage on the federation loop guard (M8 mesh). The REST handler for
`POST /api/v1/runs` already refuses a delegated run whose hop exceeds the limit
(508 Loop Detected) — that *refusal* path is well-tested (over-limit, at-limit,
env-override, primary-bus + tenant-bus audit). But the guard has a second,
equally load-bearing half: for a *within-limit* delegated run, the handler threads
the incoming hop into the run context (`r.WithContext(meshctx.WithHop(ctx, hopIn))`,
restapi.go:351) so this node's OWN `remote_run` forwards `hop+1` in turn. If that
propagation were broken, the hop would silently reset to 0 at every node and the
loop guard would never fire across a multi-node chain — the exact runaway it
exists to stop. That propagation had no test.

## What
Test-only. In **`kernel/restapi/`**:
- `restapi_test.go`: the shared `fakeEngine` now records `ranHop` —
  `meshctx.Hop(ctx)` observed in its `RunModel` context (and imports `meshctx`).
- `mesh_hop_test.go`: two new tests —
  - `TestMeshHop_WithinLimitPropagatesIntoRunContext`: a within-limit incoming hop
    (3) runs (200) and the engine observes hop==3 in its context, proving the
    forward chain stays intact (so a downstream `remote_run` sends hop+1).
  - `TestMeshHop_NoHeaderStartsChainAtZero`: a local (non-delegated) run carries
    hop 0, so a `remote_run` it spawns starts the chain at 1.

## Verification
- `go test ./kernel/restapi -run MeshHop -v` — all 8 mesh-hop tests pass
  (6 pre-existing + 2 new).
- `gofmt -l` clean; `go vet ./kernel/restapi/` clean; `GOOS=linux go build ./...`
  exit 0. Full suite **2061** passing (was 2059; +2), `go test ./...` exit 0.
  `go.mod` / `go.sum` unchanged.

## Scope notes
- No production change — the propagation already worked; this pins it so a refactor
  of the handler can't silently drop the hop. The refusal half (508 + audit) was
  already covered; the mesh loop guard is now tested on both halves (refuse-over-
  limit AND propagate-within-limit).
- The `remote_run` tool's send-side (reads `meshctx.Hop` from ctx, POSTs `hop+1`)
  is the symmetric counterpart; it lives in the tool layer and is exercised where
  that tool is tested. This milestone covers the receiving node's contract.
