# 02 — Operator CLI `agt` (cmd/agt)

**Scope:** `cmd/agt` (package `main`, 233 non-test `.go` files, ~37k LOC; 95 `_test.go` files) and its eight
subpackages `cmd/agt/{dial,format,haltresume,jsonout,keys,providerlookup,router,whoami}`.
Sibling docs: daemon side of every op → [03-control-plane-and-http.md](03-control-plane-and-http.md);
daemon boot / `internal/paths` / `internal/brand` → [01-daemon-boot-cmd-agezt.md](01-daemon-boot-cmd-agezt.md);
vault/catalog/edict semantics → [05-governance-routing-security.md](05-governance-routing-security.md);
journal/memory/world → [06-data-memory-state.md](06-data-memory-state.md); schedules/standing/skills/market/
workflow/plugins/acp → [07-autonomy-and-extensibility.md](07-autonomy-and-extensibility.md); provider adapters used by
`provider check` → [08-providers.md](08-providers.md); peer tool → [10-tools.md](10-tools.md).

## Responsibilities at a glance

- **Single operator binary** (`agt`, name from `internal/brand.CLI`) over a long-running daemon (`agezt`,
  `brand.Binary`). 71 top-level commands, ~400 subcommand verbs.
- **Thin RPC client** for ~95% of verbs: each command parses argv by hand, dials the daemon's local control plane
  (`kernel/controlplane.Client`, newline-delimited JSON over TCP localhost, token auth), issues ONE op
  (`controlplane.Cmd*`, 248 distinct constants used), renders the `map[string]any` reply as a table or `--json`.
- **Offline toolbelt** for the rest: vault, catalog sync `--local`, provider key setup/import/check, backup/restore,
  journal import + bundle verify, plan validate/visualize/cost, plugin scaffolding/hashing/registry, skill registry,
  market pack authoring/signing, peer mesh and Home Assistant clients, STT, JWT capability tokens, rollback catalog.
- **Governance-preserving bridges**: `agt acp` (Agent Client Protocol over stdio for IDEs) and `agt transcribe/listen
  --run` turn external input into a normal daemon `run`, so Edict/governor/journal still apply.
- **Uniform UX contract**: help table is the single source of truth (guard tests), `agt <cmd> -h` never executes the
  command, `--json` everywhere (pretty, via `jsonout.Write`), exit codes 0/1/2/3.

## Package map

| Package | Purpose | Internal deps (from import graph) | Depended on by |
|---|---|---|---|
| `cmd/agt` (main) | All command implementations, dispatcher wiring, help table | `cmd/agt/{dial,format,haltresume,jsonout,keys,providerlookup,router,whoami}`, `internal/{brand,paths,strutil}`, `kernel/{acp,agent,agentgw,assure,cadence,catalog,configcenter,controlplane,creds,event,governor,journal,market,meshctx,netguard,planner,plugin,skill,stt,webhook}`, `plugins/providers/compat`, `plugins/tools/peer` | — (binary) |
| `cmd/agt/router` | Generic `Command` shape + `Registry` (Register/Lookup/All/Execute/Suggest, Levenshtein ≤2) | none | main |
| `cmd/agt/dial` | `New(stderr)` / `NewAtBase(base, stderr)` → verified `*controlplane.Client` or nil + hint | `internal/{brand,paths}`, `kernel/controlplane` | main, haltresume, keys, whoami |
| `cmd/agt/jsonout` | `Write(w, v) int` — 2-space-indented JSON, returns 0 | none | main, whoami |
| `cmd/agt/format` | Pure renderers: `Bytes`, `Percent`, `Time`, `Duration`, `Uptime`, `Number`, `Str`, `ParseDuration`, const `DiskWarnPct=10.0` | none | main |
| `cmd/agt/haltresume` | `agt halt` / `agt resume` (`Halt`, `Resume` over private `run(action,…)`) | dial, brand, controlplane | main (cmd_register.go) |
| `cmd/agt/whoami` | `agt whoami` (`Run`) → `CmdWhoami` | dial, jsonout, brand, controlplane | main |
| `cmd/agt/keys` | `agt provider keys list/add/activate/rm` (`Run`) → `CmdProviderKey*` | dial, brand, controlplane | main (provider.go) |
| `cmd/agt/providerlookup` | `ScopedVaultLookup`, `CredentialLookup` — vault-then-env credential chain with duplicate-env scoping | `kernel/{catalog,creds}` | main (provider_lookup.go) |

**Layering notes.** `cmd/agt` imports `plugins/providers/compat` (to build providers in-process for
`provider check`) and `plugins/tools/peer` (AGEZT_PEERS parsing / mesh health for `peers`, `status`, `doctor`).
`plugin_new_render.go` mentions `plugins/sdk` only inside a generated-source string template (not a Go import).
The subpackages are the start of a "command-as-package" migration; only halt/resume, whoami and provider keys moved
so far (see each `doc.go`).

---

## 1. Entry and dispatch

```
os.Args ─► main() ─► run(args, stdout, stderr)                       main.go:38
             │ len(args)==0            → printHelp()                  help.go
             │ args[1] ∈ {-h,--help} && helpHas(args[0])
             │                         → cmdHelp(args[:1])            (M936 uniform -h; command NEVER runs)
             │ args[0] ∈ {-v,--version,version} → "agt <ver> (protocol vN)"
             │ args[0] ∈ {-h,--help,help}       → cmdHelp(args[1:])
             └ default → ExecuteCommand(name, rest)                   commands.go
                           └ registry.Execute (router.Registry)
                               ├ found   → cmd.Run(rest, stdout, stderr) → exit code
                               └ missing → "agt: unknown command "x" — did you mean a, b?" + "run `agt help`…" ; exit 2
```

- **Registration** happens in `init()` of `cmd_register.go` (`registerRunCommands`, `registerInfoCommands`,
  `registerAgentCommands`, `registerConfigCommands`, `registerManagementCommands`). Each is
  `Register(&Command{Name, Aliases, Run})`. `Register` (commands.go) writes both the live
  `router.Registry` (`registry`) and a parallel exported map `CommandRegistry` (used only by tests:
  `AllCommands()` in `commands_test_helpers_test.go`, help sync guards).
- `Command = router.Command` (type alias) — fields `Name, Aliases, Description, HelpLong, HelpHandler, Run`.
  In practice only `Name`, `Aliases`, `Run` (and `Description` for run/halt/resume) are populated; `HelpLong`
  and `HelpHandler` are unused by the binary (help comes from `help_data.go`).
