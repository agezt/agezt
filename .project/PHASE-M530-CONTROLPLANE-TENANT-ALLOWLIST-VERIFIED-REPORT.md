# M530 — Verify the control-plane tenant command-allowlist (privilege boundary)

## Context
Extending the control-plane verification (after the M529 primary-token gate) to the
second authorization primitive: `tenantTokenAllows` (server.go:418), the deny-by-default
allowlist deciding which commands a scoped TENANT token may invoke vs. the admin-only
commands (halt, shutdown, tenant-registry management, cross-tenant stats, pulse,
durable-policy compaction). This is the M14/M38 tenant-isolation privilege boundary — a
defect here is a privilege escalation (a tenant running daemon-global commands) or a
denial (a tenant locked out of its own data). Run with `GOMAXPROCS=3` (CPU-capped).

## tenantTokenAllows — verified solid (both directions)
The real enforcement path: a non-primary token must name a tenant and pass
`s.tenants.Authorize`, then `if !tenantTokenAllows(req.Cmd) { forbidden }`. Both directions
of the allowlist were mutated by exact line and run against the integration tests:

- allow case `return true → return false` (line 84) — **killed**:
  `TestTenantToken_AuthorizesOwnTenant`, `…_AllowsOwnObservability`, `…_AllowsOwnRunStats`
  all fail (a tenant wrongly denied its own run/observe commands).
- default `return false → return true` (line 86, the **dangerous** direction — a tenant
  token able to run any admin command) — **killed**:
  `TestTenantToken_ForbidsNonAllowlistedCmd` fails (halt / tenant_list / tenant_stats /
  edict_compact would no longer be forbidden).

So the privilege boundary is genuinely pinned: tenant-routed commands are allowed, and
daemon-global / cross-tenant commands stay primary-only.

## Process note (honesty)
An earlier attempt with multi-line `perl` reported BOTH mutants as "surviving" — but the
`\t`-anchored multi-line pattern had silently failed to apply, so the tests ran against
unmutated source. Re-applying by exact line (`sed -i '84s/…/…/'`) with the function body
confirmed before each run showed both mutants are in fact killed. Lesson reinforced and
recorded in next.md: always confirm the mutant is actually in the source (grep/`sed -n`)
before concluding a survivor.

## Verification / gate
- No code change; `go test ./kernel/controlplane/` passes (`GOMAXPROCS=3`, `-count=1`).
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Control-plane authorization — both primitives verified
`tokenIsPrimary` (M529) and `tenantTokenAllows` (M530) — the two gates of the M38
authentication/authorization flow — are both verified solid by negative control. The
~40 command handlers remain covered by the 71 test files but not exhaustively
mutation-tested (intractable at ~10k LOC), as noted in M529.
