# Phase M844 — board tool infers a missing `op` (workflow node fix)

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "workflow içinden
tool board failed: board: op required (post|read|topics|send|inbox|reply|replies)
ile board'a mesaj yazamadı node."

## Root cause

A workflow **tool node** passes its `config.args` object verbatim as the tool's
input. The board tool **required** an explicit `op` and hard-errored ("op
required …") when it was absent. A board node configured with just
`{topic, text}` (the natural "post a message" shape) therefore failed.

## Fix (`plugins/tools/boardtool/tool.go`)

The board tool now **infers a missing `op`** from the supplied fields, before the
op switch:

- `text` + `to` → **send** (a direct message)
- `text` + `id` → **reply**
- `text` (alone or with a topic) → **post**
- nothing → **read** (the harmless default)

Explicit ops are untouched. The input schema no longer lists `op` as `required`
and documents the inference. So a workflow board node with `{topic, text}` now
posts, and an agent that forgets `op` does the obvious thing instead of erroring.

## Verification

- **Unit** (`TestInferOp`): op-less `{topic, text}` → post; `{to, text}` → send;
  empty `{}` → read (no error). `TestBadInputs` updated (an empty op is no longer
  an error — it reads). Board + workflow package tests green.
- The workflow runner's verbatim `args → tool.Invoke` path is unchanged; the fix
  is entirely in how the board tool interprets its input, so it applies to the
  reported workflow node, the agent, and the CLI alike.

## Gate

boardtool + workflow tests green; vet + staticcheck + linux clean; gofmt swept.
go.mod unchanged.
