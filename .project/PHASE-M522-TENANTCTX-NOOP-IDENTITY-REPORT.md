# M522 — Mutation testing tenantctx: pin the empty-id no-op as context identity

## Context
Thirty-second package in the mutation pass: `kernel/tenantctx` (the per-run tenant-id
context carrier — `WithTenant` / `Tenant`). Tiny (36 LOC, 2 mutants). Run with
`GOMAXPROCS=3`. go-mutesting score 0.500 (1 killed, 1 survived); tree restored clean.
After this milestone the package is at a full kill (1.0).

## The genuine gap (closed)
```
func WithTenant(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx          // survivor: → _ = ctx (drop the early return)
	}
	return context.WithValue(ctx, ctxKey{}, id)
}
```

The one survivor drops the early `return ctx`, so an empty id falls through to
`context.WithValue(ctx, ctxKey{}, "")`. `Tenant` then still returns `""` (the stored value
*is* `""`), so the existing `TestWithTenant_EmptyIsNoOp`, which only checks
`Tenant(ctx) == ""`, cannot tell the two apart — that is why it survived. But the
documented contract is stronger: "An empty id is a no-op (the context is returned
unchanged)." The mutant violates it by allocating a `valueCtx` wrapper on **every**
untenanted run — i.e. every run on the primary (single-tenant) kernel, the common case.

## Fix
Strengthened `TestWithTenant_EmptyIsNoOp` to assert **identity**: `WithTenant(base, "")`
must `== base`, not merely yield `Tenant() == ""`. (`go vet` and `staticcheck` are clean
on the `context.Context` comparison.)

## Negative control (manual, CPU-capped)
`return ctx → _ = ctx`: FAIL (the returned context is a new wrapper, `!= base`). Restored
byte-for-byte (`git diff --ignore-all-space` on tenantctx.go empty); passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — thirty-two packages (M490–M522)
…artifact, reflect, meshctx, tenantctx — plus the controlplane primary-token auth gate
verified solid. tenantctx now has every mutant killed (1.0). The gap class: a "no-op"
contract verified only by output value, not by the identity the contract actually promises.
