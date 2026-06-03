# M201 — `agt peers models [<name>]` (mesh model discovery)

## Why
The M8 mesh lets one Agezt dispatch a task to another node via the `remote_run`
tool, named by peer (`AGEZT_PEERS="name=url|token,…"`). But to dispatch usefully an
operator needs to know *which* node can serve a given model. The existing `agt peers`
only ran a health check — it reported a model *count* (`7 model(s)`) but not the
actual model ids, so there was no way to answer "which peer has `opus`?" without
curling each node by hand.

Each peer already exposes `GET /api/v1/models` → `{"default": "<id>", "models":
[ids…]}` (the same surface `remote_run` routes against). M201 surfaces it as a first
-class operator command so mesh routing decisions can be made from the CLI.

## What
- **New verb `agt peers models [<name>] [--json]`** in `cmd/agt/peers.go`:
  - With no name, queries every configured peer (sorted); with a name, just that one
    (an unknown name exits 1 with `unknown peer`).
  - Text output: `  <name>  <url>  default=<id>  models: a, b, c` per peer, or
    `UNREACHABLE: <error>`; an empty list renders `(none)`. `--json` emits an array of
    `peerModels{name,url,reachable,default,models,error}` for scripting.
  - Exits non-zero if any queried peer is unreachable (so it composes in scripts /
    health gates), `0` when all reachable.
- **`fetchPeerModels`** mirrors the existing `checkPeer`: 5s context timeout, bearer
  auth from the peer's token, status handling (401 → `token rejected`, non-2xx →
  `status N`), and a **bounded-read** decode.
- **Parser hardening**: `cmdPeers` now distinguishes the verb (`list`/`models`) from a
  bare positional peer name and rejects a name without `models` (`a peer name is only
  valid with 'models'`) and unknown flags. `list` behaviour is unchanged.
- **Shared response cap**: the M200 `maxPeerHealthBytes` const is renamed
  `maxPeerResponseBytes` (still 1 MiB) and now bounds both the health and models reads
  — the models reader is born bounded, never reading an unbounded peer body.

Tokens are never printed in any output path.

## Tests (+6)
`cmd/agt/peers_models_test.go` (with a `modelsServer` httptest helper):
- `TestPeers_Models_JSON` — reachable peer → correct `default` + ordered `models`.
- `TestPeers_Models_Text` — text output carries `default=…` and the model ids.
- `TestPeers_Models_ByName` — a name filters to exactly that peer; a second peer
  pointed at a dead address is never queried (proving the filter, not just the output).
- `TestPeers_Models_UnknownName` — `models ghost` → exit 1, `unknown peer`.
- `TestPeers_NameOnlyValidWithModels` — a bare name without `models` → exit 2.
- `TestPeers_Models_OversizedBody` — an unbounded models body is cut off and the peer
  is reported unreachable with `bad models response`, proving the bound is active.

All drive the real `cmdPeers` against `httptest` peers (network-free). The M200 test's
const reference was updated to the renamed `maxPeerResponseBytes`.

## Live proof (network-free)
- `agt peers --help` shows the new `models` verb usage.
- `AGEZT_PEERS='' agt peers models` → `No peers configured. …`.

## Verification
- `go test ./...` — 1628 passing (1622 + 6 new), 0 failing.
- `go vet ./cmd/agt/` — clean.
- `gofmt -l` (CRLF-normalized) clean on touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only).
- Local commit only (no push); standard trailer.

## Files
- `cmd/agt/peers.go` — `models` verb, `peersModels`, `fetchPeerModels`, `peerModels`,
  parser update, `maxPeerResponseBytes` rename.
- `cmd/agt/peers_models_test.go` — new discovery + bound tests.
- `cmd/agt/peers_oversize_test.go` — const reference updated.

## Scope note
This is read-only discovery over the existing peer REST surface; it adds no new
endpoint and no new dependency. It composes with `remote_run` (which still routes by
peer name) by telling the operator which name to use. A natural follow-on — automatic
model→peer routing for `remote_run` — is deliberately left out to keep this milestone
single-purpose.
