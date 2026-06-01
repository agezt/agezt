# Phase Report — Milestone M52 (Sub-agent answer preview on the delegation arc)

> Status: **shipped** · Date: 2026-06-01
> SPEC-12 (multi-agent orchestration). Eleventh step on the multi-agent axis —
> a small follow-on that completes the delegation story: from "did it succeed,
> and what did it cost" (M44/M50) to "and what did it say".

## Why

M51 journaled every run's final answer. M44/M50 made the lead's arc show each
delegation's outcome and cost (`↳ completed (1 iters, 42ms, $0.0021)`). The one
thing still missing inline was the delegation's *result*: to learn what a
sub-agent actually answered, an operator had to copy the child correlation and run
`agt runs show <child>`. M52 folds M51's answer into a one-line preview on the `↳`
line, so the lead's arc shows what each delegation said — the last drill-down the
delegation view required.

## What shipped

- **`runEntry.AnswerPreview` + fold (`kernel/controlplane/runs.go`)** —
  `collectRuns`'s `task.completed` case now extracts a one-line excerpt of the M51
  `answer` field via `extractAnswerPreview`: whitespace runs collapsed to single
  spaces, trimmed, truncated to 80 runes with an ellipsis. The excerpt is computed
  server-side so the list payload stays small.
- **`answer_preview` per row (`handleRunsList`)** — each runs-list row exposes the
  excerpt; `""` when the run had no text answer.
- **`childOutcome.answerPreview` + render (`cmd/agt/runs.go`)** — `cmdRunsShow`
  pulls the preview from the row it already fetches into `childOutcome`;
  `renderTaskArc` appends it to the `↳` line as `: "<preview>"`. Shown only when
  non-empty, so a still-running or text-less delegation just shows its outcome.

## Design decisions

- **Preview server-side, not the full answer.** The runs-list row already carries
  per-run metadata; adding an 80-rune excerpt keeps it light. The full answer
  stays one `agt runs show <child>` away (and is journaled per M51). Folding the
  excerpt in `collectRuns` means both `runs list` and `runs show` get it for free
  from the same walk.
- **Collapse to one line.** A final answer can be multi-paragraph; the `↳` line is
  a single line, so `strings.Fields`-join collapses all whitespace (newlines,
  tabs, runs of spaces) to single spaces before truncating — the preview never
  breaks the arc's layout.
- **Quote it.** The preview renders as `: "<text>"` (Go `%q`), so its boundaries
  are unambiguous against the surrounding `(… iters, …)` metadata and any internal
  punctuation is visually contained.
- **Reuse, don't re-journal.** M52 is pure derivation over M51's `answer` — no new
  event, no schema change, no extra round-trip. `collectRuns` folds it, the row
  carries it, the renderer shows it.

## Tests

- `kernel/controlplane/runs_test.go::TestRunsList_RowCarriesAnswerPreview` — an
  answer with newlines/tabs/multiple spaces folds to a single-spaced one-line
  excerpt on the row.
- `TestRunsList_AnswerPreviewTruncated` — a 200-char answer is truncated to the
  80-rune cap with an ellipsis; the row never carries the full text.
- `cmd/agt/runs_show_test.go::TestRenderTaskArc_DelegationShowsAnswerPreview` — a
  child outcome with a preview renders `↳ completed (1 iters, 42ms): "<preview>"`.
- The M44/M50 `↳` tests (no preview set) are unchanged and still pass — the
  preview suffix appears only when present.

Test count: **1281 → 1284**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, added lines gofmt-clean.

## Live proof (offline mock + AGEZT_DEMO_DELEGATE=3 + AGEZT_SUBAGENT_FANOUT=2)

```
$ agt runs show <lead>
  …
    delegated → run-…  (task: subtask 1)
      ↳ completed (1 iters, 1ms, $0.0021): "[offline-mock sub-agent 1] done."
    delegated → run-…  (task: subtask 2)
      ↳ completed (1 iters, 1ms, $0.0021): "[offline-mock sub-agent 2] done."
```

The lead's arc now shows each delegation's outcome, cost, AND result inline — no
drill-down needed to see what a sub-agent said.

## What's next

The delegation view is now complete: link → task → outcome → cost → result. The
multi-agent axis has no remaining inline gaps. Sharpest remaining frontiers (all
small / off-axis):

1. **Tenant-scoped `why`** (LOW-MED) — route `handleWhy` via `kernelFor` (M39
   pattern) + a tenant-token allowlist; the last non-tenant-aware control surface.
2. **Boot-banner the delegation caps** (LOW) — echo the active depth / fan-out /
   spend ceilings at daemon startup, alongside the model-advisory / recovery banners.
3. **`agt runs stats` spend percentiles** (LOW) — extend the M47 spend aggregate
   with a per-run cost distribution (avg/p50/p95), mirroring the duration block.
4. **`agt runs list` answer preview column** (LOW) — `answer_preview` is now on
   every row; show it (truncated) in the flat list too, not only on the `↳` line,
   so `agt runs list` previews each run's result.
