# M546 — Mutation testing strutil.Ellipsis: pin the non-positive-max edge

## Context
`internal/strutil.Ellipsis` is the daemon-and-CLI-wide rune-safe truncation helper
(its whole reason to exist is "one fix covers every call site"). First `internal/`
mutation target. `GOMAXPROCS=3`.

## Result
`go-mutesting ./...`: 7 survivors. After analysis, **5 are genuine, 2 equivalent**:

```go
func Ellipsis(s string, maxBytes int, marker string) string {
	if len(s) <= maxBytes { return s }
	cut := maxBytes
	if cut < 0 { cut = 0 }                              // ← guard
	for cut > 0 && !utf8.RuneStart(s[cut]) { cut-- }    // ← rune-backing loop
	return s[:cut] + marker
}
```

Genuine survivors — all expose that `maxBytes == -1` and the empty-string + negative
cap are untested, and would **panic** under the mutation:
- `cut < 0 → cut < -1` (survivor 4): `Ellipsis(s, -1, m)` leaves `cut == -1` → `s[:-1]`
  panics. (Original clamps to 0 → returns the marker.)
- `cut > 0 → cut >= 0` / `→ true` / `→ cut > -1` (survivors 2/3/5): with an empty
  string and a negative cap, `cut` is clamped to 0 and the loop then indexes `s[0]`
  on `""` → index-out-of-range panic.

The function's own doc promises "a non-positive maxBytes yields just the marker" —
for *any* input, never a panic — but the test exercised only `0` and `-5` with a
non-empty string, missing `-1` (the value one below the clamp) and the empty string.

Equivalent survivors (left unpinned, no padding):
- `cut < 0 → cut <= 0` and `→ cut < 1` (survivors 1/6): differ from `cut < 0` only at
  `cut == 0`, where the body sets `cut = 0` — a no-op. Confirmed equivalent by
  applying each and seeing the suite still pass.

## Fix
Extended `TestEllipsis_NonPositiveMax` with `Ellipsis("abc", -1, "…") == "…"`,
`Ellipsis("", -1, "…") == "…"`, and `Ellipsis("", -3, "x") == "x"`.

## Negative control (manual, CPU-capped)
All four genuine mutants (guard `→ cut < -1`; loop `→ cut >= 0` / `→ true` /
`→ cut > -1`) make the strengthened test FAIL/panic; the two equivalent mutants keep
it green. Restored byte-for-byte (`git diff --ignore-all-space` on strutil.go empty).

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.
