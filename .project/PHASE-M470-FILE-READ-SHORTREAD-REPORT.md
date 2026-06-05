# M470 — File tool: truncated-read fills the buffer and surfaces read errors

## Context
`doRead` (plugins/tools/file/file.go), for a file larger than `MaxReadBytes`,
returns a "first N bytes" preview.

## The bug (LOW)
```go
buf := make([]byte, MaxReadBytes)
n, _ := f.Read(buf)
out := fmt.Sprintf("[file truncated: showing first %d of %d bytes]\n%s", n, info.Size(), string(buf[:n]))
```

`(*os.File).Read` returns "up to len(buf)" bytes — it is not guaranteed to fill the
buffer in one call. So the model can receive an unpredictably short prefix while the
header claims it shows the first N (= `MaxReadBytes`) bytes. The read error is also
discarded (`n, _`), so a genuine I/O error is silently presented as truncated
content.

## The fix
A `readUpTo(r, max)` helper that loops past short reads with `io.ReadFull` and
surfaces real errors:

```go
func readUpTo(r io.Reader, max int) ([]byte, error) {
    buf := make([]byte, max)
    n, err := io.ReadFull(r, buf)
    if err == io.EOF || err == io.ErrUnexpectedEOF {
        err = nil // normal end-of-stream (file shorter than max)
    }
    return buf[:n], err
}
```

`doRead` now uses it and returns an error result on a genuine read failure instead
of silently truncating.

## Test + negative control
`plugins/tools/file/file_test.go`: `TestReadUpTo_FillsDespiteShortReads` — feeds
`readUpTo` an `iotest.OneByteReader` (one byte per `Read`, the worst case) over a
5000-byte source and asserts it returns the full 4096 bytes requested.

**Negative control:** reverting `readUpTo` to a single `r.Read(buf)` returned just 1
byte — the test FAILED with `read 1 bytes, want 4096`. Restored; test passes.

## Closes
This and M467 (atomic `replace`) close the round-7 file-tool findings. The remaining
noted item — `doWrite`'s overwrite path shares the truncate-then-write window — is a
"write the whole file" op that already `Sync`s; lower value than `replace` and left
documented.

## Verification / gate
- `plugins/tools/file` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
