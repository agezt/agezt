# M474 — Tenant: heal a blank token file instead of wedging the tenant

## Context
Each tenant has a persistent token in `<tenant-dir>/.tenant-token`, minted on first
`Acquire` (under the registry lock) and read by `Token`/`Authorize`.
`loadOrMintToken` writes it with `O_CREATE|O_EXCL` so concurrent first-mints have a
single winner; the loser reads the winner's token.

## The bug (MED)
The write is two steps — `OpenFile(O_EXCL)` creates a **zero-length** file, then
`WriteString` fills it. A crash in that window leaves a zero-length token file. From
then on the tenant is wedged:

```go
if b, err := os.ReadFile(p); err == nil {
    if tok := strings.TrimSpace(string(b)); tok != "" { return tok, nil }
}   // empty → fall through to mint
...
f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
if err != nil {
    if os.IsExist(err) {
        b, _ := os.ReadFile(p)
        return strings.TrimSpace(string(b)), nil   // empty → returns "" FOREVER
    }
    ...
}
```

Every call: top read sees empty → falls through → `O_EXCL` fails (file exists) →
re-reads empty → returns `("", nil)`. `Token()` returns a blank credential with no
error, and the tenant can never authenticate again (`Authorize` fails closed on a
blank token, so it's an availability/lockout bug, not an auth bypass). Manual file
deletion is the only recovery.

## The fix
In the `IsExist` branch, read with a brief retry, then reclaim if still blank:

```go
for attempt := 0; attempt < 50; attempt++ {
    b, rerr := os.ReadFile(p)
    if rerr != nil { return "", fmt.Errorf("tenant: read token: %w", rerr) }
    if t := strings.TrimSpace(string(b)); t != "" { return t, nil }
    time.Sleep(time.Millisecond)
}
if rmErr := os.Remove(p); rmErr != nil { return "", fmt.Errorf("tenant: reclaim blank token: %w", rmErr) }
return loadOrMintToken(dir)
```

The retry handles the legitimate µs-scale concurrent-write window safely — a live
winner finishes within the retry budget, so the loser reads the real token and the
single-winner property is preserved (no premature reclaim of a winner's
mid-write file). Only a file that stays blank past ~50 ms is treated as stale and
reclaimed, after which a fresh token is minted. The top-of-function read is left
unchanged (no reclaim there) so it can't race a concurrent winner.

## Test + negative control
`kernel/tenant/tenant_test.go`: `TestRegistry_HealsBlankTokenFile` — acquires a
tenant, truncates its `.tenant-token` to zero length (the crash artifact), then
asserts `Token()` returns a fresh non-empty token and that it is stable on re-read.

**Negative control:** reverting to the old swallow-empty branch made `Token()` return
`""` — the test FAILED with `tenant wedged (no self-heal)`. Restored; test passes.

## Provenance
From the scoped review of kernel/tenant + kernel/tenantctx (tenantctx and the rest
of tenant — id validation, path containment, constant-time auth, locked map access —
all reviewed CLEAN; no cross-tenant leak or id-spoofing path).

## Verification / gate
- `kernel/tenant` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
