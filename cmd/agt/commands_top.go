// SPDX-License-Identifier: MIT

package main

// Provenance: the CLI's top-level command vocabulary. Every top-level command
// is declared once, in one table, and the registry and the help overview are
// derived from it.
//
// The table used to be written twice: cmd_register.go declared the commands
// (name, aliases, runner) and help_data.go declared the same names again
// with their summaries and usage lines. Two tests kept the copies in step —
// which is a guard, not a single source — and the root dispatcher switched on
// args[0] a third time to answer "version" and "help" before the registry
// ever saw them.

import (
	"io"

	"github.com/agezt/agezt/cmd/agt/haltresume"
	"github.com/agezt/agezt/cmd/agt/whoami"
)

// topRunner runs one top-level command. args are the arguments after the
// command name.
type topRunner func(args []string, stdout, stderr io.Writer) int

// topCommand is one top-level command of the CLI.
type topCommand struct {
	// name is the canonical name (agt run).
	name string
	// aliases are the other names the command answers to.
	aliases []string
	// group is the help overview's section the command is listed under.
	group string
	// summary is the one-line overview.
	summary string
	// detail is the full usage block "agt help <command>" prints. A line
	// that names a domain's sub-commands is derived from that domain's own
	// table, not written out here.
	detail []string
	// run executes the command.
	run topRunner
}

// cliTopCommands is the CLI's top-level vocabulary.
//
// It is filled in init() rather than in a var initializer: the table names
// cmdHelp as one of its runners, cmdHelp reads helpGroups(), and helpGroups()
// reads this table — a var initializer would be an initialization cycle.
var cliTopCommands []topCommand

