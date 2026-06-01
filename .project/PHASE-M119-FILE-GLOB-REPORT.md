# Phase Report — Milestone M119 (file tool glob)

> Status: **shipped** · Date: 2026-06-02 · agent capability.

## Why

The file tool could `list` ONE directory and `search` file CONTENT, but had no
way to find files by NAME across the workspace tree. An agent orienting in a
project ("where are the *.go files?", "find every test file") had to walk
directories one `list` at a time. `glob` answers it in one call.

## What shipped

- **`file` op `glob`** — `{op:"glob", pattern, path?}` walks the workspace (or the
  `path` subtree) and returns every file whose NAME matches the shell pattern
  (`*`, `?`, `[..]` via `filepath.Match`), workspace-relative, sorted, and capped
  at `MaxListEntries` (`capped:true` when hit). Directories are skipped; a bad or
  empty pattern errors cleanly.

## Design notes

- **Name-match across the tree** is the common agent need ("find all X files");
  it complements `list` (one dir) and `search` (content). Full `**` path globbing
  was intentionally left out to keep semantics simple and predictable.
- **Reuses workspace containment** (`resolve`), so glob can't escape the root.

## Tests

- `TestGlob_FindsAcrossTree` — `*.go` finds files at root + nested dirs, excludes
  README.md, count correct.
- `TestGlob_ScopedAndErrors` — `path` scopes to a subtree; empty and malformed
  patterns error.

Test count: **1394 → 1396**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Example

```
glob {pattern:"*.go"} →
{ "pattern": "*.go", "matches": ["main.go","pkg/sub/deep.go","pkg/util.go"], "count": 3, "capped": false }
```
