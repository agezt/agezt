# M542 — Mutation testing acpagent: pin the output-cap inclusive edges

## Context
`plugins/tools/acpagent` drives an *untrusted external ACP agent* and relays its
streamed answer back into a governed run. Two layers bound that output so a
runaway/hostile agent can't OOM the daemon: an in-stream accumulation guard
(M256) and the final `truncate` (M256). Both edges were inclusive but unpinned at
the exact boundary. `GOMAXPROCS=3`.

## The two genuine gaps (closed)
```go
if answer.Len() >= MaxOutputBytes { return }   // in-stream accumulation guard
...
func truncate(s string, max int) string {
	if len(s) <= max { return s }              // final output cap
	return s[:max] + fmt.Sprintf("\n… [truncated %d bytes]", len(s)-max)
}
```

`bound_test.go`'s existing `TestACPAgent_BoundsRunawayStream` streams 200 KiB and
allows "a chunk or two" of slack, so it never distinguished `>=` from `>` at the
exact cap, and no test fed `truncate` a string of length *exactly* `max`. Both
inclusive boundaries survived:
- `answer.Len() >= MaxOutputBytes → >` — at an accumulation sitting exactly on the
  cap, `>` would append one more whole chunk before stopping.
- `truncate len(s) <= max → < max` — output that exactly fills the cap would be
  wrongly torn down with a `truncated 0 bytes` footer.

## Fix
Two precise tests:
- `TestACPAgent_RunawayGuard_StopsExactlyAtCap`: streams exactly `MaxOutputBytes`
  (60×1024) then one more chunk; asserts the result is exactly `MaxOutputBytes`
  with **no** truncation footer — i.e. the extra chunk was dropped at the
  inclusive boundary.
- `TestTruncate_InclusiveMaxBoundary`: `truncate(s, max)` returns `s` verbatim at
  `len==max`, and truncates with a `truncated 1 bytes` footer at `len==max+1`.

## Negative control (manual, CPU-capped)
- `answer.Len() >= MaxOutputBytes → >` (line 156): FAIL — accumulation = 61467,
  want exactly 61440; a footer appears. Restored byte-for-byte.
- `truncate len(s) <= max → < max` (line 183): FAIL — `truncate(len==max)` emits a
  `truncated 0 bytes` footer instead of the verbatim string. Restored byte-for-byte.
- `git diff --ignore-all-space` on acpagent.go empty after each restore.

## Verification / gate
- Tests pass; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Cross-package pattern note
This is the same inclusive-max DoS-guard idiom pinned in plugin `readFrame` (M509),
control-plane `readBoundedLine` (M531), and mcpbridge frame cap (M538) — here in
its two acpagent forms (in-stream guard + final truncate). Every bounded-output
edge in the untrusted-agent relay path is now pinned at its exact boundary.
