# M156 — `agt run --quiet` / `-q`

## Why
`agt run` prints a live token stream, a `[evt seq=… kind=…]` line per event, and a
correlation + usage footer — great for interactive use, noise for scripting. To
capture just the answer you had to use `--json` and extract the `answer` field with
`jq`. A `-q` that prints only the answer makes `agt run -q --file spec.md >
answer.txt` (composing with M151 file/stdin intent) clean.

## What
- **`cmd/agt/main.go`** — `--quiet` / `-q` flag on `cmdRun`. When set: the stream
  callback renders nothing (no token stream, no per-event lines), and after the run
  only the final answer is printed (one `Fprintln`), skipping the
  `--- final answer ---` header, the correlation line, and the usage line. `--json`
  takes precedence (machine consumption); errors still go to stderr with a non-zero
  exit.

## Files
- `cmd/agt/main.go` — `quiet` flag, gated rendering, help text.
- `cmd/agt/run_test.go` — `TestCmdRun_QuietFlagAccepted`.

## Tests (+1, all passing)
- `TestCmdRun_QuietFlagAccepted` — `-q` is a recognized flag (with no daemon the run
  fails at dial, exit 1, NOT an arg error, exit 2).

## Live proof (offline mock, real booted daemon)
```
$ agt run "hi"          # normal
  …14 lines (event lines + final answer + correlation + usage)…

$ agt run -q "hi"       # quiet
  [offline-mock] I ran a directory listing via the shell tool. …   ← 1 line, just the answer
```

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on touched files.
- `go test ./...` — **FAIL 0**, **1486 tests** (was 1485; +1), 61 packages.

## Result
`agt run -q` gives scripts the answer and nothing else, composing with `--file` /
stdin intent and the per-run `--model` / `--system` / `--timeout` overrides.
