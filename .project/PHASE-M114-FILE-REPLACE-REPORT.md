# Phase Report — Milestone M114 (file tool `replace` — partial edit)

> Status: **shipped** · Date: 2026-06-02 · agent capability.

## Why

The file tool could read, write (whole-file), append, list, and search — but
NOT edit in place. A `patch` op was explicitly deferred (file.go header). So an
agent changing one line had to read the whole file and rewrite it: expensive in
context, and risky (the rewrite can drop or mangle the rest). `replace` gives
the agent a surgical edit.

## What shipped

- **`file` op `replace`** — `{op:"replace", path, find, replacement, all?}`.
  Default semantics require `find` to match EXACTLY ONCE, so an ambiguous edit
  fails loudly (no silent wrong-place change); `all=true` replaces every
  occurrence. Guards: empty `find`, `find == replacement`, directory target,
  not-found, and the existing workspace-escape containment all error cleanly.
  Reports the occurrence count and byte delta.
- **Edict mapping** — `file.replace` maps to `CapFileWrite` (it's a write-class
  op, same risk/level as write/append: L2 Ask-First), so it's governed exactly
  like writing and isn't default-denied.

## Design notes

- **Unique-match-by-default** mirrors proven editor tooling: it turns "edited the
  wrong occurrence" from a silent corruption into an actionable error the agent
  can fix by adding surrounding context.
- **Reuses all existing containment** (`resolve` + symlink/`..` rejection), so
  the new op can't escape the workspace.

## Tests

- `TestReplace_UniqueMatch`, `TestReplace_AmbiguousRequiresAll` (errors + leaves
  file unchanged, then `all` works), `TestReplace_NotFound`,
  `TestReplace_GuardsEmptyAndIdentical`, `TestReplace_RejectsEscape`.
- `toolmap` table: `file replace` → `CapFileWrite`.

Test count: **1378 → 1384**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ AGEZT_DEMO_FILE_EDIT=1 agezt &   # agent writes notes.txt then edits it in place
$ agt run "update the notes file"
$ cat workspace/notes.txt
status = published                 # was "draft"
$ agt tool log
  ok  file  replaced 1 occurrence(s) in notes.txt (+4 bytes)
  ok  file  wrote 30 bytes to notes.txt
```
