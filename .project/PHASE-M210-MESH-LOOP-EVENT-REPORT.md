# M210 — Audit a refused mesh delegation loop

## Why
M209 added the mesh delegation loop guard: a cross-node delegation past `MaxHops` is
refused with `508 Loop Detected`. But the refusal was **silent** to everyone except the
rejected caller — nothing was recorded. Agezt's guiding principle is "everything is an
event": a safety-relevant action like stopping a federation loop should be in the
hash-chained journal so an operator can see (and `agt why`) that it happened, who tried,
and how deep the chain got. Without it, a misconfigured mesh that's quietly looping-then-
refusing leaves no trace for diagnosis.

## What
`kernel/event/kinds.go`:
- New event kind **`KindMeshLoopRefused = "mesh.loop_refused"`** (payload `{hop, max_hops}`).

`kernel/restapi/restapi.go` (`handleRunsRoot`):
- When the inbound hop exceeds `meshctx.MaxHops`, before returning `508`, the server now
  publishes a `mesh.loop_refused` event on its bus:
  ```go
  s.bus.Publish(event.Spec{
      Subject: "mesh.loop",
      Kind:    event.KindMeshLoopRefused,
      Actor:   "restapi",
      Payload: map[string]any{"hop": hopIn, "max_hops": meshctx.MaxHops},
  })
  ```
- Best-effort: a nil bus is skipped, and a publish error is ignored — auditing must never
  block the refusal itself.

So a stopped loop now surfaces via `agt pulse --kind mesh.loop_refused` and the journal,
the same way budget/netguard/capability refusals already do.

## Tests (+1)
`kernel/restapi/mesh_hop_test.go`:
- `TestMeshHop_RefusalIsAudited` — subscribes to `mesh.>` on the server's bus, POSTs a run
  with a hop past the limit, asserts `508` **and** that a `mesh.loop_refused` event is
  received. The existing M209 hop tests (over-limit refused, at-limit runs, no-header runs)
  remain.

## Verification
- `go test ./...` — 1663 passing (1662 + 1 new), 0 failing.
- `go vet ./kernel/event/ ./kernel/restapi/` — clean.
- `gofmt -l` (CRLF-normalized) clean on touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only).
- Local commit only (no push); standard trailer.

## Files
- `kernel/event/kinds.go` — `KindMeshLoopRefused`.
- `kernel/restapi/restapi.go` — publish on hop-limit refusal.
- `kernel/restapi/mesh_hop_test.go` — audit-event test.

## Mesh thread (M8) so far
- **M200** bounded peer health read · **M201** `agt peers models` · **M202**
  `remote_run {model}` · **M203** auto-route by model · **M204** `agt peers route` ·
  **M205** discovery cache · **M206** auto-route failover · **M207** `agt doctor` mesh
  check · **M208** `agt status` mesh config · **M209** loop guard · **M210** loop-refusal
  audit event (this milestone).

The loop guard is now both enforced (M209) and observable (M210). Making `MaxHops`
operator-tunable, and surfacing a count of refused loops in `agt status`/`doctor`, remain
natural follow-ons left deferred to keep this milestone single-purpose.
