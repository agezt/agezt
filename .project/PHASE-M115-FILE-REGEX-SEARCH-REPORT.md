# Phase Report — Milestone M115 (file tool regex search)

> Status: **shipped** · Date: 2026-06-02 · agent capability.

## Why

The file tool's `search` matched a literal substring only — an agent looking for
a code pattern (every function definition, a `TODO(\w+)`, an import line) had no
way to express it. Adding an opt-in RE2 regex mode makes `search` a real
code-grep for the agent, completing the file tool's editing story alongside M114's
`replace`.

## What shipped

- **`search` gains `regex: true`** — when set, `pattern` is compiled as an RE2
  regular expression and matched per line; otherwise the existing literal
  substring behaviour is unchanged (back-compatible default). A malformed regex
  errors loudly (`bad regex: …`) rather than silently matching nothing.

## Design notes

- **Opt-in, default-unchanged.** Existing callers pass no `regex`, so literal
  search behaves exactly as before — zero behaviour change for current agents.
- **RE2 (Go `regexp`)** is linear-time and safe against catastrophic backtracking,
  so an agent-supplied pattern can't hang the daemon.

## Tests

- `TestSearch_RegexMode` — `func \w+\(` finds nothing literally but matches both
  function defs in regex mode (count 2, Foo + Bar).
- `TestSearch_BadRegexErrors` — an unbalanced `func(` regex errors.

Test count: **1384 → 1386**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean. (Unit-proven: regex is a matcher swap on the already
live-proven `search` op.)
