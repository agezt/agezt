# M468 — Peer tool: rune-safe answer truncation

## Context
The peer tool caps a peer agent's answer at `MaxAnswerBytes` (60 KB) via
`truncate(s, max)` before returning it to the model.

## The bug (LOW)
`truncate` cut at a raw byte offset:

```go
return s[:max] + fmt.Sprintf("\n… [truncated %d bytes]", len(s)-max)
```

If byte `max` lands in the middle of a multi-byte UTF-8 rune (common in non-ASCII
answers), the slice leaves a partial rune (e.g. a lone `0xC5`), so invalid UTF-8 is
spliced into the result handed to the model. The sibling tools already fixed exactly
this — `browser.truncate` and `coding.truncate` back the cut up to a
`utf8.RuneStart` boundary with regression tests; peer was the odd one out.

## The fix
Back the cut up to a rune boundary, mirroring `coding.truncate`:

```go
cut := max
for cut > 0 && !utf8.RuneStart(s[cut]) {
    cut--
}
return s[:cut] + fmt.Sprintf("\n… [truncated %d bytes]", len(s)-cut)
```

## Test + negative control
`plugins/tools/peer/peer_test.go`: `TestTruncate_RuneSafeAtByteBoundary` — builds a
string whose byte `max` is the continuation byte of `ş` (U+015F) and asserts
`truncate` returns valid UTF-8 with no replacement char.

**Negative control:** disabling the rune-boundary backup (`... && false`) produced
`"aaaaaaaaaaaaaaa\xc5\n… [truncated 11 bytes]"` — a lone `0xC5` — and the test FAILED
on `utf8.ValidString`. Restored; test passes.

## Verification / gate
- `plugins/tools/peer` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
