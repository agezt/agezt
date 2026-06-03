# M222 — `agt plugin new` scaffolder (create-agezt-plugin)

## Why
M221 shipped the Go plugin SDK but left a gap: to use it an author still had to
*know* the shape — copy `plugins/sdk/example/greet` by hand, write a `go.mod`,
remember the `AGEZT_PLUGINS` wiring. ROADMAP §6 names the fix directly:
`create-agezt-plugin`. This milestone realises it as a CLI subcommand so the SDK
goes from "copy the example" to "one command to a working plugin."

## What
`agt plugin new <name> [--dir <path>] [--module <modulepath>]` scaffolds a
complete, buildable plugin project:

- **`main.go`** — an SDK plugin with one example tool, derived-and-sanitised
  tool name. The source is generated from a template and then run through
  `go/format` (`format.Source`), so the output is **guaranteed valid Go and
  always gofmt-clean** — the formatter doubles as a correctness gate.
- **`go.mod`** — `module <path>` + `require github.com/agezt/agezt v<version>`
  (the current `brand.Version`) + a commented local-dev `replace` hint.
- **`README.md`** — build (`go build`), run (`AGEZT_PLUGINS=...`), pin
  (`agt plugin hash`), and develop notes.
- **`.gitignore`** — ignores the built binary.

Safety: refuses to write into a non-empty directory (never clobbers existing
work); validates that the name yields a usable tool identifier;
`sanitizeToolName` reduces an arbitrary name to the conservative
letters/digits/underscore/dash set agezt tool names use.

Wired as `agt plugin new` alongside the existing `hash` / `list` subcommands.

## Files
- `cmd/agt/plugin_new.go` — `cmdPluginNew`, `sanitizeToolName`, and the
  `main.go` / `go.mod` / `README.md` renderers (new).
- `cmd/agt/plugin.go` — dispatch `new` + help text (edited).
- `cmd/agt/plugin_new_test.go` — 8 tests (new): full scaffold + parse +
  gofmt-clean + SDK import + tool name + go.mod assertions; default-dir;
  `--module` override; non-empty-dir refusal (and existing file untouched);
  missing-name; unknown-flag; unusable-name; `sanitizeToolName` table.

## Verification
- `go test ./cmd/agt/` — green; full suite **1713 → 1721** (+8), 66 packages.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./cmd/agt/` clean.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.
- **Live proof (gold standard):** ran `agt plugin new demo`, added a local
  `replace` to this checkout, and built the scaffold **network-free**
  (`GOPROXY=off go build`) into a 3.2 MB binary; then drove its protocol by hand
  — `initialize` returned the `demo` tool definition and `tool/invoke` returned
  `"demo received: hi"`. The scaffolder produces a genuinely working plugin.

## Caught in the act
The live build surfaced a real bug the unit tests had missed: the `go.mod`
`require` line used `brand.Version` verbatim (`1.0.0`), but module versions are
semver-with-leading-`v` (`v1.0.0`). Fixed (the renderer now prefixes `v`); the
test assertion was tightened to match, and the live build then succeeded.

## Scope notes
- Go scaffolder only. A standalone `create-agezt-plugin` binary and the
  polyglot (ts/py/rust) SDKs remain later milestones.
- Generated projects `require` agezt at the current version; until that tag is
  published, authors use the `replace` hint for local builds (documented in the
  generated go.mod + README).
