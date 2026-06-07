# M502 — Mutation testing tenant: pin List's spurious-entry exclusion

## Context
Thirteenth package in the mutation pass: `kernel/tenant` (multi-tenant isolation).
Run with `GOMAXPROCS=3` (CPU-capped). Score 0.394 over 71 mutants (most survivors are
error-message / MkdirAll mutants).

## Security gate verified solid (no change)
`Registry.Authorize(id, presented)` is the per-tenant auth gate. Its surviving mutant
(`if err != nil || want == ""` → `&&`) is **equivalent**: `presented == ""` is guarded
above, so a non-empty presented vs an empty/garbage `want` makes
`subtle.ConstantTimeCompare` return 0 → false regardless; the meaningful flips
(`err == nil`, `want != ""`) break all auth and are killed by the positive tenant-auth
tests (`TestRegistry_Authorize`, controlplane `TestTenantToken_*`). The constant-time
comparison and empty-token handling are robust.

## The genuine gap (closed)
`Registry.List()` enumerates tenants by scanning the root with
`if !e.IsDir() || !ValidID(e.Name()) { continue }` — only directories with valid ids
are real tenants. `TestRegistry_ListReflectsDiskAndOpenState` only ever creates valid
tenants, so the *exclusion* of spurious entries was unpinned: the mutation `||`→`&&`
**survived**, which would surface a stray file (non-dir) or an invalid-named directory
as a "tenant" — feeding malformed ids/paths to downstream tenant handling.

## Fix
`kernel/tenant/tenant_list_test.go` — `TestRegistry_ListExcludesSpuriousRootEntries`:
plants a non-dir file with a valid-id name (`strayfile`, excluded by `!IsDir`) and a
directory with an invalid id (`UPPER`, excluded by `!ValidID`) in the registry root
alongside a real tenant, then asserts `List()` returns only the real tenant.

## Negative control (manual, CPU-capped)
Applying the survivor (`|| → &&`) makes the test fail with
`List = [UPPER alpha strayfile]` (both spurious entries surfaced); restored
byte-for-byte (`git diff --ignore-all-space` on tenant.go empty); passes again.

## Verification / gate
- New test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — thirteen packages (M490–M502)
redact, journal, edict, netguard, event, creds, warden, governor, scheduler, bus,
cadence, runtime, tenant — plus the controlplane primary-token auth gate verified solid.
The tenant *auth* gate is verified robust (equivalent survivors); the genuine gap was in
tenant *enumeration*. Genuine gaps closed where they existed; the rest verified solid.
