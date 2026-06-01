# Phase Report — Milestone M66 (`agt tool log` — tool-invocation audit)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

`agt tool list` shows the tools the daemon **advertises** to the model. Nothing
showed the tools the agent **actually called** and how each turned out. The
journal already records the full picture — every tool call emits a `tool.invoked`
(tool, call_id, input) followed by a `tool.result` (output, error) — but there
was no surface over it. An operator asking "what did the agent run, and what
broke?" had to grep raw journal events.

M66 adds `agt tool log`: the **execution analogue of `agt edict log`**. Where
`edict log` audits the policy *gating* of each call ("was it allowed?"), `tool
log` audits the *execution* ("what did it do?"). Together they close the loop on
a single tool call.

## What shipped

- **`handleToolLog` (`kernel/controlplane/tool_log.go`)** — folds the journal for
  `tool.result` events (the always-present event: a policy-denied call emits a
  result but no `tool.invoked`), joining each with its `tool.invoked` input by
  `call_id`. Newest-first, limited, tenant-scoped via `kernelFor`.
- **Filters** — `errors` (only failed calls), `tool` (one tool name), and
  `since_ms` (reusing the shared M65 `sinceCutoff` helper).
- **`previewString`** — collapses whitespace and truncates input/output excerpts
  to 100 runes, mirroring `answerPreviewRunes`' role for runs; keeps the response
  compact whether the output is one line or a megabyte.
- **CLI `agt tool log [N] [--errors] [--tool <name>] [--since <dur>] [--tenant
  <id>] [--json]`** — renders `<time> ok|ERROR <tool>  <output-preview>`.
- **Tenant-allowlisted** — a tenant token may audit its own tool calls
  (`CmdToolLog` added to `tenantTokenAllows`).

## Design decisions

- **Fold on `tool.result`, join `tool.invoked`.** The result is the event that
  always fires (even a denied call produces one), so it anchors each row; the
  invoked event only contributes the input annotation. This matches how the agent
  loop actually journals (agent.go:441 invoked, agent.go:500 result).
- **Raw-JSON input preview.** `tc.Input` is `json.RawMessage`, so the input shows
  its raw JSON form (`{"command":"dir"}`) — faithful to what the model sent.
- **Reuse, don't reinvent.** `sinceCutoff` (M65), the limit/sort/preview idioms,
  and the `decode*` pattern all follow the established log handlers, so the new
  surface agrees with the others by construction.

## Tests

- `TestToolLog_ListsAndJoinsInput` — newest-first ordering + input joined by call_id.
- `TestToolLog_FiltersErrorsAndTool` — `--errors` keeps only failures; `--tool`
  scopes to one tool.
- `TestToolLog_SinceWindow` — 1h window includes a just-published call; 1ms after
  a sleep excludes it.

Test count: **1305 → 1308**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt run "summarize this project"          # mock scripts a real shell tool call
$ agt tool log
  2026-06-01 13:11:28  ok    shell             Volume in drive D is New Volume … Directory of D:\Codebox\PROJECTS\L…
$ agt tool log --errors
  no failed tool calls.
$ agt tool log 1 --json
  { "invocations": [ { "tool": "shell", "call_id": "call-1",
    "input": "{\"command\":\"dir\"}", "output": "Volume in drive D …", "error": false } ], "count": 1 }
```
