# M161 — Per-run override args: type-validated, not silently mis-handled (code review)

## Why
The control-plane run-submission path (`handleRun`) grew a per-run override family
one milestone at a time (M148 model, M149 system, M154 timeout, M158 tools, M159
dry-run, plus M91 images). Each was wired in with the idiomatic comma-ok type
assertion on the decoded-JSON arg:

```go
modelOverride, _ := req.Args["model"].(string)
if dr, _ := req.Args["dry_run"].(bool); dr { ... }
```

An independent code review of the accreted path flagged that this pattern
**collapses two different situations into the same zero value**: "the arg is
absent" and "the arg is present but the wrong JSON type." A client-side typo
therefore becomes silent wrong behavior, not an error. Two cases are genuinely
harmful:

- **`dry_run` mistyped** (e.g. the string `"true"` instead of the boolean
  `true`): the `bool` assertion fails, `dr` is `false`, and the run **executes for
  real — spending tokens** — when the entire point of `--dry-run` is "don't spend
  a token."
- **`tools` mistyped** (e.g. a bare string `"shell"` instead of `["shell"]`): the
  `[]any` assertion fails, the allow-list stays empty, and `WithTools(ctx, [])`
  scopes the run to **zero tools** — a silent pure-reasoning run with no diagnostic.

`timeout`/`model`/`system`/`images` had the milder version (a mistyped value is
silently dropped, so the run uses a surprising default or drops an attachment).

## Fix
New typed accessors in `kernel/controlplane/args.go` that return `(value, present,
error)` and distinguish all three states — absent (`present=false`), present-OK,
and present-but-wrong-type (`present=true, error!=nil`):

- `argString(args, key)` — string arg (returned verbatim; caller trims).
- `argBool(args, key)` — boolean arg.
- `argStringList(args, key)` — JSON array of strings, trimming + skipping empty
  elements; a present-but-non-array value, or a non-string element, is an error.

`handleRun` now:
- validates each override and returns a `RespError` (`args.X must be a …`) for a
  wrong-typed value instead of swallowing it;
- parses each arg **once** into a local, reused by both the real run and the
  dry-run `runPlanInput` — the dry-run plan previously re-read `req.Args["system"]`
  / `["timeout"]` / `["tools"]`, an accreted double-parse that could drift from the
  real run on any future change. Now the plan is built from the same locals, so it
  is structurally impossible for the preview to disagree with what would execute;
- stores the `--system` override **trimmed** (it was passed verbatim, so `" hi "`
  reached the provider with surrounding whitespace — every other override was
  already trimmed).

## Reviewed-and-confirmed-correct (left unchanged)
The same review checked the surrounding concurrency and confirmed it correct, so
it was deliberately NOT touched:
- `filterTools` allocates a fresh map and never mutates `k.tools` (built once in
  `Open`, never reassigned) — concurrent runs are safe.
- The `--no-tools` (present-but-empty) vs unset distinction is modeled correctly
  end to end.
- `effModel` is used consistently by the vision gate, the dry-run plan, and the
  real run.
- The dry-run short-circuit creates no subscription/goroutine and returns after a
  single response — no leak.
- The run goroutine + cancel-on-disconnect watcher + final-event drain
  (`*event.Event`, so the `ev == nil` close-check is sound) are pre-existing
  M31/M35 concurrency and were left as-is.

## Tests (+12, all passing)
- `args_test.go` — `TestArgString` / `TestArgBool` / `TestArgStringList`: absent vs
  present-OK vs present-but-wrong-type, including the `dry_run`-as-string and
  `tools`-as-bare-string traps, element trimming/skipping, and non-string elements.
- `run_argvalidation_test.go` (server-level, via `startPair` against a real
  control plane):
  - `TestRun_RejectsWrongTypedArgs` (7 subtests) — `dry_run`/`tools`/`model`/
    `timeout`/`system`/`images` wrong-typed each return the matching usage error.
  - `TestRun_WellTypedArgsStillRun` — `dry_run:true` returns a plan (no execution);
    an ordinary run then completes (the mock stayed fresh, proving the dry-run
    spent nothing).

## Live proof
The server-level tests above ARE the live proof (they exercise a real
`controlplane.Server` + `Kernel` + `Client`). Additionally, a CLI smoke test on a
mock daemon confirmed the refactor preserved every normal path:
`agt run --dry-run --tools shell --system "be terse" --timeout 45s "x"` resolved
`system prompt: per-run (--system)`, `timeout: 45s (per-run)`, `tools: restricted
(1): shell`, and a subsequent real `agt run` completed normally.

## Verification
- `go.mod` / `go.sum` unchanged; no new protocol command or env var.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./... -count=1` — **FAIL 0**, **1523 tests** (was 1511; +12), 61 packages.

## Result
A mistyped per-run override is now a clear usage error at the submission boundary,
not silent wrong behavior — most importantly, a malformed `dry_run` can no longer
execute (and bill for) a run the operator meant only to preview, and a malformed
`tools` can no longer silently strip every tool. The dry-run plan and the real run
are now guaranteed to read from the same parsed values.
