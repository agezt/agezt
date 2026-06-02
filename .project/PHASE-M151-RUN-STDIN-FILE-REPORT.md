# M151 — `agt run` reads intent from stdin or a file

## Why
`agt run` only took its intent as a quoted command-line argument. A real, multi-
paragraph prompt — the kind you'd actually hand an agent ("here's a spec, here's
the constraints, do X then Y…") — is miserable to quote in a shell: newlines,
quotes, `$`, and backticks all fight you. The standard Unix answer is to read from
stdin (a pipe / heredoc) or a file. `agt run` lacked both.

## What
- **`cmd/agt/main.go`** — a new `resolveRunIntent(parts, file, stdin)` helper
  resolves the intent with clear precedence:
  1. `--file <path>` / `--file=<path>` — read the file (a read error is a usage
     error, exit 2);
  2. else if the sole positional arg is `-` — read all of stdin (the pipe
     convention: `cat prompt.txt | agt run -`, heredocs);
  3. else the joined positional text.
  All results are trimmed; an empty result still errors with the usage hint. `cmdRun`
  calls it with `os.Stdin`; the rest of the run path is unchanged (it composes with
  `--model` / `--system` / `--tenant` / `--json` / `--image`).

## Files
- `cmd/agt/main.go` — `resolveRunIntent`; `--file` flag; intent resolution + help.
- `cmd/agt/run_test.go` (new) — `TestResolveRunIntent_*`.

## Tests (+5, all passing)
- positional args join into the intent; `-` reads (trimmed, multi-line) stdin;
  `--file` reads the file and takes precedence over positional/stdin; a missing
  `--file` errors; empty input yields an empty intent (caller then errors with the
  usage hint).

## Live proof (offline mock, real booted daemon)
```
$ echo "what is this project?" | agt run -
  --- final answer ---
  [offline-mock] …                      # stdin intent ran

$ agt run --file prompt.txt
  …
  --- final answer ---
  [offline-mock] …                      # file intent ran (exit 0)
  usage: mock · 2 iteration(s)
```

(The `resolveRunIntent` unit tests cover the resolution logic directly; the daemon
run confirms the wired path.)

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./...` — **FAIL 0**, **1480 tests** (was 1475; +5), 61 packages.

## Result
A real prompt can now come from a pipe, a heredoc, or a file —
`agt run --file spec.md`, `agt run - <<'EOF' … EOF` — instead of being mangled into
a single shell-quoted argument, and it composes with the per-run `--model` /
`--system` overrides for fast, scripted experiments.
