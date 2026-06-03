# M220 — Cut the v1.0.0 release (local)

## Why
The ROADMAP defines **v1.0 = M8: "federated mesh + multi-tenant — One Agezt across many
nodes."** Both halves are now complete and fused:

- **Multi-tenant isolation** — M14 (per-tenant kernels/tokens/ceilings), M38 (per-tenant
  authenticated control access), M39 (tenant-scoped run observability).
- **Federated mesh** — M200–M218: peer discovery (`agt peers models`), capability-aware
  auto-routing of `remote_run` by model, transport-fault failover, a bounded-TTL discovery
  cache, a delegation loop guard (hop-limited M209, audited M210, tunable M211,
  tenant-scoped M212, validated M213), auth-posture + reachability checks in
  `agt doctor`/`agt status`, and env-spec config hardening (M215–M218).
- **The intersection** — M219: per-tenant peer sets, leak-safe via kernel-stamped tenant
  identity, so each tenant federates to its own node set.

With the M8 capability scope genuinely met and tested (1693 passing, 0 failing), the
project reaches its v1.0 milestone. This milestone cuts that release **locally** — the
maintainer pushes/publishes when they choose (per the standing no-push constraint).

## What
- **`internal/brand/brand.go`** — `Version` bumped `0.1.0` → **`1.0.0`**; the doc comment
  now describes the Scale release. This is the canonical product semver; `agt --version`
  and the client/daemon skew check both read it (they stay equal — same binary version).
- **`CHANGELOG.md`** — the accumulated `[Unreleased]` block is released as
  **`## [1.0.0] — 2026-06-03`** with a Scale-release header summarising the mesh +
  multi-tenant fusion; a fresh empty `[Unreleased]` sits above it for future work.
- **`README.md`** — status line updated from "v0.1.0 — the MVP ships" to
  "v1.0.0 — Scale: One Agezt across many nodes", describing the mesh + multi-tenant fusion.
- **Git tag** — an annotated **`v1.0.0`** tag created locally (NOT pushed).

Scope-limited on purpose: the ACP `agentInfo.version` and `skill.DefaultVersion` literals
(`0.1.0`) are *separate* version fields (ACP protocol agent version; default version for a
newly-authored skill), not the product release semver, so they are intentionally left
untouched to keep this release cut minimal and test-safe.

## Verification
- `go test ./...` — 1693 passing, 0 failing (the version bump changes no behaviour; the
  only version test asserts non-empty, which still holds).
- `go vet ./internal/brand/` — clean.
- `gofmt -l` clean on `brand.go`.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged.
- `agt --version` → `agt 1.0.0 (protocol v1)`.
- Local commit + local annotated tag `v1.0.0`; **no push** (the maintainer publishes).

## Files
- `internal/brand/brand.go` — version bump.
- `CHANGELOG.md` — `[1.0.0]` release section + fresh `[Unreleased]`.
- `README.md` — v1.0.0 status line.

## Note on scope and authority
This is the **local** release cut. Publishing it (pushing the commit and the `v1.0.0` tag,
producing release artifacts/binaries) is the maintainer's action and is outside the
no-push constraint. The version decision itself was the maintainer's standing directive
("proje 1.0.0 sürüme gelene kadar") — executed now that the underlying M8 scope is real and
complete, not a cosmetic bump.
