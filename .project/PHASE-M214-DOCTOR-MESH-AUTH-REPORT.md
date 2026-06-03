# M214 — `agt doctor` flags token-less mesh peers

## Why
A mesh peer is configured as `name=url|token` in `AGEZT_PEERS`, with the token optional.
A peer written as just `name=url` (no `|token`) means `remote_run` POSTs delegated tasks
to that node with **no `Authorization` header** — an unauthenticated cross-node
delegation. That's at odds with Agezt's standing "loopback + token only" posture, and
it's an easy mistake to make (forget the `|token`, or copy a URL-only example). Nothing
surfaced it: the daemon happily delegates, and the failure (if the peer requires auth) is
a confusing 401 at run time — or, worse, silent success against an open node.

The operator's go-to diagnostic should catch this the way it catches the other mesh
posture issues (reachability M207, hop-limit config M213).

## What
`cmd/agt/doctor.go`:
- **New `checkMeshAuth(peers)` check**, appended to `runDoctorChecks` only when
  `AGEZT_PEERS` is configured (no noise for single-node operators).
- All peers carrying a token → **OK** (`all N peer(s) authenticate with a token`).
- Any token-less peer → **WARN** (`M/N peer(s) have no token — unauthenticated delegation:
  <names>`) with a hint to add `|token`. A tokened peer is never listed as token-less.
- **WARN, not FAIL**: a peer on a trusted private network may legitimately need no token,
  so this is a posture nudge that only fails `--strict`, not a hard stop.
- Tokens are never printed (only peer names and the count).

## Tests (+2)
`cmd/agt/doctor_mesh_test.go`:
- `TestCheckMeshAuth_AllTokened` — two tokened peers → OK with `all 2 peer(s) authenticate`.
- `TestCheckMeshAuth_TokenlessWarns` — one tokened (`secure`) + one token-less (`open`) →
  WARN naming `open` and `1/2`, NOT listing `secure`, with a non-empty hint.

The M207/M208/M213 mesh checks remain and pass; `checkMeshAuth` is only wired in when
peers are configured (the doctor integration tests don't set `AGEZT_PEERS`, so they're
unaffected).

## Verification
- `go test ./...` — 1679 passing (1677 + 2 new), 0 failing.
- `go vet ./cmd/agt/` — clean.
- `gofmt -l` (CRLF-normalized) clean on touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib + existing internal `peer` package).
- Local commit only (no push); standard trailer.

## Files
- `cmd/agt/doctor.go` — `checkMeshAuth` + conditional wiring.
- `cmd/agt/doctor_mesh_test.go` — token-posture tests.

## Mesh thread (M8) so far
- **M200** bounded peer health read · **M201** `agt peers models` · **M202**
  `remote_run {model}` · **M203** auto-route · **M204** `agt peers route` · **M205**
  discovery cache · **M206** failover · **M207** `agt doctor` mesh check · **M208**
  `agt status` mesh config · **M209** loop guard · **M210** loop-refusal audit · **M211**
  tunable hop limit · **M212** tenant-scoped loop audit · **M213** doctor flags bad hop
  config · **M214** doctor flags token-less peers (this milestone).

`agt doctor` now covers the mesh on three axes: reachability (M207), safety-config
validity (M213), and auth posture (M214). Remaining deferred follow-ons — refused-loop
count in status/doctor, load/cost-aware routing, per-tenant peer sets — keep their own
milestones.
