# M208 — `agt status` shows the configured peer mesh

## Why
M207 put mesh *health* into `agt doctor` (which probes each peer). But `agt status` —
the operator's quick "what is this node?" overview — said nothing about the mesh at
all. An operator glancing at status to confirm a node's configuration couldn't see
whether (and to which) peers it federates. The right split: `status` shows the mesh
**configuration** cheaply; `doctor`/`peers` probe its **health**.

Crucially, `agt status` must stay fast — it's a single local control-plane call. So the
mesh view here is config-only (parsed from `AGEZT_PEERS`), with **no** network probe: a
down peer never slows `status` down.

## What
`cmd/agt/status.go`:
- **Text**: a new `mesh : …` line rendered via the existing `peer.Describe` (which lists
  `name→url` per peer with tokens redacted), shown only when peers are configured —
  quiet for single-node operators, like the channels/schedules lines.
- **`--json`**: a client-side `mesh` augment (alongside `client_version`) — an array of
  `{name, url}` objects, sorted by name, built by the new `meshSummary()` helper.
  **Tokens are never included**, under any key.
- Both paths parse `AGEZT_PEERS` client-side (the same spec the daemon, `agt peers`, and
  `remote_run` read); a malformed spec is treated as "no mesh" (quiet) rather than an
  error, since `agt doctor`'s mesh check (M207) is the place that flags a bad spec.
- Imports added: `os`, `sort`, and the `plugins/tools/peer` package.

## Tests (+2)
`cmd/agt/status_mesh_test.go` (testing the pure `meshSummary` helper):
- `TestMeshSummary_NoneOrMalformed` — empty and malformed `AGEZT_PEERS` both yield nil
  (status stays quiet).
- `TestMeshSummary_SortedNamesURLsNoToken` — two configured peers come back sorted by
  name with their URLs, **no** `token` key, and no token value leaks into any field.

The text path's token redaction is already covered by the peer package's
`TestDescribe_RedactsToken`. Testing the full `cmdStatus` render is left to the
existing integration surface (it needs a live daemon); the M208 logic is the pure
`meshSummary` + the `peer.Describe` call, both independently covered.

## Verification
- `go test ./...` — 1653 passing (1651 + 2 new), 0 failing.
- `go vet ./cmd/agt/` — clean.
- `gofmt -l` (CRLF-normalized) clean on touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib + existing internal `peer` package).
- Local commit only (no push); standard trailer.

## Files
- `cmd/agt/status.go` — `mesh` text line, `--json` augment, `meshSummary` helper, imports.
- `cmd/agt/status_mesh_test.go` — new `meshSummary` tests.

## Mesh thread (M8) so far
- **M200** bounded peer health read · **M201** `agt peers models` · **M202**
  `remote_run {model}` · **M203** auto-route by model · **M204** `agt peers route`
  inspector · **M205** discovery cache · **M206** auto-route failover · **M207**
  `agt doctor` mesh-health check · **M208** `agt status` mesh config (this milestone).

The mesh is now legible from both operator dashboards: `status` shows the topology
(fast, config-only), `doctor`/`peers` show health (probed). Load/cost-aware routing and
`remote_run` streaming remain deferred to keep each milestone single-purpose.