- Aliases at top level: `exec-profile`↔`execution-profile`; `version`(`-v`,`--version`), `help`(`-h`,`--help`)
  — the latter two are intercepted in `run()` before dispatch, so their registered `Run` bodies are effectively dead
  (`version`'s registered Run is an empty `return 0`).
- **Subcommand dispatch** is hand-rolled per family: `cmdX(args)` does `switch args[0]` (or `if args[0]=="log"` for
  `approvals`, `budget`, `backup`, `acp`, `pulse`) and passes `args[1:]` down. Many verbs have aliases
  (`ls`/`list`, `rm`/`remove`/`delete`, `show`/`get`, `add`/`create`, …).
- **Flag parsing** is manual (`for i := 0; i < len(args); i++ { switch {...} }`) with `--flag v` and `--flag=v`
  forms; unknown flags are errors (exit 2) by design so typos are not swallowed as positionals. Exceptions use
  `flag.FlagSet` (`token create`).
- **Exit-code convention**: 0 success; 1 runtime/daemon error (including "daemon unreachable"); 2 usage error;
  3 domain-negative answer used by scripts (`edict test` deny, `state get` absent, `budget check` exhausted,
  `conductor` unverified, `skill diff`/`schedule test` not found).

### Help system (help.go, help_data.go)

- `helpGroups()` returns `[]helpGroup{title, []commandHelp{name, summary, detail[]}}` — 9 groups in
  task-frequency order: Getting started, Run & control, Plans & automation, Providers & models,
  Memory & knowledge, Journal & audit, Console/config & data, Channels & integrations, Daemon.
- `printHelp` (overview), `cmdHelp` (detail block; detail lines starting with a space are rendered as continuation
  lines), `helpHas` (gate for `-h` interception), `suggestCommands` (prefix/substring + `editDistance` ≤2, max 4).
  Note there are two "did you mean" engines: help-table based (`agt help typo`) and router-based (`agt typo`).
- Guard tests (`help_test.go`): `TestHelp_CoversEveryRegisteredCommand`, `TestHelp_DocumentsOnlyRegisteredCommands`,
  `TestHelp_EntriesAreComplete`, `TestUniformDashH`, `TestUnknownCommand_ShortErrorWithSuggestion`. The registry
  (71 names) and the help table (71 names) are currently in exact sync.
- Per-command `-h` inside subcommands (e.g. `agt schedule add -h`) is handled by each command's own parser, not the
  table.

---

## 2. How the CLI talks to the daemon

### Transport (cmd/agt/dial → kernel/controlplane.Client)

1. `dial.New(stderr)` → `paths.BaseDir()` (honours `AGEZT_HOME`) → `NewAtBase(base, stderr)`.
2. `controlplane.NewClient(base)` reads `<base>/runtime/control.addr` (TCP `host:port` on localhost) and the token:
   **`AGEZT_TOKEN` env overrides** `<base>/runtime/control.token` (primary/admin token) — this is how a tenant
   operator authenticates with a tenant token (`agt whoami` reports which).
3. Liveness probe (M239): a 2 s `CmdStatus` call. Transport failure → "daemon recorded but not responding — a stale
   socket from a crash?" + hint, return nil. A **server-side** error (`*controlplane.ErrServerError`, e.g. bad token)
   is NOT treated as unreachable — the client is returned so the command surfaces the real error.
   Missing runtime files → "Hint: start the daemon with `agezt`…".
4. Each call opens a fresh TCP connection (5 s dial timeout), writes one `Request{ID:"q-HHMMSS.mmm", Cmd, Token, Args}`
   JSON line (10 s write deadline), reads the response with a read deadline derived from the caller's `ctx`.
5. Client methods used by the CLI:
   - `Call(ctx, cmd, args) (map[string]any, error)` — the default.
   - `CallRaw` — only `journal export` (keeps the bundle bytes verbatim for re-verification).
   - `Stream(ctx, cmd, args, onEvent)` — `run` (main.go, main_run_modes.go, acp.go) and `plan` execution
     (main_approvals_plan_run.go): event frames are delivered to `onEvent(*event.Event)` until the final result.
   - `StreamUntilCancel` — `pulse` live tail (`CmdPulseSubscribe`), cancelled by `signal.NotifyContext(SIGINT,SIGTERM)`.
6. Timeouts are per command (typically 5–30 s); `run` keeps the connection open 15 min by default or
   `--timeout + 30 s`. The `overseer` family uses `context.TODO()` (no client-side deadline).

Most commands call `dialpkg.New` directly; a few build the client themselves without the probe
(`doctor`, `catalog sync`, `web password`, `quickstart`'s `persistProviderModel`) because they have an offline
fallback and must stay silent when the daemon is down.

### `agt run` request lifecycle (main.go `cmdRun`)

1. Parse flags anywhere in argv: `--json`, `-q/--quiet`, `--tenant`, `--model`, `--agent`, `--system`, `--timeout`,
   `--exec-profile/--execution-profile`, `--peer/--remote-peer` (requires `--exec-profile remote-agezt`),
   `--dry-run`, `--assure[=N]` (default `assure.DefaultMaxAttempts`), `--max-cost <usd>` (→ microcents via
   `parseUSDToMicrocents`), `--tools <csv>` / `--no-tools` (explicit empty list ≠ omitted), `--image <path>`
   (read locally, sent as `data:` URL by `loadImageDataURL`), `--file <path>`.
2. `resolveRunIntent`: `--file` > sole positional `-` (read stdin) > joined positionals.
3. Build `runArgs` map (`intent, tenant, model, agent, system, execution_profile, remote_peer, timeout, images,
   tools, dry_run, max_cost, assure`).
4. `--dry-run` → single `Call(CmdRun)` returns the resolved plan (`runDryRunMode`); `--json` → `runJSONMode`
   prints ndjson `{"type":"event",...}` lines then a final `{"type":"result"|"error"}` line.
5. Otherwise `Stream(CmdRun)`: `llm.token` payload text streams inline; `llm.reasoning` hidden unless
   `AGEZT_SHOW_REASONING=1`; every other event prints `[evt seq=N kind=K]`. Final block prints the answer,
   `correlation_id`, and a usage line (`model · iterations · $cost` from `model`, `iters`, `spent_mc`).

### Offline vs daemon matrix

| Mode | Commands / verbs |
|---|---|
| **Daemon required** (control-plane op) | everything not listed below |
| **Fully offline** (reads/writes `AGEZT_HOME` or local files directly) | `version`, `help`, `vault *` (creds store), `provider creds list/set/rm`, `provider setup`, `provider import`, `backup`, `backup inspect`, `restore` (incl. `--at` point-in-time), `journal import`, `journal verify --bundle`, `plan validate`, `plan visualize`, `plan cost` (governor pricing table in-process), `compare audit` (scans a repo root), `netguard test`, `webhook test`, `token create/validate` (agentgw secret from home), `plugin hash`, `plugin new` (scaffold), `plugin registry` (dir/URL, installs into `<home>/plugins`), `skill registry` (listing), `market validate/publish/keygen`, `rollback list/show/dry-run` and `rollback apply` of `file.snapshot` checkpoints, `acp config` |
| **Offline but networked** (direct HTTP, no daemon) | `provider check` (builds adapters via `plugins/providers/compat`, real tokens spent, NOT governed/journaled), `catalog sync --local` (models.dev), `peers *` (peer REST `/api/v1/*` from `AGEZT_PEERS`), `ha *` (Home Assistant REST), `transcribe`, `listen` (STT endpoint; `listen` runs `AGEZT_VOICE_RECORD_CMD`), `skill registry <url>` listing, `plugin registry <url>` |
| **Hybrid** (daemon if up, else local fallback) | `catalog sync` (daemon `CmdCatalogSync` + hot reload, else local write), `web password set/clear` (`CmdConfigSet` live, else vault write), `quickstart` (offline wizard; pins `AGEZT_PROVIDER/MODEL` via `CmdConfigSet` only if a daemon answers), `doctor` (base-dir + `memory/memory.json` checks/repair offline, the rest via daemon) |
| **Local pre-step + daemon** | `skill import` (verifies bundle content address offline before dialing), `skill registry --install`, `transcribe/listen --run` (→ `cmdRun`), `plan run --dry-run` (generate via daemon, validate/cost offline), `rollback apply` (config/skill/workflow checkpoints → `CmdConfigSet`/`CmdSkillRestore`/`CmdWorkflowRestore`), `skill export` (fetch via daemon, write files locally) |

---

## 3. Complete command table

Op names are `controlplane.Cmd*` constants with their wire string in parentheses where it adds information.
"(local)" = no daemon call for that verb. Files are package-main files unless prefixed.

### Getting started / daemon

| Command | Subcommands / flags | Files | Ops / endpoints |
|---|---|---|---|
| `quickstart` | interactive | quickstart.go (+ catalog_sync.go, provider_setup.go reuse) | (local catalog + vault); `CmdConfigSet` for `AGEZT_PROVIDER`/`AGEZT_MODEL` if daemon up |
| `doctor` | `[--json] [--strict] [--repair]` | doctor.go, doctor_daemon.go, doctor_mesh.go, doctor_mesh_loop.go, doctor_mesh_plugins.go, doctor_ops.go, doctor_ops_env.go, doctor_ops_state.go | `controlplane.ProbeExisting`, `CmdStatus`, `CmdJournalVerify`, `CmdBudget`, `CmdCatalogList`, `CmdProviderStats`, `CmdApprovalsStats`, `CmdWardenStats`, `CmdWebhookStats`, `CmdScheduleList`, `CmdStandingList`, `CmdAgentList`, `CmdReaperScan`, `CmdDiskStats`, `CmdNetguardLog`, `CmdRateLimitStats`, `CmdJournalStats`; repair: `CmdScheduleEnable`, `CmdStandingSetEnabled`, `CmdAgentCapabilities` (guardian quiet patch) |
| `status` | `[--json]` | status.go, status_helpers.go, status_mesh.go | `CmdStatus` (+ local `AGEZT_PEERS` mesh summary) |
| `version` | — | main.go | (local) |
| `help` | `[<command>]` | help.go, help_data.go | (local) |
| `shutdown` | `[--json]` | shutdown.go | `CmdShutdown` |

### Run & control

| Command | Subcommands / flags | Files | Ops |
|---|---|---|---|
| `run` | see §2 | main.go, main_run_modes.go, main_run_helpers.go | `CmdRun` (stream / dry-run call) |
| `halt` / `resume` | `[--reason R] [--json]` | haltresume/haltresume.go | `CmdHalt` / `CmdResume` |
| `runs` | `list [N]`, `show <corr>`, `last`, `stats`, `cancel <corr>`, `pause`, `resume`, `step`, `steer <corr> <msg>`, `intervene` | runs.go, runs_list.go, runs_show.go, runs_last.go, runs_last_arc.go, runs_stats.go, runs_intervene.go | `CmdRunsList`, `CmdJournalTail` (show), `CmdRunsStats`, `CmdCancelRun`, `CmdRunPause`, `CmdRunResume`, `CmdRunStep`, `CmdRunSteer`, `CmdRunIntervene` |
| `why` | `<event_id> [--json\|--payload]` | why.go | `CmdWhy` |
| `conductor` | `"<task>" [--thinker/--worker/--verifier M] [--max-rounds N] [--plan] [--json\|-q]` | conductor.go | `CmdConductorAsk` (exit 3 = unverified) |
| `research` | `"<q>" [--max-sources] [--max-sub-questions] [--max-verify-claims] [--no-verify] [--json\|-q]` | research.go | `CmdResearchAsk` |
| `approvals` | (pending list), `log [N] [--denied]`, `stats` | main_approvals_plan.go, approvals_log.go | `CmdApprovals`, `CmdApprovalsLog`, `CmdApprovalsStats` |
| `approve` / `deny` | `<id> [reason]` | main_approvals_plan.go (`cmdDecide`) | `CmdDecide` (grant / deny) |
| `whoami` | `[--tenant <id>] [--json]` | whoami/whoami.go | `CmdWhoami` |
| `overseer` | `status`, `agents`, `runs`, `get <slug>`, `cancel <corr>`, `halt`, `resume`, `pause/unpause <slug>`, `retire <slug> [reason]`, `revive`, `delete\|rm`, `impact`, `bulk {pause\|retire\|delete} <csv>` | overseer.go, overseer_lifecycle.go, overseer_bulk.go | `CmdStatus`, `CmdAgentList`, `CmdCancelRun`, `CmdHalt`, `CmdResume`, `CmdAgentSetEnabled`, `CmdAgentImpact`, `CmdAgentRetire`, `CmdAgentRevive`, `CmdAgentRemove` |

### Plans & automation

| Command | Subcommands | Files | Ops |
|---|---|---|---|
| `plan` | `<file.json>` (execute), `generate\|gen "<intent>"`, `run [--dry-run] "<intent>"`, `refine <file> --feedback`, `history\|runs\|ls [N]`, `stats`, `validate <file>` (local), `visualize\|viz <file> [--raw]` (local Mermaid), `cost <file> --model <id>` (local) | main_approvals_plan_run.go, plan_dryrun.go, plan_refine.go, plan_history.go, plan_validate.go, plan_visualize.go, plan_cost.go | `CmdPlan` (stream), `CmdPlanGenerate`, `CmdPlanRefine`, `CmdPlanHistory`, `CmdPlanStats`; local: `planner.ValidateJSON`, `planner.EstimateCost` + `governor` pricing |
| `schedule` | `add` (agent task / `--workflow` / `--system-task` / `--tool`; `--every`, `--at`, `--in`, `--once`, `--continuous`, `--between`, `--days`, `--tz`), `edit <id>`, `list\|ls`, `fires\|history`, `stats`, `test\|preview <id> [--count N]`, `rm\|remove`, `run\|trigger`, `pause`, `resume` | schedule.go, schedule_add.go, schedule_edit.go, schedule_ops.go, schedule_ops_fires.go, schedule_ops_format.go, schedule_ops_stats.go, schedule_test_cmd.go | `CmdScheduleAdd`, `CmdScheduleEdit`, `CmdScheduleList`, `CmdScheduleFires`, `CmdScheduleStats`, `CmdScheduleTest`, `CmdScheduleRemove` (`schedule_rm`), `CmdScheduleRun`, `CmdScheduleEnable`; parses via `kernel/cadence` |
| `standing` | `list`, `add`, `edit\|set`, `pause`, `resume`, `remove\|rm`, `why` | standing.go, standing_actions.go | `CmdStandingList`, `CmdStandingAdd`, `CmdStandingEdit`, `CmdStandingSetEnabled`, `CmdStandingRemove`, `CmdStandingWhy` |
| `workflow` | `list`, `show`, `save\|add\|import --file`, `run`, `draft "<desc>" [--save]`, `refine [--save]`, `runs`, `templates [--use T --name N]`, `enable`, `disable`, `remove\|rm` | workflow.go, workflow_run.go, workflow_run_list.go, workflow_run_templates.go | `CmdWorkflowList`, `CmdWorkflowShow`, `CmdWorkflowSave`, `CmdWorkflowRun`, `CmdWorkflowDraft`, `CmdWorkflowRefine`, `CmdWorkflowRuns`, `CmdWorkflowTemplates`, `CmdWorkflowSetEnabled`, `CmdWorkflowRemove` (+ rollback checkpoint via `CmdWorkflowShow` before every mutation) |
| `workboard` | `list\|ls`, `lanes`, `show\|get`, `create\|add`, `claim`, `heartbeat\|beat`, `comment`, `block`, `fail`, `unblock`, `complete\|done`, `prove`, `seat`, `archive`, `link`, `policy`, `depend`, `reclaim`, `sweep`, `dispatch`, `watch` | workboard.go, workboard_mutate.go, workboard_mutate_lifecycle.go, workboard_mutate_policy.go, workboard_mutate_relations.go, workboard_mutate_run.go, workboard_render.go | `CmdWorkboard{List,Lanes,Show,Create,Claim,Heartbeat,Comment,Block,Fail,Unblock,Complete,Prove,Seat,Archive,Link,Policy,Depend,Reclaim,Sweep,Dispatch,Watch}` (`watch` polls until `workboardWatchTerminal`) |
| `okr` | `list\|ls`, `show\|get`, `create\|add`, `kr\|keyresult`, `link`, `unlink`, `archive` | okr.go, okr_mutate.go, okr_helpers.go | `CmdOKR{List,Show,Create,KeyResult,Link,Unlink,Archive}` |
| `taste` | `list\|ls`, `add\|create`, `remove\|rm\|delete` | taste.go | `CmdTasteList`, `CmdTasteCreate`, `CmdTasteDelete` |
| `seats` | `[list]`, `add\|create <id>`, `remove\|rm\|delete <id>` | seat.go | `CmdSeatList`, `CmdSeatCreate`, `CmdSeatDelete` |
| `agent` | `list`, `add\|create <slug>`, `show`, `authority`, `impact`, `tombstone`, `graveyard`, `set\|edit`, `task\|tasks`, `wake`, `repair`, `repair-status`, `pause`, `resume`, `retire`, `revive`, `remove\|rm` | agent*.go (16 files) | `CmdAgentList`, `CmdAgentAdd`, `CmdAgentEdit`, `CmdAgentImpact`, `CmdAgentTombstone`, `CmdAgentGraveyard`, `CmdAgentTaskUpdate`, `CmdAgentWake`, `CmdAgentRepair`, `CmdAgentRepairStatus`, `CmdAgentSetEnabled`, `CmdAgentRetire`, `CmdAgentRevive`, `CmdAgentRemove`; `show` adds `CmdApprovalsLog`; `authority` = `CmdAgentList`+`CmdEdictShow` compared client-side |
| `toolforge` | `list`, `show`, `draft\|add\|create`, `edit\|set`, `test`, `promote`, `quarantine`, `remove\|rm` | toolforge.go, toolforge_cmds.go, toolforge_actions.go, toolforge_helpers.go | `CmdToolforge{List,Show,Draft,Edit,Test,Promote,Quarantine,Remove}` |
| `mcp` | `list`, `add\|register` (`--cmd EXE --arg A` or `--url URL --header`), `attach`, `detach`, `enable`, `disable`, `remove\|rm` | mcp.go, mcp_view.go, mcp_modify.go | `CmdMCPList`, `CmdMCPAdd`, `CmdMCPAttach`, `CmdMCPDetach`, `CmdMCPSetEnabled`, `CmdMCPRemove` |
| `market` | `list\|ls`, `search <q>`, `show\|get`, `install [--marketplace M] [--version V]`, `uninstall`, `sources`, `add <url>`, `remove\|rm`, `sync`, `validate <dir>` (local), `publish` (local), `keygen` (local) | market.go, market_browse.go, market_modify.go, market_ops.go, market_sources.go, market_publish.go | `CmdMarketList`, `CmdMarketShow`, `CmdMarketInstall`, `CmdMarketUninstall`, `CmdMarketSources`, `CmdMarketAddSource`, `CmdMarketRemoveSource`, `CmdMarketSync`; local `market.BuildPackFromDir`, Ed25519 keypair written as `<prefix>.key` (0600) / `.pub` |

### Providers & models

| Command | Subcommands | Files | Ops |
|---|---|---|---|
| `catalog` | `sync [url] [--local] [--json]`, `list`, `discover [url]` | main_approvals_plan_catalog.go, catalog_sync.go | `CmdCatalogSync` (else local `catalog` store write; `AGEZT_CATALOG_URL`), `CmdCatalogList`, `CmdCatalogDiscover` (via `cmdSimple`) |
| `provider` | `connect <id> --url --model [--env --key] [--default]` | provider_connect.go | `CmdProviderConnect`, `CmdProviderKeyAdd`, `CmdConfigSet`, `CmdProviderReload` |
| | `chatgpt login\|signin\|connect`, `import`, `logout\|disconnect`, `status` | provider_chatgpt.go | `CmdProviderOAuthStart` (+poll `CmdProviderOAuthStatus`), `CmdProviderOAuthImport`, `CmdProviderOAuthLogout`, `CmdProviderOAuthStatus` (daemon owns the 127.0.0.1:1455 redirect listener) |
| | `creds list\|ls`, `set <NAME> [value]`, `rm` | provider.go, provider_creds_ops.go | (local vault `creds.Store`; prints "run `agt provider reload`") |
| | `keys list\|add\|activate\|rm [--provider <id>] <ENV> …` | keys/keys.go, keys/keys_add.go, keys/keys_helpers.go | `CmdProviderKeyList`, `CmdProviderKeyAdd`, `CmdProviderKeyActivate`, `CmdProviderKeyRemove` |
| | `check [id\|--all] [--bench N] [--stream] [--caps] [--json]` | check.go, check_helpers.go, check_all.go, check_all_caps.go, check_all_stream.go, check_bench.go, check_json.go, check_render.go | (local: catalog + vault, `compat.Build`, real `Complete`/stream probe "Say 'pong'") |
| | `log [N] [--fallbacks]`, `stats`, `rejections` | provider_log.go | `CmdProviderLog`, `CmdProviderStats`, `CmdProviderRejections` |
| | `cost --model <id> [--input-tokens N] [--output-tokens N]` | provider_cost.go | `CmdCatalogList` (price lookup) |
| | `reload` | provider.go | `CmdProviderReload` |
| | `setup [provider-id]` | provider_setup.go | (local catalog + vault, prompts) |
| | `import [--from f] [--all] [-y]` | provider_import.go, provider_import_files.go | (local: env, `.env`, known CLI creds files → vault) |
| `budget` | (snapshot), `check [--task-type t]` (exit 3 = exhausted), `set <usd\|0\|off>` | budget.go, budget_check.go | `CmdBudget`, `CmdBudgetSet` |
| `tool` | `list`, `log`, `stats` | tool.go, tool_log.go, tool_stats.go | `CmdToolList`, `CmdToolLog`, `CmdToolStats` |
| `cache` | `[--since] [--tenant] [--json]` | cache.go | `CmdCacheStats` |
| `tenant` | `create\|add <id>`, `list\|ls`, `stats`, `token <id>`, `release\|close <id>`, `rm\|remove\|delete <id>` | tenant.go | `CmdTenant{Create,List,Stats,Token,Release,Remove}` |

### Memory & knowledge

| Command | Subcommands | Files | Ops |
|---|---|---|---|
| `memory` | `add <subject> <content> [--type --evidence --half-life --tag k=v --conf]`, `list\|ls`, `log`, `search <q> [N]`, `get`, `forget\|rm`, `promote`, `audit`, `clean [--execute]`, `consolidate`, `profile`, `prune`, `bulk-forget`, `find-related` | memory.go, memory_add.go, memory_list.go, memory_log.go, memory_ops.go, memory_ops_ai.go | `CmdMemory{Add,List,Log,Search,Get,Forget,Promote,Audit,Clean,Consolidate,Prune,BulkForget,FindRelated}`, `CmdProfileRebuild` (profile) |
| `world` | `add <name> [--kind --alias]`, `relate <from> <verb> <to>`, `resolve <phrase> [N]`, `neighbors\|neighbours`, `list\|ls`, `log`, `show\|get`, `forget`, `audit` | world.go, world_helpers.go, world_list.go, world_log.go, world_resolve.go, world_view.go, world_write.go | `CmdWorld{Add,Relate,Resolve,Neighbors,List,Log,Get,Forget}`; `audit` = `CmdWorldList` analysed client-side |
| `skill` | `list\|ls`, `show\|get`, `history\|log`, `promote`, `quarantine`, `archive`, `revert`, `share`, `reassign [--agent S]`, `diff <id> [<id2>]`, `export [--all]`, `import <bundle\|SKILL.md\|dir>`, `files <id>`, `cat <id> <path>`, `registry <dir\|url> [--install]`, `hygiene`, `workshop …` | skill.go, skill_transition.go, skill_share.go, skill_view.go, skill_diff.go, skill_export.go, skill_export_cli.go, skill_import.go, skill_md.go, skill_files.go, skill_registry.go, skill_registry_remote.go | `CmdSkill{List,Get,History,Promote,Quarantine,Archive,Revert,Share,Reassign,Import,Files,ReadFile,Hygiene}` |
| `skill workshop` | `list\|ls`, `inspect\|show\|get`, `scan <id>`, `diff`, `curate\|curator`, `apply\|promote`, `reject`, `quarantine`, `propose\|import`, `propose-create\|create`, `propose-update\|update` | skill_workshop.go, skill_workshop_apply.go, skill_workshop_apply_args.go, skill_workshop_apply_propose.go, skill_workshop_curate.go, skill_workshop_scan.go | `CmdSkillList`, `CmdSkillHistory`, `CmdSkillGet`, `CmdSkillPromote`, `CmdSkillArchive`, `CmdSkillQuarantine`, `CmdSkillHygiene`, `CmdSkillImport`; `scan` is a client-side heuristic (URLs, unpinned installs) over the fetched body |
| `reflect` | `run`, `show` | reflect.go | `CmdReflectRun`, `CmdReflectShow` |
| `state` | `list [<ns>]`, `get <ns> <key>` (exit 3 absent) | state.go | `CmdStateList`, `CmdStateGet` |
| `artifact` | `get <ref> [--out <file>]` | artifact.go | `CmdArtifactGet` |

### Journal & audit

| Command | Subcommands | Files | Ops |
|---|---|---|---|
| `journal` | `verify [--bundle f] [--scope task:<corr>]`, `tail [N]`, `grep <pat> [--kind --subject --actor --correlation]`, `head`, `export [--since] [--out]`, `import <bundle> [--home]` (local), `stats` | main_run_journal.go, journal_verify.go, journal_tail.go, journal_grep.go, journal_head.go, journal_export.go, journal_import.go, journal_stats.go | `CmdJournalVerify` (live), `CmdJournalTail`, `CmdJournalGrep`, `CmdJournalHead`, `CmdJournalExport` (`CallRaw`), `CmdJournalStats`; bundle verify + import run in-process (`kernel/journal.Restore`, `event` hashing) |
| `pulse` | (live tail `--subject --kind --correlation --since --until --text --json …`), `status`, `pause`, `resume`, `asks [{approve\|reject} <issue_key>]` | pulse.go, pulse_control.go | `CmdPulseSubscribe` (`StreamUntilCancel`), `CmdPulseStatus`, `CmdPulsePause`, `CmdPulseResume`, `CmdPulseAsks`, `CmdPulseAskResolve` |
| `changelog` | `[N] [--since] [--json]` | changelog.go | `CmdChangelog` |
| `edict` | `show`, `test <cap> [<input>]` (exit 3 deny), `overlay`, `compact`, `deny {list\|add\|rm}`, `level <cap> <level>`, `mode`, `log [--denied]`, `stats` (all accept `--tenant`) | edict.go, edict_show.go, edict_deny.go, edict_overlay.go, edict_log.go, edict_stats.go, edict_helpers.go | `CmdEdictShow`, `CmdEdictTest`, `CmdEdictOverlay`, `CmdEdictCompact`, `CmdEdictDenyList/Add/Remove`, `CmdEdictSetLevel`, `CmdEdictSetMode`, `CmdEdictLog`, `CmdEdictStats` |
| `warden` | `log [N] [--issues]` (filters `exec`/`downgrade`/`limit`), `stats` | warden.go | `CmdWardenLog`, `CmdWardenStats` |
| `exec-profile` (`execution-profile`) | `list\|ls`, `show <id>`, `check\|doctor` | execution_profile.go, execution_profile_cmds.go, execution_profile_helpers.go | `CmdExecutionProfiles`, `CmdExecutionProfileShow`, `CmdExecutionProfileCheck` |
| `redact` | `test <string>` | redact.go | `CmdRedactTest` (live redactor) |
| `compare` | `audit [--target openclaw\|hermes\|all] [--root <repo>]` | compare.go, compare_audit.go, compare_data.go, compare_helpers.go | (local repo scan) |
| `netguard` | `test <host\|ip>` (local), `log [N]` | netguard.go | `CmdNetguardLog`; `test` = `kernel/netguard` guard built from `AGEZT_HTTP_ALLOW_LOOPBACK/PRIVATE` |
| `ratelimit` | `log [N]`, `stats` | ratelimit.go | `CmdRateLimitLog`, `CmdRateLimitStats` |
| `webhook` | `test [<url>] [--subject] [--secret]` (local POST via `kernel/webhook`), `log [N] [--failed]`, `stats` | webhook.go, webhook_dispatch.go | `CmdWebhookLog`, `CmdWebhookStats` |

### Console, config & data

| Command | Subcommands | Files | Ops |
|---|---|---|---|
| `config` | `show`, `ls`, `get <ENV>`, `set <ENV> <value>`, `schema [register <file> \| unregister <id>]` | config.go, config_show.go, config_query.go, config_schema.go | `CmdConfig`, `CmdConfigValues`, `CmdConfigSet` (+ `config.setting` rollback checkpoint first), `CmdConfigSchema`, `CmdConfigSchemaRegister`, `CmdConfigSchemaUnregister` |
| `web` | `password set [<v>]`, `clear\|reset\|rm\|unset`, `status` | web.go | `CmdConfigSet` (`AGEZT_WEB_PASSWORD`) else local vault |
| `configcenter` | `set <k> <v> [--rating public\|internal\|restricted\|secret] [--description]`, `get`, `list`, `delete`, `rating`, `access-log`, `audit`, `health` | configcenter.go, configcenter_set.go, configcenter_list.go, configcenter_meta.go | `CmdConfigCenter{Set,Get,List,Delete,SetRating,AccessLog,Audit,Health}` (wire `configcenter.*`) |
| `token` | `create [--run-id --caps --max-rpm --burst --expiry]`, `validate <TOKEN>`, `help` | token.go | (local `agentgw.NewTokenManager(agentgw.ResolveTokenSecret(home))`) |
| `vault` | `status`, `encrypt`, `decrypt`, `rotate`, `migrate` | vault.go, vault_subcommands.go | (local `kernel/creds`; `AGEZT_VAULT_PASSPHRASE[_NEW]`) |
| `backup` | `[--home] [--out]`, `inspect <file>` | backup.go, backup_lib.go | (local tar.gz of `journal/` + `catalog/` + manifest) |
| `restore` | `<bundle> --home <fresh>`; `--at <seq\|RFC3339> --to <dir> [--home src]` | backup_restore.go, backup_restore_helpers.go | (local) |
| `rollback` | `list\|ls [--run]`, `show\|dry-run\|preview <cp>`, `apply <cp>` | rollback.go, rollback_catalog.go, rollback_apply.go, rollback_apply_checkpoints.go | local `<home>/rollback/checkpoints.json`; apply → `CmdConfigSet` / `CmdSkillRestore` / `CmdWorkflowRestore` or local file restore |
| `disk` | `[--json]` | disk.go | `CmdDiskStats` |

### Channels & integrations

| Command | Subcommands | Files | Ops / endpoints |
|---|---|---|---|
| `inbox` | `[N] [--channel KIND] [--json]` | inbox.go | `CmdInbox` |
| `send` | `--channel KIND --to ID <text>` | send.go | `CmdSend` |
| `channel` | `list` | channel.go | `CmdChannelList` |
| `ha` | `states [entity]`, `services`, `call <domain>.<service> [json]` | ha.go, ha_call.go, ha_helpers.go | direct HTTP to `AGEZT_HOMEASSISTANT_URL` (`/api/states`, `/api/states/<id>`, `/api/services`, `/api/services/<d>/<s>`) with `AGEZT_HOMEASSISTANT_TOKEN` |
| `transcribe` | `<file> [--model] [--run] [--json]` | transcribe.go | local `kernel/stt` client (`AGEZT_STT_API_URL/KEY/MODEL`, fallback `OPENAI_API_KEY`); `--run` → `cmdRun` |
| `listen` | `[--seconds N] [--model] [--run] [--json]` | listen.go | runs `AGEZT_VOICE_RECORD_CMD` (`{seconds}`,`{out}`), then `transcribeFile` |
| `peers` | `[list]`, `models [<name>]`, `route <model>`, `run <peer> <corr>`, `artifacts <peer> <corr>`, `artifact-get <peer> <id> <out>` | peers.go, peers_inspect.go, peers_remote.go, peers_fetch.go, peers_fetch_cmd.go | direct HTTP to each peer from `AGEZT_PEERS` (`/api/v1/health`, `/api/v1/models`, `/api/v1/runs/<corr>`, `/api/v1/artifacts`, `/api/v1/artifacts/<id>`) |
| `acp` | (stdio server `[--tenant]`), `agents [--json]`, `config --tenant <id>` | acp.go | `kernel/acp.New(controlPlaneRunner, stdin, stdout).Serve`; each prompt → `Stream(CmdRun)`; `CmdACPAgents` |
| `plugin` | `list`, `hash <path>` (local BLAKE3 for `AGEZT_PLUGIN_PINS`), `new <name>` (local scaffold), `registry <dir\|url> [--install <name>]` (local) | plugin.go, plugin_new.go, plugin_new_helpers.go, plugin_new_render.go, plugin_registry.go, plugin_registry_index.go, plugin_registry_helpers.go | `CmdPluginList`; registry install verifies `plugin.HashBytes` vs index BLAKE3 and writes `<home>/plugins/<file>` |

---

## 4. Subpackages in detail

### cmd/agt/router (router.go, doc.go)
- `type Command struct{Name, Aliases, Description, HelpLong, HelpHandler, Run}`; `type Registry struct{cmds map[string]*Command}`.
- `NewRegistry`, `(*Registry).Register` (name + every alias; later registration wins), `Lookup`, `All` (dedup by
  primary name, sorted), `Execute(name, args, stdout, stderr, unknownMsg) int`, `Suggest(name, maxSug)`
  (case-insensitive Levenshtein with early-out budget, tolerance 2).
- Concurrency: not safe for concurrent mutation; contract is "register in init(), read-only dispatch after".
- No dependencies; exists so dispatch logic is testable without compiling 233 command files.

### cmd/agt/dial (dial.go, doc.go)
- `New(stderr) *controlplane.Client`, `NewAtBase(base, stderr) *controlplane.Client` (see §2). Returns nil on
  failure after printing an actionable hint; callers `return 1`.
- Note: `doc.go` still describes a "3-line shim" in main; `main.go` says the shim was deleted — the doc comment is stale.

### cmd/agt/jsonout (jsonout.go, doc.go)
- `Write(w io.Writer, v any) int` — `json.Encoder` with 2-space indent; swallows encode errors; returns 0 so callers can
  `return jsonout.Write(stdout, res)`.

### cmd/agt/format (format.go, doc.go)
- `Bytes`, `Percent(spent, cap)` ("—" when cap≤0), `Time` (zero time → "never"), `Duration(ms)`, `Uptime(secs)`,
  `Number(any) int`, `Str(any) string`, `ParseDuration(s)` (Go duration or bare seconds), const `DiskWarnPct = 10.0`
  (consumed by `doctor_ops.go` as the free-space warn threshold; `diskCritPct = 3.0` is doctor-local).
- Pure: no I/O, no globals.

### cmd/agt/haltresume (haltresume.go, doc.go)
- `Halt`, `Resume` → private `run(action, args)`: flags `--reason/-r`, `--json`, `-h`; 10 s `Call(CmdHalt|CmdResume,
  {reason})`; prints `halted (reason: …)`. Halt cancels every in-flight run and refuses new ones; resume clears the
  flag only.

### cmd/agt/whoami (whoami.go, doc.go)
- `Run(args)`: `--tenant`, `--json`; `CmdWhoami` → prints "primary (admin token — full access)" or
  `tenant "<id>" (own token — tenant-scoped access)`. Pilot of the command-as-package refactor.

### cmd/agt/keys (keys.go, keys_add.go, keys_helpers.go, doc.go)
- `Run(args)` dispatch: `list|ls`, `add|set`, `activate|use`, `rm|remove|del|delete`.
- `addCmd` reads the value from argv or prompts on stdin (single line); `--active` makes it active (daemon reloads the
  provider). Helpers: `providerFlag`, `keyRequest(provider, env)` → `{"env", "provider"?}`, `targetLabel`,
  `flagSnippet`. `--provider <id>` scopes storage to one catalog provider; without it the legacy global env keyring is used.

### cmd/agt/providerlookup (lookup.go, doc.go)
- `ScopedVaultLookup(cat, vaultLookup)`: a bare env name answers from the vault only when exactly ONE catalog provider
  declares it (`cat.DuplicateCredentialEnvs()`); `provider:<id>:<env>` names (`catalog.IsProviderCredentialName`)
  always answer. `CredentialLookup` = `creds.ChainLookup(ScopedVaultLookup, os.Getenv)` (process env stays global).
- Used by `provider_lookup.go` (`catalogCredentialLookup`) for `provider check` and `quickstart`.

---

## 5. File-by-file table — package `main` (233 files)

### 5.1 Entry, dispatch, help
| File | What it does |
|---|---|
| `main.go` | `main`/`run` entry (version/help handling, uniform `-h` interception), `resolveRunIntent`, `cmdRun` (`agt run` flag parsing, streaming renderer, usage footer). |
| `cmd_register.go` | `init()` registering all 71 top-level commands in five groups; `approve`/`deny` are closures over `cmdDecide`. |
| `commands.go` | `Command` alias of `router.Command`, `CommandRegistry` test index, `registry` dispatcher, `Register`, `ExecuteCommand` (adds the "run `agt help`" trailer on exit 2). |
| `doc.go` | Package documentation for the registration/dispatch design. |
| `help.go` | `printHelp`, `helpHas`, `cmdHelp`, `suggestCommands`, `editDistance`. |
| `help_data.go` | `commandHelp`/`helpGroup` types and `helpGroups()` — the single source of truth for help text. |
| `main_run_modes.go` | `runJSONMode` (ndjson event stream + result/error envelope) and `runDryRunMode` for `agt run`. |
| `main_run_helpers.go` | `imageMediaType`, `loadImageDataURL` (local image → data URL), `parseUSDToMicrocents`, `toStringSlice`, `cmdSimple` (generic call-and-print-JSON helper, 30 s). |

### 5.2 Run control, runs, approvals, overseer
| File | What it does |
|---|---|
| `runs.go` | `cmdRuns` dispatcher + `cmdRunsSteerVerb` (pause/resume/step) and `cmdRunsSteer` (inject a steer message). |
| `runs_intervene.go` | `runs intervene` and `runs cancel` (`CmdRunIntervene`, `CmdCancelRun`). |
| `runs_list.go` | `runs list [N]` with `renderRunRow` and `renderRunsTree` (parent/child delegation tree). |
| `runs_show.go` | `runs show <corr>`: finds the run in `CmdRunsList`, pulls events, renders child outcomes (`childOutcome`). |
| `runs_last.go` | `runs last` + helpers (`failedByReasonStr`, `arcPreview`, `synthesizePlanSummary`, `fmtDuration`). |
| `runs_last_arc.go` | `renderTaskArc` — the human "task arc" replay renderer. |
| `runs_stats.go` | `runs stats` aggregate (counts, success rate, durations, spend) from `CmdRunsStats`. |
| `why.go` | `agt why <event_id>`: correlation-chain listing with `--payload`/`--json` (`renderPayload`, `renderJSONIndented`). |
| `conductor.go` | `agt conductor` Thinker/Worker/Verifier panel via `CmdConductorAsk`; `conductorExit` (0 verified / 3 not). |
| `research.go` | `agt research` deep-research harness via `CmdResearchAsk`; renders sources, claims, verification %. |
| `main_approvals_plan.go` | `cmdApprovals` (pending HITL list; routes `log`/`stats`) and `cmdDecide` (grant/deny). |
| `approvals_log.go` | `approvals log` (resolved audit timeline) and `approvals stats` (grant rate, denied-by-capability). |
| `overseer.go` | `agt overseer` dispatcher + status/agents/runs/halt/resume/cancel/pause-unpause (mirrors the agent-facing overseer tool). |
| `overseer_lifecycle.go` | Overseer `impact`/`retire`/`revive`/`get`/`delete`. |
| `overseer_bulk.go` | `overseer bulk {pause|retire|delete} <csv>` + `jsonOrString`. |
| `shutdown.go` | `agt shutdown` → `CmdShutdown` (same teardown path as SIGTERM). |

### 5.3 Plans
| File | What it does |
|---|---|
| `main_approvals_plan_run.go` | `cmdPlan` dispatcher, `cmdPlanExecuteFile` (stream `CmdPlan`), `cmdPlanGenerate`, `cmdPlanRun`, `runPlanJSON`, `formatTime` shim. |
| `plan_dryrun.go` | `runDryRunPreview` (pretty-print, validate, cost the generated plan offline) and `jsonPretty`. |
| `plan_refine.go` | `plan refine <file> --feedback` via `CmdPlanRefine`; prints the revised plan JSON. |
| `plan_history.go` | `plan history` and `plan stats` (plan runs live under `plan-…` correlations, invisible to `runs`). |
| `plan_validate.go` | Client-side `planner.ValidateJSON` over a plan file. |
| `plan_visualize.go` | Mermaid `graph TD` renderer (`renderPlanMermaid`, node shapes, label/ID escaping). |
| `plan_cost.go` | `plan cost` using `planner.EstimateCost` with the governor pricing table; exported `CostMicrocents`. |

### 5.4 Schedules, standing orders, workflows
| File | What it does |
|---|---|
| `schedule.go` | `cmdSchedule` dispatcher/help + `scheduleSystemTaskUsage`, `parseHHMM`, `parseWindow`, `nextWallclock`. |
| `schedule_add.go` | `schedule add`: agent-task / workflow / system-task / tool targets, every/at/in/once/continuous/between/days/tz → `CmdScheduleAdd`. |
| `schedule_edit.go` | `schedule edit <id>` in-place cadence/intent/model change → `CmdScheduleEdit`. |
| `schedule_ops.go` | `schedule list` and pause/resume (`cmdScheduleEnable`). |
| `schedule_ops_fires.go` | `schedule fires [N] [--id]` recent firings + outcomes. |
| `schedule_ops_format.go` | Text helpers `scheduleTargetStatusText`, `scheduleExecutorText`, `scheduleActionText` (uses `kernel/cadence`). |
| `schedule_ops_stats.go` | `schedule stats`, `schedule rm`, `schedule run` + `scheduleByID`. |
| `schedule_test_cmd.go` | `schedule test <id> [--count N]` next-fire-time preview (`CmdScheduleTest`, exit 3 if absent). NOT a Go test file despite the name. |
| `standing.go` | `cmdStanding` dispatcher + edit/why/list and renderers (`renderStandingLine`, `standingTargetStatus`, `initiativeMode`). |
| `standing_actions.go` | Standing order add / set-enabled / remove. |
| `workflow.go` | `cmdWorkflow` dispatcher + list/show/save (`workflowContractText`); checkpoint before save. |
| `workflow_run.go` | `workflow draft`/`refine` (LLM copilot, `--save`), `workflow run`, `savedState`. |
| `workflow_run_list.go` | `workflow runs`, enable/disable, remove (each checkpointed). |
| `workflow_run_templates.go` | `workflow templates [--use T --name N]` gallery + save. |

### 5.5 Workboard, OKR, taste, seats
| File | What it does |
|---|---|
| `workboard.go` | Dispatcher + `list`, `lanes`, `show`; routes unblock/complete/prove/archive via `cmdWorkboardActor`. |
| `workboard_mutate.go` | `workboard create` (title, criteria, assignee, priority, max-attempts, seat). |
| `workboard_mutate_lifecycle.go` | claim, heartbeat, comment, block, fail. |
| `workboard_mutate_policy.go` | policy, depend, reclaim. |
| `workboard_mutate_relations.go` | seat, generic actor verbs (`cmdWorkboardActor`), link. |
| `workboard_mutate_run.go` | sweep (stale claims), dispatch (to an agent), watch (poll until terminal). |
| `workboard_render.go` | Arg parsers, `workboardFlagValue`, `callWorkboard` (the one dial+call site), task/mutation/watch renderers, `mapAny`, `truthy`. |
| `okr.go` | OKR dispatcher + list/show. |
| `okr_mutate.go` | create, key-result, link/unlink (task → KR), archive. |
| `okr_helpers.go` | `okrFlagValue`, `okrIDArg`, `callOKR`, renderers. |
| `taste.go` | `agt taste` exemplars list/add/remove (`callTaste`, `tasteFlagValue`). |
| `seat.go` | `agt seats` list/add/remove execution seats. |

### 5.6 Agents (roster)
| File | What it does |
|---|---|
| `agent.go` | `cmdAgent` dispatcher, `agentUsage`, list-state labels. |
| `agent_list.go` | `agent list`. |
| `agent_show.go` | `agent show <slug>` full profile + recent approval summary (`agentApprovalSummary`). |
| `agent_crud.go` | `agent add` / `agent set` (→ `CmdAgentAdd` / `CmdAgentEdit`). |
| `agent_crud_action.go` | `agent wake`, `repair`, `repair-status` + async-action helper and payload builders. |
| `agent_crud_state.go` | pause/resume (`cmdAgentSetEnabled`). |
| `agent_crud_task.go` | `agent task` (per-agent task list/update) + payload/flag helpers and usage. |
| `agent_flags.go` | `parseAgentFlags` — the large flag parser shared by add/set (soul, model, budget, workdir, policy …). |
| `agent_render.go` | Impact summary printing, `str`/`intNumber` shims over `format`. |
| `agent_render_flags.go` | Policy/advanced flag appliers, `taskScope`, `parseConfigOverrides`, list helpers. |
| `agent_render_print.go` | Printers for policy, lifecycle, task summary. |
| `agent_authority.go` | `agent authority` (agent capabilities vs Edict levels) and `agent impact`. |
| `agent_authority_view.go` | `buildAgentAuthority`, `renderAgentAuthority`. |
| `agent_authority_helpers.go` | `anyToStringSlice`, `levelExceeds`, `levelRank`, `orDash`. |
| `agent_lifecycle.go` | tombstone, graveyard, retire, revive, remove. |
| `agent_lifecycle_helpers.go` | `agentRemoveResultSummary`, `buildAgentRemovePayload` (`--with-*` cascade flags), usage. |

### 5.7 Tools, toolforge, MCP, market, plugins
| File | What it does |
|---|---|
| `tool.go` | `agt tool` dispatcher + list (`toolRollbackLabel`). |
| `tool_log.go` | Per-invocation tool log viewer (`CmdToolLog`). |
| `tool_stats.go` | Per-tool calls/errors aggregate (`CmdToolStats`). |
| `toolforge.go` | Toolforge dispatcher + usage. |
| `toolforge_cmds.go` | list/show. |
| `toolforge_actions.go` | draft, edit, test, promote, quarantine, remove. |
| `toolforge_helpers.go` | `toolforgeFlags` + `parseToolforgeFlags`. |
| `mcp.go` | MCP dispatcher. |
| `mcp_view.go` | `mcpUsage`, `mcp list`. |
| `mcp_modify.go` | `mcp add` (stdio `--cmd/--arg` or remote `--url/--header`), attach/detach/remove (`cmdMCPRefAction`), enable/disable. |
| `market.go` | Market dispatcher/help. |
| `market_browse.go` | list/search/show. |
| `market_modify.go` | install/uninstall. |
| `market_ops.go` | `market sources` list + `market validate <dir>` (local `market.BuildPackFromDir`). |
| `market_sources.go` | add/remove source, sync. |
| `market_publish.go` | `market publish` (local pack build/sign) + `market keygen` (Ed25519 key files). |
| `plugin.go` | Plugin dispatcher, `plugin list` (`CmdPluginList`), `plugin hash` (`plugin.HashFile`). |
| `plugin_new.go` | `plugin new` scaffold writer (main.go, go.mod, README into a new dir). |
| `plugin_new_helpers.go` | `pluginNewUsage`, `sanitizeToolName`. |
| `plugin_new_render.go` | Template renderers for the scaffold (`renderPluginMain` emits a `plugins/sdk`-based main). |
| `plugin_registry.go` | Index types (`pluginIndex`, `indexPlugin`, `indexBinary`) + `cmdPluginRegistry`. |
| `plugin_registry_index.go` | `loadPluginIndex`, `listPluginRegistry`, `installPluginFromRegistry` (BLAKE3-verified, writes `<home>/plugins`). |
| `plugin_registry_helpers.go` | `selectBinary` (GOOS/GOARCH), `platformList`, `safeRegistryFilename`, `readRegistryFile`, `httpGetBounded`. |

### 5.8 Providers, catalog, budget, tenancy, onboarding
| File | What it does |
|---|---|
| `main_approvals_plan_catalog.go` | `cmdCatalog` dispatcher, `catalog list` (grouped), `catalog discover` (Ollama). |
| `catalog_sync.go` | `catalog sync` daemon-first, local fallback; `renderSyncResult`, `envOr`. |
| `provider.go` | `cmdProvider` 12-way dispatcher, `provider reload`, `provider creds` dispatcher. |
| `provider_creds_ops.go` | `openCredsStore` + vault list (grouped by catalog provider) / set (prompt when value omitted) / rm. |
| `provider_helpers.go` | `loadCatalogIfAny` (`<home>/catalog` store), `plural`. |
| `provider_lookup.go` | `catalogCredentialLookup` thin wrapper over `providerlookup.CredentialLookup`. |
| `provider_connect.go` | `provider connect` (custom-layer registration + optional key + optional default, live). |
| `provider_chatgpt.go` | `provider chatgpt` Codex-OAuth login (prints URL, polls), import, logout, status. |
| `provider_setup.go` | Guided offline key entry (`listProvidersNeedingKeys`, `promptProviderCredential`, `suggestProviders`, `firstModelID`). |
| `provider_import.go` | `provider import`: discover keys in env/.env/known CLI files, confirm, copy into vault. |
| `provider_import_files.go` | `knownCredFiles`, `parseDotEnvFile`, `parseJSONCredFile`. |
| `provider_log.go` | provider log (routing/fallbacks), stats, rejections (capability gating). |
| `provider_cost.go` | Model price lookup + hypothetical token cost (`estimateCostMicrocents`, `findModelCost`, `commaInt`). |
| `check.go` | `checkFlags`, `parseCheckFlags`, `cmdProviderCheck` mode router (caps/all/single/stream/bench). |
| `check_helpers.go` | `runCheckSingle`, `runCheckCaps`, `emitCapsHuman`. |
| `check_all.go` | `runCheckAll` over credentialed providers, `runProbe` (builds via `compat.Build`, `Complete` "pong" probe). |
| `check_all_caps.go` | Network-free capability matrix `runCheckCapsAll`, `renderCapsTable`. |
| `check_all_stream.go` | Streaming probe (`agent.StreamingProvider`) `runStreamProbe`. |
| `check_bench.go` | `runBench` N-probe loop, `computeLatencyStats` (p50/p95), `toCheckRow`. |
| `check_json.go` | JSON output + `autoPickFromCatalog`, `computeCostMicrocents`, `formatMicrocentsUSD`, `truncate`. |
| `check_render.go` | `checkRow` human renderers and aligned tables. |
| `budget.go` | `agt budget` snapshot, `budget set`, money helpers `mcFromAny`, `fmtUSD`, `usdToMicrocents`, `pct`. |
| `budget_check.go` | `budget check` preflight (`effectiveHeadroom`; exit 3 when exhausted). |
| `cache.go` | Prompt-cache savings view (`CmdCacheStats`). |
| `tenant.go` | Tenant create/list/stats/token/release/rm (`cmdTenantByID`). |
| `quickstart.go` | First-run wizard (catalog sync local → choose provider → key → start command), `persistProviderModel`, `keyedConfigured`, injectable `stdin`. |

### 5.9 Memory, world, skills, reflection, state, artifacts
| File | What it does |
|---|---|
| `memory.go` | `cmdMemory` dispatcher. |
| `memory_add.go` | `memory add` with type/evidence/half-life/tags/confidence (`validMemoryEvidence`). |
| `memory_list.go` | list, search, get. |
| `memory_log.go` | Memory operation timeline (`CmdMemoryLog`). |
| `memory_ops.go` | forget, promote (private → shared brain), audit, clean + `num`, `lenSlice`, `renderRecordLine`. |
| `memory_ops_ai.go` | consolidate, profile (`CmdProfileRebuild`), prune, bulk-forget, find-related. |
| `world.go` | `cmdWorld` dispatcher. |
| `world_helpers.go` | `worldCall` dial+call helper. |
| `world_list.go` | list, show. |
| `world_log.go` | World-model operation timeline. |
| `world_resolve.go` | resolve (phrase → entity), neighbors. |
| `world_view.go` | `renderEntityLine`, `world audit`. |
| `world_write.go` | forget, add, relate. |
| `skill.go` | `cmdSkill` dispatcher + list/show/history. |
| `skill_transition.go` | promote/quarantine/archive/revert. |
| `skill_share.go` | share / reassign ownership (`cmdSkillReassign`). |
| `skill_view.go` | Renderers (`renderSkillLine`, `shadowProgress`, `renderSkillEventDetail`, `shortID`) + `skill hygiene`. |
| `skill_diff.go` | LCS `lineDiff`, `fetchSkill`, `skill diff` (vs lineage parent or second id). |
| `skill_export.go` | `skillBundle` types, `buildSkillBundle`, `verifySkillBundle` (content address), `safeSkillFilename`. |
| `skill_export_cli.go` | `skill export [--all]` (writes `*.skill.json` + registry `index.json`, 0600). |
| `skill_import.go` | `skill import` (verify offline, then `CmdSkillImport` as DRAFT), `importSkillBundleBytes`. |
| `skill_md.go` | SKILL.md / agentskills.io directory bundle import (`isSkillMarkdown`, `importSkillDir`, `importSkillMarkdownBytes`). |
| `skill_files.go` | `skill files <id>` / `skill cat <id> <path>` bundle resources. |
| `skill_registry.go` | Local registry scan/list/install (`scanSkillRegistry`, `installFromRegistry`, `cmdSkillRegistry`). |
| `skill_registry_remote.go` | HTTP(S) registry via `index.json` (`fetchRegistryFile` bounded + path-validated, `remoteRegistry`, `remoteInstall`). |
| `skill_workshop.go` | Workshop dispatcher/usage + list, inspect. |
| `skill_workshop_apply.go` | apply/reject/quarantine/propose-create/propose-update + transition callers (checkpointed). |
| `skill_workshop_apply_args.go` | `parseWorkshopIDJSON`, `parseWorkshopReasonArgs`, `workshopFetchSkill`, `workshopProposals`, `workshopCanReject`. |
| `skill_workshop_apply_propose.go` | Proposal import path (`callSkillWorkshopImport…`, flags, body file reader). |
| `skill_workshop_curate.go` | `workshop scan` and `workshop curate` (hygiene → quarantine with checkpoints). |
| `skill_workshop_scan.go` | Client-side skill risk scan (`workshopScanSkill`, URL pattern, `unpinnedInstall`, severity) + renderers. |
| `reflect.go` | `reflect run` / `reflect show` (`renderReport`). |
| `state.go` | `state list` / `state get` (exit 3 absent). |
| `artifact.go` | `artifact get <ref> [--out]` raw bytes of an offloaded tool result. |

### 5.10 Journal, pulse, policy and audit surfaces
| File | What it does |
|---|---|
| `main_run_journal.go` | `cmdJournal` dispatcher (verify/tail/grep/head/export/import/stats). |
| `journal_verify.go` | Live verify (`cmdSimple(CmdJournalVerify)`) or offline bundle verify (`verifyBundleEvents`, scoped verify, `checkBundleCompleteness`, `shortHash`). |
| `journal_tail.go` | Last N events snapshot. |
| `journal_grep.go` | Server-side filtered search + help. |
| `journal_head.go` | Current head seq + chain-tail hash (checkpoint for `pulse --since`). |
| `journal_export.go` | Re-verifiable bundle export (`CallRaw`), `--scope task:<corr>` via `scopeCorrelation`. |
| `journal_import.go` | Offline restore into an EMPTY journal dir (`journal.Restore`, rejects non-genesis bundles), then re-opens to verify. |
| `journal_stats.go` | Journal size/shape per event kind. |
| `pulse.go` | Live bus tail (`StreamUntilCancel(CmdPulseSubscribe)`; subject/kind/correlation/since/until/replay-rate/text filters) + human/JSON renderers. |
| `pulse_control.go` | `pulse status/pause/resume` and `pulse asks [approve|reject]` (M1001 ask-mode bridge). |
| `changelog.go` | System timeline folded from the journal. |
| `edict.go` | `cmdEdict` dispatcher + `edict mode` / `edict level` setters. |
| `edict_show.go` | `edict show` (loaded policies) + `edict test` dry-run decision. |
| `edict_deny.go` | Hard-deny list/add/rm. |
| `edict_overlay.go` | `edict overlay` (net runtime overrides) + `edict compact` (snapshot durable overlay). |
| `edict_log.go` | policy.decision audit log. |
| `edict_stats.go` | Policy decision aggregates. |
| `edict_helpers.go` | `extractTenantFlag`, `withTenant`, `joinCaps`. |
| `warden.go` | Sandbox execution audit log + stats. |
| `execution_profile.go` | `exec-profile` dispatcher. |
| `execution_profile_cmds.go` | list/show/check of execution profiles (requested vs effective isolation). |
| `execution_profile_helpers.go` | `callExecProfile`, `tenantArg`, `dashJoin`. |
| `redact.go` | `redact test` against the live redactor. |
| `compare.go` | `compare audit` dispatcher + types (`compareCapability`, `compareEvidence`, `compareAuditRow`). |
| `compare_data.go` | `compareCapabilities()` static OpenClaw/Hermes parity list. |
| `compare_audit.go` | `buildCompareAudit` (checks evidence paths exist in repo) + `renderCompareAudit`. |
| `compare_helpers.go` | Target validation, `resolveCompareRoot`, `isAgeztRepoRoot`, evidence counters. |
| `netguard.go` | `netguard log` (daemon) + `netguard test` (local DNS resolve + `guardFromEnv` + `classifyIPs`). |
| `ratelimit.go` | rate.limited log + stats. |
| `webhook.go` | Webhook dispatcher + delivery log + stats. |
| `webhook_dispatch.go` | `webhook test` — local signed POST of a synthetic `webhook.test` event through `kernel/webhook` + netguard. |

### 5.11 Config, console, credentials, data safety
| File | What it does |
|---|---|
| `config.go` | `cmdConfig` dispatcher, `sortedKeys`, `config set` (checkpoint then `CmdConfigSet`; prints live/restart state). |
| `config_show.go` | `config show` snapshot + `renderRoutingTable` + `configValues`. |
| `config_query.go` | `config ls` / `config get <ENV>` (`CmdConfigValues`). |
| `config_schema.go` | Config-schema list/register/unregister. |
| `web.go` | `web password set/clear/status` (prompt + confirm; daemon `CmdConfigSet` else vault). |
| `configcenter.go` | Config Center dispatcher, help, get, delete. |
| `configcenter_set.go` | `configcenter set` with rating validation (`kernel/configcenter`). |
| `configcenter_list.go` | `configcenter list [--rating]`. |
| `configcenter_meta.go` | rating, access-log, audit, health. |
| `token.go` | JWT capability tokens (`agentgw.TokenClaims`, `parseCaps` allow-list, `getTokenSecret`). |
| `vault.go` | `cmdVault` dispatcher, help, KDF diagnostics (`creds.InspectVault`). |
| `vault_subcommands.go` | migrate (PBKDF2 upgrade), status, encrypt, rotate, decrypt — all direct `kernel/creds`. |
| `backup.go` | `agt backup` (verify journal first, write tar.gz) + `backup inspect`; `backupManifest`, `backupIncludeDirs = {"journal","catalog"}`. |
| `backup_lib.go` | `inspectBackup`, `createBackup`, `writeTarFile`. |
| `backup_restore.go` | `agt restore` (traversal-safe extraction into a fresh home, `verifyHomeJournal`, `resolveHome`). |
| `backup_restore_helpers.go` | `isAllowedBackupPath`, `parseAtSpec`, `pointInTimeRestore` (journal prefix replay into `--to`). |
| `rollback.go` | Rollback constants/types, dispatcher, list, show. |
| `rollback_catalog.go` | Checkpoint catalog I/O (`<home>/rollback/checkpoints.json`, tmp+rename), arg parsing, renderers. |
| `rollback_apply.go` | `rollback apply` → per-kind restore; `applyFileSnapshotCheckpoint` refuses symlinks/dirs. |
| `rollback_apply_checkpoints.go` | Savers for `skill.status`, `workflow.snapshot`, `config.setting` checkpoints (fetch "before" state via daemon). |
| `disk.go` | Journal size + free space (`CmdDiskStats`). |

### 5.12 Channels, integrations, status and doctor
| File | What it does |
|---|---|
| `inbox.go` | Unified inbox by correlation. |
| `send.go` | One-off outbound channel message. |
| `channel.go` | Channel list with live/configured/roundtrip readiness (`channelProbeText`). |
| `ha.go` | Home Assistant dispatcher, states, services, usage. |
| `ha_call.go` | `haCall` (service call) + `haRequest` (bearer-token HTTP). |
| `ha_helpers.go` | `haCheck` (env present), `printBodyJSON`. |
| `transcribe.go` | `sttClientFromEnv`, `cmdTranscribe`, shared `transcribeFile` (optional `--run`). |
| `listen.go` | Mic capture via `AGEZT_VOICE_RECORD_CMD` (`substituteRecord`, `execRecord` with injectable `recordFunc`). |
| `peers.go` | `agt peers` dispatcher + health list from `AGEZT_PEERS` (tokens never printed). |
| `peers_inspect.go` | `checkPeer`, `peersModels`, `fetchPeerModels`. |
| `peers_remote.go` | `peersRoute` (which peer serves a model), `peersRun` (fetch a remote run). |
| `peers_fetch.go` | HTTP primitives `fetchPeerRun`, `fetchPeerArtifacts`, `fetchPeerArtifactBytes` (bounded). |
| `peers_fetch_cmd.go` | CLI wrappers for `artifacts` and `artifact-get`. |
| `acp.go` | ACP stdio server (`controlPlaneRunner.Prompt` → streamed `run`, `llm.token` → chunks), `acp config`, `acp agents`. |
| `status.go` | `agt status` (version skew, uptime, runs, tools, journal head, fallbacks, channels, aws creds, mesh, schedules, tenants, approvals, delegation caps). |
| `status_helpers.go` | `intOfStatus`, `fmtUptime`. |
| `status_mesh.go` | `meshSummary`, `scheduleStatusLine`. |
| `doctor.go` | `cmdDoctor`, check types/helpers, `runDoctorChecks` orchestrator, text/JSON renderers, exit code (`--strict`). |
| `doctor_daemon.go` | Daemon-backed checks: sandbox, provider, catalog, approvals, webhooks, agent health (reaper), guardian noise (+repair), schedules (+repair), standing (+repair). |
| `doctor_mesh.go` | `checkMemoryStoreFile` (offline, `--repair` rewrites `memory/memory.json`), mesh auth, hop limit. |
| `doctor_mesh_loop.go` | Mesh loop refusals, provider fallbacks, tenant peers. |
| `doctor_mesh_plugins.go` | Plugin pin/config check, mesh check. |
| `doctor_ops.go` | netguard, ratelimit, disk checks + `humanBytes`, thresholds. |
| `doctor_ops_env.go` | base dir, version skew, tools, halt. |
| `doctor_ops_state.go` | exposure, budget, journal, credentials, channels, model readiness. |

### Test files (95 `*_test.go`, grouped)
- `coverage_server_test.go`: `startCoverageServer` boots a real `runtime.Kernel` (mock provider) + `controlplane.Server`
  on a temp `AGEZT_HOME` so commands are exercised end-to-end over the real wire. `coverage_*_test.go` use it broadly.
- `help_test.go`, `coverage_help_test.go`, `commands_test_helpers_test.go` (`AllCommands`, test-only `lookup`): registry/help sync, uniform `-h`, unknown-command suggestion.
- `json_flags_test.go`, `tenant_flag_test.go`, `configcenter_flags_test.go`: flag-surface pins (e.g. `--json` documented, unknown flags rejected).
- Per-family tests (`runs_*`, `schedule_fires`, `skill_*`, `doctor_*`, `peers_*`, `vault_*`, `backup*`, `restore_pit`, `rollback`, `plan_*`, `provider_*`, …) and `evidence_proof_test.go`, `edict_test_check_test.go`.
- Subpackages each have a single `*_test.go`.

---

## 6. Persistence the CLI writes directly (outside the daemon)

| Path (under `AGEZT_HOME` unless noted) | Writer | Format |
|---|---|---|
| `creds.json` / encrypted vault (`creds.Store`) | `provider creds set/rm`, `provider setup`, `provider import`, `quickstart`, `web password` (offline path), `vault encrypt/decrypt/rotate/migrate` | `kernel/creds` format (encryption per `AGEZT_VAULT_PASSPHRASE` / machine-bound policy) |
| `catalog/` store | `catalog sync --local` / daemon-down fallback, `quickstart` | `catalog.NewStore(base+"/catalog")` |
| `journal/*.jsonl` | `journal import`, `restore`, `restore --at --to` | journal segments via `kernel/journal` |
| `memory/memory.json` | `doctor --repair` (recreate/reset to `{}`) | JSON object |
| `rollback/checkpoints.json` | config/skill-workshop/workflow mutating verbs (pre-mutation checkpoint), `rollback apply` | `{version:1, checkpoints:[{id,kind,action,run_id,before…}]}`, tmp + rename, 0600 |
| `plugins/<file>` | `plugin registry --install` | binary, BLAKE3-verified |
| arbitrary user paths | `backup --out` (tar.gz, default `agezt-backup.tar.gz`), `journal export --out`, `skill export` (`*.skill.json`, `index.json`), `plugin new <dir>`, `market keygen` (`.key` 0600 / `.pub`), `artifact get --out`, `peers artifact-get` | various |

Runtime files read (never written): `runtime/control.addr`, `runtime/control.token`; agentgw token secret via
`agentgw.ResolveTokenSecret(home)`.

## 7. Environment variables read by cmd/agt

`AGEZT_HOME` (via `paths.BaseDir`), `AGEZT_TOKEN` (client token override, in `controlplane.NewClient`),
`AGEZT_SHOW_REASONING`, `AGEZT_PROVIDER`, `AGEZT_MODEL`, `AGEZT_CATALOG_URL`, `AGEZT_PEERS`, `AGEZT_TENANT_PEERS`,
`AGEZT_MESH_MAX_HOPS` (via `meshctx`), `AGEZT_PLUGINS`, `AGEZT_PLUGIN_PINS`, `AGEZT_PLUGIN_TOOLS`, `AGEZT_WEBHOOKS`,
`AGEZT_WEBHOOK_ALLOW_PRIVATE`, `AGEZT_WEBHOOK_ALLOW_LOOPBACK`, `AGEZT_HTTP_ALLOW_PRIVATE`, `AGEZT_HTTP_ALLOW_LOOPBACK`,
`AGEZT_APPROVAL_MODE`, `AGEZT_WEB_PASSWORD` (name written via config_set/vault), `AGEZT_STT_API_URL`,
`AGEZT_STT_API_KEY`, `AGEZT_STT_MODEL`, `OPENAI_API_KEY` (STT fallback), `AGEZT_VOICE_RECORD_CMD`,
`AGEZT_HOMEASSISTANT_URL`, `AGEZT_HOMEASSISTANT_TOKEN`, `AGEZT_VAULT_PASSPHRASE`, `AGEZT_VAULT_PASSPHRASE_NEW`
(via `creds`). Most of these are read by `doctor`/`status` to mirror daemon configuration, not to configure the CLI.

Events: the CLI emits no bus/journal events itself; it consumes `event.Event` frames from `run`/`plan` streams and
`pulse_subscribe` (`llm.token`, `llm.reasoning`, all others summarised).

## 8. Extension points

- **New top-level command**: implement `func cmdX(args []string, stdout, stderr io.Writer) int` in a new file,
  `Register(&Command{Name: "x", Run: cmdX})` in the right `register*Commands` group of `cmd_register.go`, AND add a
  `commandHelp{"x", summary, detail}` to the right group in `help_data.go` — `help_test.go` fails otherwise
  (both directions). `-h` then works automatically.
- **New subcommand**: add a `case` in the family dispatcher and a usage line in its local `-h` text and in
  `help_data.go` detail.
- **New daemon op**: add the `Cmd*` constant + handler in `kernel/controlplane` (see 03), then call it through
  `dialpkg.New(stderr)` + `c.Call(ctx, controlplane.CmdX, args)`; render with `jsonout.Write` for `--json`.
- **Command-as-package**: follow `whoami/` / `haltresume/` / `keys/`: depend only on `dial`, `jsonout`, `format`,
  `brand`, `controlplane`; register the exported `Run` from `cmd_register.go` (or the family dispatcher).
- **Reversible mutation**: call a `save*RollbackCheckpoint` helper (rollback_apply_checkpoints.go) before the
  mutating op and teach `applyRollbackCheckpoint` the new kind.

## 9. Gotchas / invariants

- **`-h` must never execute** (M936): `run()` intercepts `agt <cmd> -h` only when `<cmd>` is in the help table and
  `-h` is the FIRST argument; `agt run hello -h` still sends "hello -h" as the intent.
- **Unknown flags are errors**, never positionals — deliberate, pinned by tests (e.g. `TestCmdApprovals_RejectsUnknownFlag`).
- **`dial.NewAtBase` distinguishes dead vs rejecting daemon**: only transport errors yield the "stale socket" hint;
  auth errors surface verbatim. Commands with offline fallbacks build the client themselves to avoid printing hints.
- **`provider check` is ungoverned**: it constructs adapters in-process from catalog + vault and spends real tokens
  outside the governor, Edict, budget and journal. Credentials resolve via `providerlookup` (scoped vault, then env).
- **Vault writes from the CLI are not live**: `provider creds set` prints "run `agt provider reload`"; `provider keys`
  goes through the daemon instead (live reload). `web password` uses the daemon when reachable precisely to be live.
- **Backup is intentionally partial**: only `journal/` and `catalog/` (secrets excluded by construction); memory,
  state, schedules etc. are expected to rebuild from the journal (projection). Run backup/restore/import with the
  daemon stopped; `journal import` refuses a non-empty journal or a non-genesis bundle.
- **Rollback catalog is shared and unlocked**: `rollback/checkpoints.json` has three independent implementations
  (`cmd/agt/rollback*.go`, `kernel/webui/rollback.go`, `plugins/tools/file/checkpoint.go`) doing load-modify-write
  with tmp+rename but no cross-process lock — concurrent CLI/daemon writes can drop a checkpoint.
- **`schedule_test_cmd.go` is production code** (does not end in `_test.go`); naive `grep -v _test` file filters miss it.
- **Overseer verbs have no client deadline** (`context.TODO()` in overseer*.go); every other family uses `WithTimeout`.
- **Registered-but-unreachable bodies**: `version` and `help` registrations are shadowed by `run()`; `router.Command`
  fields `HelpLong`/`HelpHandler` are unused.
- **Stale comments to not trust**: `cmd_register.go` claims to be blank-imported (it is same-package `init`);
  `dial/doc.go` describes a main-package shim that no longer exists; in `edict.go` the doc comment above
  `cmdEdictMode` describes `edict log`, and `cmdEdict`'s says "only subcommand today is show" (there are 9); in
  `webhook.go` the comment above `cmdWebhookLog` describes `webhook test`; `format.DiskWarnPct`'s comment says
  "fill percentage" but doctor uses it as a free-space threshold.
- **Exit code 3 is semantic** (deny / absent / exhausted / unverified) and scripts depend on it.
- **Money units**: microcents everywhere on the wire ($1 = 1e9 µ¢, `usdToMicrocents`/`fmtUSD`); the CLI converts
  user-facing dollars at the edge (`--max-cost`, `budget set`, cost filters).
- **Two typo engines**: `agt help <typo>` (help table, substring + distance) vs `agt <typo>` (router, distance only, max 3).
