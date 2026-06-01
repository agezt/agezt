# Phase Report — Milestone M117 (file tool line-range read)

> Status: **shipped** · Date: 2026-06-02 · agent capability.

## Why

The file tool's `read` returned the whole file, truncating anything past 256 KiB
to the FIRST bytes — so an agent could never reach content deeper in a large file,
and had no way to read just the region around a `search` hit. Line-range reading
lets the agent page a file and jump to a specific span.

## What shipped

- **`read` gains `start_line` / `end_line`** (1-based, inclusive). With either
  set, `read` returns just those lines under a `[lines X-Y]` header. `start_line`
  alone reads a default 200-line window; a range is capped at 5000 lines and the
  256 KiB byte budget (marked `[truncated at byte cap]` if hit). Without the new
  fields, `read` behaves exactly as before.
- Pairs with M115 `search` (regex): find a match's line number, then read the
  surrounding lines — and the raw content is directly usable for an M114
  `replace`.

## Design notes

- **Scanner-based**, so a multi-GB file is read line-by-line without loading it
  all; only the requested window is buffered. Long lines tolerated up to 4 MiB.
- **Header, not per-line numbers** — the returned body is the exact file content
  of the range (no injected line-number prefixes), so it round-trips into
  `replace`; the `[lines X-Y]` header carries the location separately.

## Tests

- `TestRead_LineRange` — `[2,4]` returns L2..L4 and excludes L1/L5; `start_line`
  alone windows to EOF.
- `TestRead_LineRange_Errors` — end-before-start and a past-EOF range error
  cleanly.

Test count: **1389 → 1391**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live behaviour

```
read {path, start_line:2, end_line:4} →
[lines 2-4]
L2
L3
L4
```
