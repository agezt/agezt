# M377 — agentskills.io / ClawHub SKILL.md adapter (SPEC-13 §1.2)

## SPEC audit (read-vs-code)
SPEC-13 §1.2 (ecosystem interop) requires a `SKILL.md` adapter:

> A `SKILL.md` adapter ingests skills written to the agentskills.io open standard
> (which Hermes/ClawHub use): parse frontmatter (name/description/triggers),
> index the body, and load it into Agezt's skill system. Hundreds of existing
> skills become usable without rewriting. Agezt skills additionally get
> versioning, shadow-testing, and reversibility on top.

**Verified gap:** `agt skill import` (M269) only handled Agezt's own
content-addressed `*.skill.json` export bundle. A grep for
`SKILL.md|frontmatter|agentskills|clawhub` found **nothing** — the open-standard
Markdown ingestion was genuinely unimplemented. Offline-verifiable, priority-B
first-party gap.

## What
- **`kernel/skill/skillmd.go`** — `ParseSkillMD([]byte) (SkillMD, error)`: a
  deliberately minimal, **stdlib-only** frontmatter parser (Agezt takes no YAML
  dependency — go.mod unchanged) covering the forms these files use: scalars
  (`name: x`), inline lists (`triggers: [a, b]`), single-scalar lists, and block
  lists (`tools_required:` then `- item`). Case-insensitive keys; `tools` aliases
  `tools_required`; quotes stripped; CRLF tolerated; unknown keys ignored
  (forward-compatible). Requires a non-empty name and body; fails closed on
  missing/unterminated frontmatter.
- **`cmd/agt/skill_md.go`** — `agt skill import <file.md>` is routed (by
  `isSkillMarkdown`) to `importSkillMarkdownBytes`, which parses and installs via
  the existing `CmdSkillImport` path: a fresh DRAFT, content-addressed by the
  daemon, journaled, never auto-active (operator promotes). The `.skill.json`
  bundle path (content-address verified) is unchanged; help text updated.

## Verification
- **`kernel/skill/skillmd_test.go`** (3 pure tests): inline-list frontmatter +
  Markdown body; block-list + quoted scalars + `tools` alias + CRLF; error set
  (no frontmatter / missing name / empty body / unterminated).
- **Live daemon demo:** wrote a real agentskills.io `diagnose-failing-ci`
  SKILL.md (inline `triggers`, block-list `tools_required`, body), ran
  `agt skill import diagnose-ci.md` → "installed as a new draft", and
  `agt skill list` showed `[draft] diagnose-failing-ci — Diagnose and fix a red
  CI build`.
- `gofmt -l` clean; `go vet` clean; `GOOS=linux go build ./...` exit 0. Full
  suite **2140** passing (was 2137; +3), `go test ./...` 0 failures.
  `go.mod`/`go.sum` unchanged (no YAML dep). CHANGELOG (Added, user-visible).

## Scope notes
- SPEC-13 audited: §1.1 MCP universe (`mcpbridge` plugin exists); §1.2 SKILL.md
  adapter (this); §1.4 OpenAI-compat API as a backend (exists); §2 first-party
  catalog (channels/tools/providers); §3 self-growth (Forge). §1.3 `agt migrate
  openclaw|hermes` (SPEC-09 §6) remains a larger import-pipeline feature —
  recorded, not closed.
- The imported skill enters as a DRAFT (SPEC-13 §5 provenance + governance): it
  is not auto-active, so the operator reviews/promotes it — exactly the
  "imported capability enters via the trust ladder / shadow-test" property the
  spec wants over raw agentskills.io use.
