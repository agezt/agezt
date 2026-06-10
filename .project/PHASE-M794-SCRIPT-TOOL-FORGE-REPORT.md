# Phase M794 — script-tool forge: agent code becomes a callable tool

**Date:** 2026-06-10 · **Status:** DONE · **Frontier:** gap-analysis #4
(code→tool promotion — the close of the write→use→improve cycle).

## What

`kernel/toolforge`: a durable, journaled registry of **script tools** —
named scripts (Python/Node/Deno) that, once **tested and promoted**, are
offered to every run (and every sub-agent) as real callable tools named
`forge_<name>`, executed through the existing code_exec sandbox.

The governed pipeline: **draft → test → (operator) promote → live**, with
quarantine as the kill switch. The core invariant is *only tested code is
ever live*: `Promote` refuses (`ErrUntested`) until a sandbox test of the
CURRENT code passed, and any code/language edit demotes the tool back to
draft with its test record cleared.

## Design

- **Store** (`scripttools.json`, roster pattern): atomic rewrite, mutex,
  rollback-on-save-fail, ULID ids, unique immutable names
  (`^[a-z][a-z0-9_]{0,39}$`), validation (desc required ≤2KiB, code ≤128KiB,
  optional JSON-object input schema ≤16KiB). Statuses draft/active/quarantined.
- **Execution** rides code_exec untouched: `codeexec.Tool.RunScript(ctx,
  lang, code, inputJSON)` (new export) marshals a normal code_exec input —
  the call's raw JSON input becomes `stdin` (surfaced to the script as
  `./stdin.txt`), stdout+stderr come back, ephemeral dir, scrubbed env,
  warden limits. The kernel sees only the `toolforge.Runner` interface
  (`Config.ScriptRunner`); cmd/agezt wires the code_exec tool in — the
  kernel still never imports plugins.
- **Offering seam**: `k.mergeScriptTools(k.tools)` in RunWith — merged
  BEFORE the WithTools filter (a restricted run can't smuggle forged tools
  back) and BEFORE injectEnvironment (the model's tool preamble sees them).
  Same merge on the delegate child path. No-active-scripts path is
  allocation-free.
- **Edict**: every `forge_*` call maps to `code.exec` (it IS sandboxed code
  execution — promotion changes who can be called, never what the sandbox
  allows). New `tool.forge` capability (AskFirst, like `skill`) gates the
  authoring ops of the `tool_forge` agent tool; `op=test` maps to
  `code.exec` because it runs code.
- **Agent surface** (`tool_forge`, plugins/tools/forgetool): draft / update /
  test / list / show. Promotion is deliberately NOT a tool op — an agent can
  author and test, only the operator promotes (controlplane + CLI + webui).
- **Operator surface**: `agt toolforge list|show|draft|edit|test|promote|
  quarantine|remove` (code via `--file`); controlplane `toolforge_*` cmds;
  webui routes (GET /api/toolforge; POST test/promote/quarantine/remove as
  writeRoutes, draft/edit as jsonRoutes). Console view → next milestone.
- **Journal**: `scripttool.created/updated/tested/promoted/quarantined/
  removed` under subject `toolforge.<name>` — `agt why` explains how a tool
  came to be, who tested it, when it went live, when it was pulled.

## Tests (17 new across 6 packages)

- toolforge store: full lifecycle (draft never offered → promote refused
  untested → test → promote → quarantine → re-promote), code-edit demotes +
  clears test record, identity/lifecycle fields protected from mutators,
  validation table, name uniqueness, persistence round-trip, remove.
- runtime e2e (mock provider + stub runner): promoted tool OFFERED to the
  model and EXECUTED (runner receives stored lang/code + the call's raw
  JSON); draft/quarantined never offered; WithTools allowlist gates forged
  tools both ways; failed test recorded + promote refused.
- edict toolmap: `forge_*` → code.exec; tool_forge op routing (test →
  code.exec, rest → tool.forge).
- forgetool: authoring loop (draft/test/list/show/update-demotes), failing
  test reports FAILED + sandbox output, promote-is-not-a-tool-op, unbound.
- controlplane wire round-trip: the full operator pipeline incl. list
  doesn't leak code bodies, show does carry them, failed test keeps the gate
  shut.
- codeexec live (skips without Python): RunScript stdin.txt contract, exit
  code → isError, unavailable language honesty.

## Smoke (isolated AGEZT_HOME, real daemon, real Python)

draft weather.py → premature promote REFUSED ("no passing test") → `agt
toolforge test weather --input '{"city":"izmir"}'` ran in the real sandbox
and printed `weather for izmir: sunny, 28C` → promote → list shows ACTIVE →
`forge_weather` → quarantine → re-promote (test record survived) → code edit
demoted to draft/untested. Journal: created/tested/promoted/quarantined/
promoted/updated, all subject `toolforge.weather`. Daemon banner shows the
new tool line. Graceful shutdown; smoke dir removed.

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean;
linux cross-build OK; 457 vitest green (frontend untouched, dist unchanged);
go.mod unchanged; no new env vars (configEnvVars untouched).
CI org-billing still blocked → local battery + arc-authority merge.

## Next

M795: Forge console view (list/draft/test/promote from the web UI). Then
gap #3 governed self-install, #5 vector memory, #6 brain distiller.
