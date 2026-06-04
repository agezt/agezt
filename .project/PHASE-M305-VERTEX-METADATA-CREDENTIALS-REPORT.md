# M305 — Vertex ambient credentials: the GCE/GKE metadata token source

## Why
A fresh product-capability axis (the audit / cost / webui veins are exhausted of
high-value work). The Vertex provider could authenticate **only** via a
service-account JSON key file (`GOOGLE_APPLICATION_CREDENTIALS`). That's fine for
desktop/CI, but in production on Google Cloud — Compute Engine, GKE with Workload
Identity, Cloud Run — the standard, Google-recommended path is the **instance
metadata server**: the platform injects a short-lived, rotating, scoped OAuth
token for the workload's service account, so no static key file ever lands on
disk (a file is the thing that gets leaked). Without this, running Agezt's Vertex
provider on GKE meant shipping a long-lived SA key — an anti-pattern. The code
even had a standing note that the metadata server was "not implemented".

## What
- **`plugins/providers/vertex/metadata.go`** (new): `MetadataTokenSource` mints
  and caches tokens from `…/computeMetadata/v1/instance/service-accounts/default/token`
  with the **mandatory `Metadata-Flavor: Google`** header (the server rejects
  requests without it — a built-in SSRF/DNS-rebinding defence). Same caching +
  `TokenSkew` refresh discipline as the service-account `TokenSource`, bounded
  response reads, clean errors on non-2xx / missing `access_token`. A `ProjectID`
  method reads `…/project/project-id` (plain-text body) so the project can be
  discovered ambiently too. The default HTTP client carries a short timeout so a
  non-GCP host fails fast instead of hanging on the link-local address.
- **`plugins/providers/vertex/auth.go`**: introduced a small `TokenMinter`
  interface (`Token(ctx) (string, error)`) — both `*TokenSource` and
  `*MetadataTokenSource` satisfy it. Updated the stale "not implemented" note.
- **`plugins/providers/vertex/vertex.go`**: `Provider.TokenSource` and `New`
  now take `TokenMinter` instead of the concrete `*TokenSource`. Source-compatible
  for every existing caller (a `*TokenSource` is a `TokenMinter`; `New(nil, …)`
  still yields an interface-nil that the existing `== nil` guard catches).
- **`plugins/providers/compat/compat.go`**: the `FamilyGoogleVertex` build branch
  now selects the auth path. `resolveVertexCreds` returns a `vertexCreds` struct;
  with `GOOGLE_VERTEX_USE_METADATA=1` (or a `GOOGLE_VERTEX_METADATA_URL` override,
  which implies it) and no key file, Build wires the metadata source and
  auto-fetches the project id from the same server when `GOOGLE_VERTEX_PROJECT`
  is unset — the fully ambient experience. The missing-credentials error now
  offers the metadata alternative. Added an `isTruthy` env helper.

## New env vars (resolved through the existing CredLookup — no inventory change)
- `GOOGLE_VERTEX_USE_METADATA` — `1`/`true`/`on`/`yes` → use the metadata server.
- `GOOGLE_VERTEX_METADATA_URL` — override the metadata base (proxy/sidecar/tests);
  implies metadata auth.

## Verification
- New tests (all green): `metadata_test.go` — token fetch + `Metadata-Flavor`
  header + caching (one server hit for two `Token()` calls), `ProjectID`
  plain-text trim, non-2xx error, missing-`access_token` error, and an
  **end-to-end** `Complete` proving a metadata-minted token reaches the request
  `Authorization: Bearer …` header. `compat_test.go` — `TestBuild_VertexMetadataAuth`
  drives the **daemon's real provider-build path** (`compat.Build` → `Complete`)
  with metadata opt-in and no project set: the token flows onto the wire and the
  auto-fetched project id (`ambient-proj`) appears in the generateContent URL.
  Updated `TestBuild_VertexMissingCredsRefused` (the old error named the now-shipped
  deferral). This network-free httptest proof is the live proof for a
  provider-wire feature (same discipline as M299–M302).
- Full suite **1957** passing (`grep "PASS:"`), 68 packages, `go test ./...`
  exit 0; `go vet` clean on vertex + compat; `GOOST=linux` build clean;
  `go.mod` / `go.sum` unchanged.
  (`gofmt -l` flags `vertex/auth.go` — `file(1)` confirms the WHOLE file is CRLF
  and `gofmt -d` is the whole-file CR diff; this is the pre-existing artifact
  next.md records for `auth.go`. My added lines follow the file's convention; the
  new `metadata.go`/`metadata_test.go` and the edited `vertex.go`/`compat.go` are
  LF-clean and unflagged.)

## Scope notes
- New capability, additive: the service-account path is byte-for-byte unchanged
  when metadata isn't opted in. No new dependency (stdlib `net/http` + existing
  `httpread`). No format/protocol change.
- Deliberately out of scope (clean follow-ups): external/federated Application
  Default Credentials (Workload Identity Federation with non-GCP IdPs); reusing
  `MetadataTokenSource` for the Google (non-Vertex) provider; AWS ambient creds
  (IRSA/assume-role) — the symmetric gap on the Bedrock side.