// init fills the domain tables, then the vocabulary table, then registers
// every command in it. The domain tables come first: the usage lines that
// name a domain's sub-commands are derived from them.
func init() {
	fillDomains()
	cliTopCommands = []topCommand{
		// Getting started
		{
			name:    "quickstart",
			group:   "Getting started",
			summary: "interactive first-run: sync catalog, add a key, print the start command",
			detail: []string{
				"quickstart",
			},
			run: cmdQuickstart,
		},
		{
			name:    "doctor",
			group:   "Getting started",
			summary: "preflight checklist (base dir, memory store, daemon, journal, tools); exit 1 = a check failed",
			detail: []string{
				"doctor [--json] [--strict] [--repair]",
				"  --repair  repair startup-blocking local store files and pause noisy schedules",
			},
			run: cmdDoctor,
		},
		{
			name:    "status",
			group:   "Getting started",
			summary: "daemon health overview (version skew, uptime, runs)",
			detail: []string{
				"status [--json]",
			},
			run: cmdStatus,
		},
		{
			name:    "version",
			aliases: []string{"-v", "--version"},
			group:   "Getting started",
			summary: "show client version",
			detail: []string{
				"version",
			},
			run: cmdVersion,
		},
		{
			name:    "help",
			aliases: []string{"-h", "--help"},
			group:   "Getting started",
			summary: "this overview, or one command's full usage",
			detail: []string{
				"help [<command>]",
			},
			run: cmdHelp,
		},
		// Run & control
		{
			name:    "run",
			group:   "Run & control",
			summary: "run an intent through the governed agent loop",
			detail: []string{
				`run "<intent>" | run - | run --file <path>     intent from arg, stdin, or a file`,
				"  [--json|-q]                -q/--quiet = only the answer; --json = ndjson event stream",
				"  [--model <id>] [--system <prompt>] [--timeout <dur>] [--tenant <id>]   per-run overrides",
				"  [--exec-profile <local|warden|docker|ssh|k8s|modal|daytona|remote-agezt>]   request local/container/remote execution",
				"  [--peer <name>]              with remote-agezt, pin delegation to a configured peer",
				"  [--tools <csv>|--no-tools] [--dry-run] [--max-cost <usd>]   restrict tools / preview / cap spend",
				"  [--assure[=<n>]]           run, verify it's actually done, retry the gap (default 3 attempts)",
			},
			run: cmdRun,
		},
		{
			name:    "halt",
			group:   "Run & control",
			summary: "freeze all in-flight runs (reason is journaled)",
			detail: []string{
				`halt [--reason "..."] [--json]`,
			},
			run: haltresume.Halt,
		},
		{
			name:    "resume",
			group:   "Run & control",
			summary: "clear the halt flag (reason is journaled)",
			detail: []string{
				`resume [--reason "..."] [--json]`,
			},
			run: haltresume.Resume,
		},
		{
			name:    "runs",
			group:   "Run & control",
			summary: "list past runs / replay one as a task arc",
			detail: []string{
				"runs list [N] [--json]        last N agent runs (task-level summary)",
				"runs show <correlation> [--json]",
				"runs last [--json]            the most-recent run",
			},
			run: cmdRuns,
		},
		{
			name:    "why",
			group:   "Run & control",
			summary: "every event sharing one event's correlation (the run's story)",
			detail: []string{
				"why <event_id> [--json|--payload]",
			},
			run: cmdWhy,
		},
		{
			name:    "conductor",
			group:   "Run & control",
			summary: "run Thinker/Worker/Verifier roles on (usually) different models to solve & verify a hard task",
			detail: []string{
				`conductor "<task>"            run with auto-filled roles (one per keyed provider)`,
				"  [--thinker <model>] [--worker <model>] [--verifier <model>]   role→model overrides (id or @chain)",
				"  [--max-rounds <n>]          worker/verifier retry cap (default 2)",
				"  [--plan]                    tailor per-role instructions first",
				"  [--json|-q]                 full transcript as JSON / answer-only",
				"  exit: 0 = verified, 3 = not verified, 1 = error",
			},
			run: cmdConductor,
		},
		{
			name:    "research",
			group:   "Run & control",
			summary: "deep-research: decompose, gather web sources, synthesize a cited answer, verify each claim",
			detail: []string{
				`research "<question>"         gather + synthesize + adversarially verify`,
				"  [--max-sources <n>]         cap sources gathered (default 8, max 20)",
				"  [--max-sub-questions <n>]   cap sub-questions explored (default 3, max 8)",
				"  [--max-verify-claims <n>]   cap claims verified (default 6, max 12)",
				"  [--no-verify]               skip the adversarial claim check",
				"  [--json|-q]                 full report (sources + claims) as JSON / answer-only",
			},
			run: cmdResearch,
		},
		{
			name:    "approvals",
			group:   "Run & control",
			summary: "list pending human-in-the-loop approval requests",
			detail: []string{
				"approvals [--json]",
			},
			run: cmdApprovals,
		},
		{
			name:    "approve",
			group:   "Run & control",
			summary: "grant a pending approval",
			detail: []string{
				"approve <id> [reason]",
			},
			run: func(args []string, stdout io.Writer, stderr io.Writer) int {
				return cmdDecide("grant", args, stdout, stderr)
			},
		},
		{
			name:    "deny",
			group:   "Run & control",
			summary: "deny a pending approval",
			detail: []string{
				"deny <id> [reason]",
			},
			run: func(args []string, stdout io.Writer, stderr io.Writer) int {
				return cmdDecide("deny", args, stdout, stderr)
			},
		},
		{
			name:    "whoami",
			group:   "Run & control",
			summary: "which identity (primary/tenant) this client authenticates as",
			detail: []string{
				"whoami [--tenant <id>] [--json]",
				"  set AGEZT_TOKEN=<tenant-token> + --tenant <id> to authenticate as a tenant",
			},
			run: whoami.Run,
		},
		// Plans & automation
		{
			name:    "plan",
			group:   "Plans & automation",
			summary: "generate / validate / visualize / execute DAG plans",
			detail: []string{
				"plan <file.json>                 execute a pre-built DAG plan",
				`plan generate "<intent>"         LLM-generate a plan; print JSON`,
				`plan run [--dry-run] "<intent>"  generate AND execute (--dry-run: preview + cost only)`,
				`plan refine <file> --feedback "..."   revise an existing plan`,
				"plan validate <file.json>        verify a hand-authored plan (client-side)",
				"plan visualize <file.json> [--raw]    render as Mermaid graph TD",
				"plan cost <file.json> --model <id>    estimate plan cost in USD (client-side)",
			},
			run: cmdPlan,
		},
		{
			name:    "schedule",
			group:   "Plans & automation",
			summary: "typed cron/event jobs",
			detail: []string{
				`schedule add "<agent task|label>" --every <dur>    also: workflow/system-task/tool targets`,
				`schedule add --system-task catalog_sync --every 24h    sync models.dev/api.json without waking an agent`,
				`schedule add "<agent task>" --continuous <dur>     cycle loop; re-wakes after each completed run`,
				`schedule edit <id> --continuous <dur>              convert an existing job into a cycle loop`,
			},
			run: cmdSchedule,
		},
		{
			name:    "standing",
			group:   "Plans & automation",
			summary: "durable wake rules for agents",
			detail: []string{
				"standing " + cliStanding.synopsis(),
			},
			run: cmdStanding,
		},
		{
			name:    "workflow",
			group:   "Plans & automation",
			summary: "author, save and run node-graph workflows",
			detail: []string{
				"workflow " + cliWorkflow.synopsis(),
				`workflow draft "DESCRIPTION" [--name N] [--save]    LLM-author a workflow`,
				"workflow save --file GRAPH.json",
			},
			run: cmdWorkflow,
		},
		{
			name:    "workboard",
			group:   "Plans & automation",
			summary: "durable typed task queue for multi-agent work",
			detail: []string{
				"workboard " + cliWorkboard.synopsis(),
				"workboard create --title T [--criterion \"tests pass\"] [--assignee A] [--priority N] [--max-attempts N]",
				"workboard prove <id>  (judge acceptance criteria → done if satisfied, else review)",
				"workboard dispatch <id> [--agent A] | workboard sweep [--stale-after 10m]",
			},
			run: cmdWorkboard,
		},
		{
			name:    "okr",
			group:   "Plans & automation",
			summary: "objectives + key results that roll up proven workboard tasks",
			detail: []string{
				"okr " + cliOKR.synopsis(),
				"okr create --title T [--owner O] | okr kr <id> --title T [--target N]",
				"okr link <id> --kr KR --task TASK  (roll a task's completion into a key result)",
			},
			run: cmdOKR,
		},
		{
			name:    "taste",
			group:   "Plans & automation",
			summary: "curated \"what good looks like\" exemplars injected into runs",
			detail: []string{
				"taste " + cliTaste.synopsis(),
				"taste add --title T --body TEXT [--scope AGENT] [--tag X]  (scope empty = every run)",
				"taste list [--scope S] | taste remove <id>",
			},
			run: cmdTaste,
		},
		{
			name:    "seats",
			group:   "Plans & automation",
			summary: "execution seats a workboard task can be dispatched under",
			detail: []string{
				"seats [list] [--json] | seats add <id> [--exec local|warden|container] [--name N] [--tool X] | seats remove <id>",
				"built-ins: default|reader|builder|isolated; a seat refines model/tool/isolation per task (workboard create --seat / workboard seat)",
			},
			run: cmdSeat,
		},
		{
			name:    "agent",
			group:   "Plans & automation",
			summary: "the named-agent roster (souls, models, budgets, workdirs)",
			detail: []string{
				"agent " + cliAgent.synopsis(),
				"agent add <slug> [--soul TEXT] [--model M] ...",
			},
			run: cmdAgent,
		},
		{
			name:    "toolforge",
			group:   "Plans & automation",
			summary: "agent-built tools: draft, test, promote to production",
			detail: []string{
				"toolforge " + cliToolforge.synopsis(),
			},
			run: cmdToolforge,
		},
		{
			name:    "mcp",
			group:   "Plans & automation",
			summary: "attach/detach Model Context Protocol servers at runtime",
			detail: []string{
				"mcp " + cliMCP.synopsis(),
				`mcp add <name> (--cmd EXE [--arg A ...] | --url URL [--header "K: V" ...]) [--desc TEXT]`,
			},
			run: cmdMCP,
		},
		{
			name:    "market",
			group:   "Plans & automation",
			summary: "capability marketplace: browse + install packs (skills + MCP + tools)",
			detail: []string{
				"market " + cliMarket.synopsis(),
				"market install <pack> [--marketplace M] [--version V]   materialize a pack into the Forge + MCP registry",
				"market <sources|add <url>|remove|sync>                 manage + sync remote marketplaces",
				"market " + cliMarket.synopsis() + "   author, sign, and publish your own packs",
			},
			run: cmdMarket,
		},
		// Providers & models
		{
			name:    "catalog",
			group:   "Providers & models",
			summary: "the models.dev provider/model catalog",
			detail: []string{
				"catalog sync [url] [--local] [--json]   sync from models.dev (--local = offline)",
				"catalog list [--json]                   providers + models + pricing",
				"catalog discover [url]                  auto-discover local Ollama models",
			},
			run: cmdCatalog,
		},
		{
			name:    "provider",
			group:   "Providers & models",
			summary: "credentials, keyrings, live checks, hot reload",
			detail: []string{
				"provider connect <id> --url <base> --model <m> [--env E --key K] [--default]   register a provider (custom) + key, live",
				"provider chatgpt " + cliProviderChatGPT.synopsis() + "   Sign in with ChatGPT (subscription, no API key)",
				"provider creds list|set <NAME> [<value>]|rm <NAME>    the vault (set prompts when value omitted)",
				"provider keys <list|add|activate|rm>    multiple keys per provider, pick the active one",
				"provider setup [provider-id]            prompt for missing keys (offline)",
				"provider import [--from f] [--all] [-y] discover keys already on this machine → vault",
				"provider check [id|--all] [--bench N] [--stream] [--json]   live roundtrip: creds + latency + cost",
				"provider reload                         re-read catalog + vault; rebuild providers in place",
			},
			run: cmdProvider,
		},
		{
			name:    "budget",
			group:   "Providers & models",
			summary: "current-day spend vs daily + per-task caps",
			detail: []string{
				"budget [--json]",
			},
			run: cmdBudget,
		},
		{
			name:    "tool",
			group:   "Providers & models",
			summary: "in-process tools advertised to the model",
			detail: []string{
				"tool list [--json]",
			},
			run: cmdTool,
		},
		{
			name:    "cache",
			group:   "Providers & models",
			summary: "prompt-cache effectiveness (reads, writes, saved $)",
			detail: []string{
				"cache [--since <dur>] [--tenant <id>] [--json]",
			},
			run: cmdCache,
		},
		{
			name:    "tenant",
			group:   "Providers & models",
			summary: "isolated multi-tenant homes (daemon AGEZT_MULTITENANT=on)",
			detail: []string{
				"tenant create <id> [--json]    create / open a tenant (prints its token)",
				"tenant list|stats|token <id>|release <id>|rm <id>",
			},
			run: cmdTenant,
		},
		// Memory & knowledge
		{
			name:    "memory",
			group:   "Memory & knowledge",
			summary: "the agent's long-term memory records",
			detail: []string{
				"memory add <subject> <content> [--type T] [--evidence E] [--half-life D] [--tag k=v] [--conf F]",
				"memory list | search <query> [N] | get <id> | forget <id>   [--json]",
				"memory audit                          report stale/suspended/conflicting records",
				"memory clean [--execute]              soft-forget low-value automatic memories",
				"memory promote <id>                   move an agent-private record into the shared brain",
				"memory consolidate                    distill the brain (same pass AGEZT_BRAIN_DISTILL_EVERY runs)",
			},
			run: cmdMemory,
		},
		{
			name:    "world",
			group:   "Memory & knowledge",
			summary: "the entity/relation world model",
			detail: []string{
				"world add <name> [--kind K] [--alias A ...]",
				"world relate <from> <verb> <to> | resolve <phrase> [N] | neighbors <name>",
				"world list | show <id> | forget <id>   [--json]",
			},
			run: cmdWorld,
		},
		{
			name:    "skill",
			group:   "Memory & knowledge",
			summary: "learned skills + lifecycle (draft → shadow → active)",
			detail: []string{
				"skill list | show <id> | history <id>   [--json]",
				"skill promote <id> | quarantine <id> [--reason R] | archive <id> [--reason R] | revert <id>",
				"skill workshop " + cliSkillWorkshop.synopsis(),
				"skill share <id> | reassign <id> [--agent S]   (ownership: per-agent ↔ shared)",
			},
			run: cmdSkill,
		},
		{
			name:    "reflect",
			group:   "Memory & knowledge",
			summary: "reflection passes (decay stale world-model entities)",
			detail: []string{
				"reflect run [--json] | reflect show [--json]",
			},
			run: cmdReflect,
		},
		{
			name:    "state",
			group:   "Memory & knowledge",
			summary: "raw key/value state namespaces",
			detail: []string{
				"state list [<namespace>] [--json] | state get <namespace> <key> [--json]",
			},
			run: cmdState,
		},
		{
			name:    "artifact",
			group:   "Memory & knowledge",
			summary: "fetch a stored artifact's raw bytes by content ref",
			detail: []string{
				"artifact get <ref> [--out <file>]",
			},
			run: cmdArtifact,
		},
		// Journal & audit
		{
			name:    "journal",
			group:   "Journal & audit",
			summary: "the tamper-evident, hash-chained event journal",
			detail: []string{
				"journal verify [--bundle <file>]   verify the live chain, or an exported bundle offline",
				"journal tail [N] [--json]          last N events (default 20)",
				"journal grep <pattern> [--kind|--subject|--actor|--correlation]",
				"journal head [--json]              current head seq + chain-tail hash",
				"journal export [--since <dur>] [--out <file>]   re-verifiable bundle (archive/audit)",
				"journal import <bundle> [--home <dir>]          restore into an empty journal (offline)",
				"journal stats [--json]             what's filling the journal",
			},
			run: cmdJournal,
		},
		{
			name:    "pulse",
			group:   "Journal & audit",
			summary: "live tail of the event bus + the proactive heartbeat",
			detail: []string{
				"pulse [--subject PATTERN] [--kind K] [--json]   live tail (Ctrl+C to exit)",
				"pulse status [--json] | pulse pause | pulse resume",
				"pulse asks [--json] | pulse asks {approve|reject} <issue_key>   ask-mode verdicts",
			},
			run: cmdPulse,
		},
		{
			name:    "changelog",
			group:   "Journal & audit",
			summary: "system-level changes folded from the journal",
			detail: []string{
				"changelog [N] [--since <dur>] [--json]",
			},
			run: cmdChangelog,
		},
		{
			name:    "edict",
			group:   "Journal & audit",
			summary: "the policy engine: levels, hard-denies, dry-runs",
			detail: []string{
				"edict show [--json]                 loaded policies (ask_policy, levels, hard-deny rules)",
				"edict test <cap> [<input>] [--json] dry-run a decision; exit 3 = deny",
			},
			run: cmdEdict,
		},
		{
			name:    "warden",
			group:   "Journal & audit",
			summary: "shell-isolation profile downgrades and limit breaches",
			detail: []string{
				"warden log [N] [--issues] [--since <dur>] [--tenant <id>] [--json]",
			},
			run: cmdWarden,
		},
		{
			name:    "exec-profile",
			aliases: []string{"execution-profile"},
			group:   "Journal & audit",
			summary: "named execution profiles with requested vs effective isolation",
			detail: []string{
				"exec-profile list [--tenant <id>] [--json]",
				"exec-profile show <id> [--tenant <id>] [--json]",
				"exec-profile check [--tenant <id>] [--json]   routing, policy, downgrade, backend/secret readiness",
			},
			run: cmdExecProfile,
		},
		{
			name:    "redact",
			group:   "Journal & audit",
			summary: "what the secret-scrubber would do to a string",
			detail: []string{
				"redact test <string> [--json]",
			},
			run: cmdRedact,
		},
		{
			name:    "compare",
			group:   "Journal & audit",
			summary: "local OpenClaw/Hermes parity evidence audit",
			detail: []string{
				"compare audit [--target openclaw|hermes|all] [--root <repo>] [--json]",
			},
			run: cmdCompare,
		},
		{
			name:    "netguard",
			group:   "Journal & audit",
			summary: "the egress guard: test a host, list blocked dials",
			detail: []string{
				"netguard test <host|ip> [--json] | netguard log [N] [--since <dur>] [--json]",
			},
			run: cmdNetguard,
		},
		{
			name:    "ratelimit",
			group:   "Journal & audit",
			summary: "throttled calls (per-minute caps)",
			detail: []string{
				"ratelimit log [N] [--tenant <id>] [--since <dur>] [--json] | ratelimit stats [--json]",
			},
			run: cmdRateLimit,
		},
		{
			name:    "webhook",
			group:   "Journal & audit",
			summary: "outbound webhook deliveries: test an endpoint, audit sends",
			detail: []string{
				"webhook test [<url>] [--subject <pat>] [--secret <key>] [--json]",
				"webhook log [N] [--failed] [--since <dur>] [--json] | webhook stats [--json]",
			},
			run: cmdWebhook,
		},
		// Console, config & data
		{
			name:    "config",
			group:   "Console, config & data",
			summary: "the Config Center from the terminal",
			detail: []string{
				"config show [--json]      resolved config (paths, model, env presence)",
				"config ls | get <ENV> | set <ENV> <value>   read/write settings (secrets → vault)",
				"config schema [register <file> | unregister <id>]",
			},
			run: cmdConfig,
		},
		{
			name:    "web",
			group:   "Console, config & data",
			summary: "console (Web UI) management",
			detail: []string{
				"web password set [<value>]   set the console password (prompts + confirms when omitted; live when the daemon runs)",
				"web password clear           remove it — the console reverts to token-only",
				"web password status          report whether one is set (never the value)",
			},
			run: cmdWeb,
		},
		{
			name:    "configcenter",
			group:   "Console, config & data",
			summary: "rated config entries for SDK agents (public/internal/restricted/secret)",
			detail: []string{
				"configcenter set <key> <value> [--rating R] [--description D]",
				"configcenter get <key> | list [--rating R] [--json] | delete <key>",
				"configcenter rating <key> [--rating R] | access-log | audit | health",
			},
			run: cmdConfigCenter,
		},
		{
			name:    "token",
			group:   "Console, config & data",
			summary: "scoped JWT capability tokens for SDK agent subprocesses",
			detail: []string{
				"token create [--run-id ID] [--caps CAPS] [--max-rpm N] [--burst N] [--expiry DUR]",
				"token validate <TOKEN>     check a token and show its claims",
			},
			run: cmdToken,
		},
		{
			name:    "vault",
			group:   "Console, config & data",
			summary: "the encrypted credentials vault",
			detail: []string{
				"vault status     encryption state + path",
				"vault encrypt    migrate a plaintext vault to encrypted",
				"vault decrypt    migrate back to plaintext",
				"vault migrate    upgrade an old encrypted vault to the current KDF policy",
				"vault rotate     re-encrypt under AGEZT_VAULT_PASSPHRASE_NEW",
			},
			run: cmdVault,
		},
		{
			name:    "backup",
			group:   "Console, config & data",
			summary: "archive the home dir (journal head recorded; creds excluded)",
			detail: []string{
				"backup [--home <dir>] [--out <file>] | backup inspect <file> [--json]",
			},
			run: cmdBackup,
		},
		{
			name:    "restore",
			group:   "Console, config & data",
			summary: "restore a backup bundle into a fresh home",
			detail: []string{
				"restore <bundle> --home <fresh-dir>",
			},
			run: cmdRestore,
		},
		{
			name:    "rollback",
			group:   "Console, config & data",
			summary: "local mutation checkpoints and operator restore",
			detail: []string{
				"rollback list [--run <id>] [--json]",
				"rollback show|dry-run <checkpoint> [--json]",
				"rollback apply <checkpoint> [--json]",
			},
			run: cmdRollback,
		},
		{
			name:    "disk",
			group:   "Console, config & data",
			summary: "what under the home dir takes the space + free headroom",
			detail: []string{
				"disk [--json]",
			},
			run: cmdDisk,
		},
		// Channels & integrations
		{
			name:    "inbox",
			group:   "Channels & integrations",
			summary: "unified channel conversations (newest first)",
			detail: []string{
				"inbox [N] [--json]",
			},
			run: cmdInbox,
		},
		{
			name:    "send",
			group:   "Channels & integrations",
			summary: "push an outbound message through a channel",
			detail: []string{
				"send --channel KIND --to ID <text>",
			},
			run: cmdSend,
		},
		{
			name:    "channel",
			group:   "Channels & integrations",
			summary: "communication channels + their connectivity status",
			detail: []string{
				"channel list [--json]    every channel: live / configured / roundtrip readiness",
			},
			run: cmdChannel,
		},
		{
			name:    "ha",
			group:   "Channels & integrations",
			summary: "operator-facing Home Assistant client",
			detail: []string{
				"ha <states|services|call>",
			},
			run: cmdHA,
		},
		{
			name:    "transcribe",
			group:   "Channels & integrations",
			summary: "speech-to-text an audio file (→ agent with --run)",
			detail: []string{
				"transcribe <file> [--run]",
			},
			run: cmdTranscribe,
		},
		{
			name:    "listen",
			group:   "Channels & integrations",
			summary: "record the mic, transcribe it (→ agent with --run)",
			detail: []string{
				"listen [--seconds N] [--run]",
			},
			run: cmdListen,
		},
		{
			name:    "peers",
			group:   "Channels & integrations",
			summary: "configured peer nodes + their REST health",
			detail: []string{
				"peers [--json]",
				"peers models [<name>] [--json]",
				"peers route <model> [--json]",
				"peers run <peer> <corr> [--json]",
				"peers artifacts <peer> <corr> [--json]",
				"peers artifact-get <peer> <artifact_id> <out_file> [--json]",
			},
			run: cmdPeers,
		},
		{
			name:    "acp",
			group:   "Channels & integrations",
			summary: "Agent Client Protocol server over stdio (point Zed/an IDE at it)",
			detail: []string{
				"acp",
			},
			run: cmdACP,
		},
		{
			name:    "overseer",
			group:   "Channels & integrations",
			summary: "Fleet supervisory commands — status, agents, runs, cancel, halt, resume, pause, unpause, retire, revive, delete, get, impact",
			detail: []string{
				"overseer status",
				"overseer agents",
				"overseer runs",
				"overseer get <slug>",
				"overseer cancel <corr-id>",
				"overseer halt",
				"overseer resume",
				"overseer pause <slug>",
				"overseer unpause <slug>",
				"overseer retire <slug> [reason]",
				"overseer revive <slug>",
				"overseer delete <slug>",
				"overseer bulk pause <slug1,slug2>",
				"overseer bulk retire <slug1,slug2>",
				"overseer bulk delete <slug1,slug2>",
				"overseer impact <slug>",
			},
			run: cmdOverseer,
		},
		{
			name:    "plugin",
			group:   "Channels & integrations",
			summary: "external tool plugins the daemon spawned",
			detail: []string{
				"plugin list [--json] | plugin hash <path>   (BLAKE3 digest for AGEZT_PLUGIN_PINS)",
			},
			run: cmdPlugin,
		},
		// Daemon
		{
			name:    "shutdown",
			group:   "Daemon",
			summary: "ask the daemon to exit gracefully (same path as SIGTERM)",
			detail: []string{
				"shutdown [--json]",
			},
			run: cmdShutdown,
		},
	}
	for _, command := range cliTopCommands {
		Register(&Command{
			Name:    command.name,
			Aliases: command.aliases,
			Run:     command.run,
		})
	}
}

// cliTopGroups is the order the help overview lists the groups in:
// task-frequency-first, the things an operator types daily lead.
var cliTopGroups = []string{
	"Getting started",
	"Run & control",
	"Plans & automation",
	"Providers & models",
	"Memory & knowledge",
	"Journal & audit",
	"Console, config & data",
	"Channels & integrations",
	"Daemon",
}

// helpGroups derives the help overview from the command table.
func helpGroups() []helpGroup {
	groups := make([]helpGroup, 0, len(cliTopGroups))
	for _, title := range cliTopGroups {
		group := helpGroup{title: title}
		for _, command := range cliTopCommands {
			if command.group != title {
				continue
			}
			group.commands = append(group.commands, commandHelp{
				name:    command.name,
				summary: command.summary,
				detail:  command.detail,
			})
		}
		groups = append(groups, group)
	}
	return groups
}
