# W3.16 — three tools name the artifact row where it lives: the contract

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the cheap end of the plugin-reach list. The browser, the code
executor and the fetch tool each reached into the artifacts module for
**one type** — the artifact row — and nothing else. The row was already a
contract row; the module merely aliased it. The tools named the alias, and the
alias is a module's implementation package, so each of the three paid a
`plugin-reach` edge for a type that was never theirs to begin with.

## What changed

| Before | After |
|---|---|
| `plugins/tools/{browser,codeexec,fetch}` → `artifact.Entry` | → `remoteapi.Artifact` (`kernel/contract/remoteapi`) — the row's real home |
| `plugin-reach plugins/tools/browser -> kernel/modules/artifacts/artifact` | **gone** |
| `plugin-reach plugins/tools/codeexec -> kernel/modules/artifacts/artifact` | **gone** |
| `plugin-reach plugins/tools/fetch -> kernel/modules/artifacts/artifact` | **gone** |

The module keeps its name: `artifact.Entry` is still `remoteapi.Artifact`, an
alias, so every other caller is unchanged. The row's home was already the
contract — `kernel/modules/artifacts/artifact/index.go` says so in its own
doc comment — the three tools simply named it the long way round.

Two small extractions make the row's wire testable where it is produced: the
fetch tool's response map became `artifactSaveRecord`, and the code
executor's export record became `artifactExportRecordOf`. The behaviour is
identical — the same keys, the same values, the same JSON.

## The ratchet went DOWN

**281 packages, 114 import violations, 13 call sites** — from 117/13. The
**three stale allowlist entries were deleted** — a leftover entry fails the
archcheck gate (the debt was paid, the list must shrink).

## The proof

### Permanent tests

- **The wire conformance** (`plugins/tools/{browser,codeexec,fetch}/artifact_wire_test.go`)
  states that the artifact record a tool reports IS the contract row's wire,
  key for key: every key the tool emits must be a key the contract row
  carries, the value must be the row's own, and the **exact key set** is
  pinned (a dropped key would leave the model with a field it expects and
  never gets).

  Nothing tied them together before: the record is a hand-written map or
  struct, so a renamed contract field would have left the tool emitting an
  empty value under a key nobody reads any more — silently, for every
  consumer of the tool's answer. The contract's own wire-shape test would
  have failed (and the developer would have updated it), and the tools would
  have kept reporting the old key with an empty value. These tests are what
  closes that gap.
- **The identity assertion** in
  `kernel/modules/artifacts/artifact/contract_type_test.go` was strengthened to
  the **function type** form (the W3.13/W3.14 recipe): the module's `Entry` IS
  the contract's `Artifact`, as a type, not as a shape. The old
  `reflect.TypeOf` comparison would also have accepted a copy with the same
  fields.

### Mutation run

**9 mutations: 8 caught, 1 build error (recorded separately).**

| # | Mutation | Result |
|---|---|---|
| M01 | the browser emits `mimetype` instead of `mime` | caught |
| M02 | the browser reports the name under the ref key | caught |
| M03 | the browser drops the kind key | caught |
| M04 | the code executor exports the kind under the mime key | caught |
| M05 | the code executor exports a zeroed size | caught |
| M06 | the fetch tool drops the ref key | caught |
| M07 | the fetch tool reports the mime under the name key | caught |
| M08 | the module's row stops being the contract's row (a local copy) | **build error** — the identity assertion fires at the seam: `cannot use (func(artifact.Entry) literal) as func(remoteapi.Artifact) value` |
| M09 | the browser reports a zeroed size | caught |

M08 is recorded as a build error, not an escape: a build failure is never
counted as one. The probe confirms the error is the identity assertion and
not an incidental one — the module's own fields compile fine against a
complete copy, and the assertion is what refuses the copy.

### Gates

The build, gofmt, vet, dead-code, archcheck (**281 packages, 114/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## What is deliberately left

- The **artifacts tool** still reaches into the module: it needs the store's
  `Index` (the implementation), the `Filter` and the `ErrNotFound` sentinel.
  That is a ports rewrite, not a rename.
- The other plugin-reach edges each need their own slice: the config tool,
  the schedule tool, the standing tool, the database tool, the workboard
  tool, the skill tool, the forgetool, the MCP tool, the introspector, the
  guardians and the market plugin. Every one of them names rows that live in
  a module's implementation package — the same shape as this slice, one at a
  time.
