# Phase Report — Milestone M59 (`agt runs list` answer preview)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 × SPEC-12.

## Why

M52 added `answer_preview` to every runs-list row (and surfaced it on the
delegation `↳` line), but the flat `agt runs list` view still showed only the
intent — not what the run produced. The data was already on the wire; M59 renders
it.

## What shipped

- **`renderRunRow` answer line (`cmd/agt/runs.go`)** — appends
  `answer  : "<preview>"` beneath the intent when `answer_preview` is present
  (M51/M52 fold), quiet when absent. Flows through both flat and `--tree` list.

## Design decisions

- **Pure render, no protocol change.** `answer_preview` was already on every row
  (M52); M59 is one render line. Quiet at empty (running runs, pre-M51 runs).

## Tests

- `cmd/agt/runs_list_test.go::TestRenderRunRow_ShowsAnswerPreview` — a row with a
  preview renders the answer line; a row without one omits it.

Test count: **1296 → 1297**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, added lines gofmt-clean.

## Live proof

```
$ agt runs list 1
  run-…
    started : … status: completed … iters: 2
    intent  : what is this project?
    answer  : "[offline-mock] I ran a directory listing via the shell tool. …"
```
