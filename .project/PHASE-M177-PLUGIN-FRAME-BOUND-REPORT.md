# M177 — Plugin stdout frame size bounded (anti-OOM)

## Why
A focused security review of the plugin host (`kernel/plugin/host.go`, the boundary
that launches out-of-process plugins and reads their newline-delimited JSON replies)
found a CRITICAL availability hole (finding C1): the read loop pulled each frame with

```go
line, err := p.stdout.ReadBytes('\n')
```

`bufio.Reader.ReadBytes` grows its internal slice without limit until it sees `\n` or
EOF. The plugin's stdout is an **untrusted** stream. A buggy or hostile plugin that
writes bytes but never emits a newline — or emits one pathologically large line —
drives the host to allocate memory without bound until the daemon is OOM-killed. One
plugin thus takes down every other plugin and the kernel, directly defeating the
"a misbehaving plugin must not crash the daemon" guarantee the protocol header states.

The stderr path was *already* bounded (a `bufio.Scanner` capped at 1 MiB per line,
`host.go`), so this was the matching, unguarded stdout hole.

## What
A stdlib-only bounded frame reader.

- **`DefaultMaxFrameBytes = 16 << 20`** and **`Config.MaxFrameBytes`** — the hard cap
  on a single stdout frame. 16 MiB is generous for legitimate JSON tool results while
  bounding the blast radius; configurable per plugin. Defaulted in `Spawn` next to the
  other timeouts, so `Reload`/`respawn` (which reuse `p.cfg`) inherit it.
- **`readFrame(r *bufio.Reader, max int) ([]byte, error)`** — reads in buffer-sized
  chunks via `ReadSlice('\n')` (which returns `bufio.ErrBufferFull` when a line is
  longer than the reader's 4 KiB buffer), copying each chunk out before the next read
  so the returned slice is stable. Once the accumulated frame would exceed `max` it
  returns `errFrameTooLarge` *instead of allocating further*. A trailing chunk with
  `io.EOF` (stream ended mid-line) is returned with that error — matching the prior
  `ReadBytes('\n')` contract the read loop already treats as fatal.
- **`readLoop`** now calls `readFrame(p.stdout, p.cfg.MaxFrameBytes)`. The existing
  `if err != nil { p.markDead(...) ; return }` path handles the overflow: the plugin
  is marked dead, in-flight callers fail fast (drained pending channels), the daemon
  lives.

No new dependency, no protocol change, no behavior change for well-behaved plugins
(legitimate frames are far under 16 MiB).

## Tests
- `kernel/plugin/frame_test.go` (white-box, pure): normal sequential frames; a line
  larger than bufio's 4 KiB buffer returned whole when under max (the multi-chunk
  `ErrBufferFull` path); **overflow rejected** with `errFrameTooLarge` for a small cap,
  across chunk boundaries (cap below 4 KiB), and a 1 MiB unterminated flood (the exact
  OOM scenario); EOF-mid-line returns the partial bytes with `io.EOF`.
- `kernel/plugin/flood_integration_test.go` + `testdata/floodplugin/` (live): a real
  child process that writes a 2 MiB unterminated blob to stdout on startup. With a
  256 KiB `MaxFrameBytes`, `Spawn` fails with an error mentioning the frame-size bound
  (the host tore the plugin down) rather than hanging or allocating unbounded. Proven
  in ~0.8s including the `go build` of the fixture. The shared echo fixture is
  untouched, so the existing "4 tools"/allowlist assertions are unaffected.

## Verification
- `go test ./...` — 1566 passing, 0 failing.
- `go vet ./kernel/plugin/` clean.
- `gofmt -l` clean on all touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `kernel/plugin/host.go` — `DefaultMaxFrameBytes`, `errFrameTooLarge`,
  `Config.MaxFrameBytes`, default in `Spawn`, `readFrame`, `readLoop` switched to it.
- `kernel/plugin/frame_test.go` — new, white-box unit tests.
- `kernel/plugin/flood_integration_test.go` — new, live teardown proof.
- `kernel/plugin/testdata/floodplugin/main.go` — new, hostile-plugin fixture.

## Follow-ups (from the same review, not in this milestone)
The plugin-host review surfaced further concurrency/DoS findings worth their own
milestones: H2 (`deathErr` read without synchronization — a real data race),
H3 (send-on-closed-channel in the dispatch/`Close` race), H4 (reload resets `nextID` →
id reuse / response confusion), M1 (unbounded callback goroutine fan-out — per-plugin
semaphore), M3 (`Kill` lacks a nil-`Process` guard), M4 (no process-group kill →
orphaned grandchildren). These are queued; C1 was the highest-severity and is fixed here.
