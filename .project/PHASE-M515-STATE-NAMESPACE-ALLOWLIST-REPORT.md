# M515 — Mutation testing state: pin the namespace allowlist char-range edges

## Context
Twenty-sixth package in the mutation pass: `kernel/state` (the file-backed atomic
key/value store — per-namespace JSON, write-temp+fsync+rename, the `validateNamespace`
path-traversal guard). Run with `GOMAXPROCS=3` (CPU-capped). go-mutesting score 0.549,
50 survivors; working tree restored clean after the run.

## Triage
The error-handling survivors (statement-removal of the wrapped `fmt.Errorf` returns in
`Open`/`atomicWrite`/`snapshotLocked`) are killed by the persistence/atomic tests. The
M426 invalid-RawMessage poison guard is pinned by `TestSet_InvalidRawMessageRejectedNoPoison`.
The traversal *rejections* (`/ \ : $ .. .` empty) are pinned by `TestValidateNamespace`
and `TestValidateNamespace_EnforcedOnAllAccessors` across every accessor.

## The genuine gap (closed)
`validateNamespace` is the only guard turning a caller-controlled string into a filename,
so its allowlist defines exactly which namespaces are legal:

```
case c >= 'a' && c <= 'z':
case c >= 'A' && c <= 'Z':
case c >= '0' && c <= '9':
case c == '_' || c == '-' || c == '.':
```

The valid-namespace cases (`agents`, `config`, `agent_01H`, `ns-with-dash`, `ns.with.dot`)
only touch the LOW edges (`a`, `0`) and mid-range letters (`H`). So the FAR edges of each
range were unpinned, and four non-equivalent mutants survived (confirmed by hand-applied
negative control against the existing suite):
- `c <= 'z' → c < 'z'` — a namespace using `z` wrongly rejected.
- `c <= 'Z' → c < 'Z'` — `Z` wrongly rejected.
- `c >= 'A' → c > 'A'` — `A` wrongly rejected.
- `c <= '9' → c < '9'` — `9` wrongly rejected.
Each silently shrinks the legal-identifier set — a valid namespace becomes
`ErrInvalidNamespace`, so the store would refuse to read/write state under that name.

## Fix
Extended `TestValidateNamespace`'s `good` list with `"azAZ09"`, a single namespace that
sits on every range edge (`a`,`z`,`A`,`Z`,`0`,`9`) and must be accepted.

## Negative control (manual, CPU-capped)
The four edge mutants above each FAIL with the new case. Restored byte-for-byte
(`git diff --ignore-all-space` on state.go empty); passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — twenty-six packages (M490–M515)
redact, journal, edict, netguard, event, creds, warden, governor, scheduler, bus,
cadence, runtime, tenant, worldmodel, approval, memory, skill, standing, catalog, plugin,
webhook, channel, anomaly, restapi, acp, state — plus the controlplane primary-token auth
gate verified solid. The traversal-rejection core was solid; the gap was the un-pinned far
edges of the namespace allowlist (valid identifiers silently rejectable).
