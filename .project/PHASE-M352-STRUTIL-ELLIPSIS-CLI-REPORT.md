# M352 — Rune-safe truncation: cmd/agt CLI sites (completes the class)

## Why
Completes the rune-safe-truncation bug class started in M349/M350 and centralised
in M351. The remaining byte-slice truncations all lived in the `cmd/agt` CLI
(package `main`) and displayed exactly the content most likely to be multi-byte
for this operator: run **intents**, sub-agent **task** strings, **pulse** text, and
generated-**plan node** labels — Turkish intents (ç/ş/ğ/ı/ö/ü) split at a byte
boundary would render a broken rune in `agt runs`, `agt pulse`, and `agt plan`.

## What
`cmd/agt` already had a `truncate(s, n)` helper (in check.go); the other sites
hand-rolled the same `s[:n] + "…"` byte slice. The fix routes them all through one
rune-safe path:
- **`cmd/agt/check.go`** — `truncate` now delegates to `strutil.Ellipsis(s, n, "…")`
  (rune-safe). Every existing caller is fixed for free.
- **`cmd/agt/runs.go`** — the inline intent (`[:69]`) and sub-agent-task (`[:59]`)
  slices now call `truncate(...)`.
- **`cmd/agt/pulse.go`** — the pulse-text excerpt (`[:160]`) now calls `truncate`.
- **`cmd/agt/plan_visualize.go`** — the plan-node body (`[:maxLen-1]`) now calls
  `truncate`.
- **`cmd/agt/check_test.go`** — `TestTruncate_RuneSafeCLIHelper`: 40×`ş` truncated
  at cap 7 (mid-rune) → valid UTF-8, no `�`; under-cap ASCII returned unchanged.

## Verification
- `go test ./cmd/agt -run Truncate_RuneSafeCLIHelper -v` — passes; existing cmd/agt
  tests (incl. `pulse_text_test`, `plan_visualize_test` which assert the `…` suffix)
  still pass.
- `gofmt -l` clean on edited files (a pre-existing comment-format diff in `runs.go`
  at line ~788, unrelated to this change, is left as-is per the leave-pre-existing-
  artifacts rule); `go vet` clean; `GOOS=linux go build ./...` exit 0. Full suite
  **2084** passing (was 2083; +1), `go test ./...` exit 0. `go.mod`/`go.sum`
  unchanged.

## Outcome — bug class closed
Every string truncation that reaches a user or the model is now rune-safe via a
single `strutil.Ellipsis` implementation: journal answer (was already safe),
tool-log preview (already safe), schedule intent (M349), coding diff (M349),
browser text (M350), status reason / plan snippet / AWS error excerpts / channel
history (M351), and the CLI intent/task/pulse/plan-node displays (M352).
Intentionally left byte-based: `rawBody[:MaxFetchBytes]` (feeds the HTML parser,
never shown as text) and hex/ULID prefix slices (`hash[:12]`, `id[:12]` — ASCII).
