// SPDX-License-Identifier: MIT

// agt help data: helpGroups() returns the structured list of every CLI command + its usage text.
// Code extracted from help.go during the Day-109 god-file split.
// Public API unchanged.
package main




// commandHelp is one command's help: the overview line and the detail block.
type commandHelp struct {
	name    string   // the dispatch token (`agt <name> …`)
	summary string   // one line for the grouped overview
	detail  []string // full usage lines for `agt help <name>` (without the leading "usage:")
}

// helpGroup is a titled section of the overview.
type helpGroup struct {
	title    string
	commands []commandHelp
}

// helpGroups is the single source of truth for `agt help`. Ordering is
// task-frequency-first: the things an operator types daily lead.
func helpGroups() []helpGroup {
	return []helpGroup{
		{"Getting started", []commandHelp{
			{"quickstart", "interactive first-run: sync catalog, add a key, print the start command", []string{
				"quickstart",
			}},
			{"doctor", "preflight checklist (base dir, memory store, daemon, journal, tools); exit 1 = a check failed", []string{
				"doctor [--json] [--strict] [--repair]",
				"  --repair  repair startup-blocking local store files and pause noisy schedules",
			}},
			{"status", "daemon health overview (version skew, uptime, runs)", []string{
				"status [--json]",
			}},
			{"version", "show client version", []string{"version"}},
			{"help", "this overview, or one command's full usage", []string{
				"help [<command>]",
			}},
		}},
		{"Run & control", []commandHelp{
			{"run", "run an intent through the governed agent loop", []string{
				`run "<intent>" | run - | run --file <path>     intent from arg, stdin, or a file`,
				"  [--json|-q]                -q/--quiet = only the answer; --json = ndjson event stream",
				"  [--model <id>] [--system <prompt>] [--timeout <dur>] [--tenant <id>]   per-run overrides",
				"  [--exec-profile <local|warden|docker|ssh|k8s|modal|daytona|remote-agezt>]   request local/container/remote execution",
				"  [--peer <name>]              with remote-agezt, pin delegation to a configured peer",
				"  [--tools <csv>|--no-tools] [--dry-run] [--max-cost <usd>]   restrict tools / preview / cap spend",
				"  [--assure[=<n>]]           run, verify it's actually done, retry the gap (default 3 attempts)",
			}},
			{"halt", "freeze all in-flight runs (reason is journaled)", []string{
				`halt [--reason "..."] [--json]`,
			}},
			{"resume", "clear the halt flag (reason is journaled)", []string{
				`resume [--reason "..."] [--json]`,
			}},
			{"runs", "list past runs / replay one as a task arc", []string{
				"runs list [N] [--json]        last N agent runs (task-level summary)",
				"runs show <correlation> [--json]",
				"runs last [--json]            the most-recent run",
			}},
			{"why", "every event sharing one event's correlation (the run's story)", []string{
				"why <event_id> [--json|--payload]",
			}},
			{"conductor", "run Thinker/Worker/Verifier roles on (usually) different models to solve & verify a hard task", []string{
				`conductor "<task>"            run with auto-filled roles (one per keyed provider)`,
				"  [--thinker <model>] [--worker <model>] [--verifier <model>]   role→model overrides (id or @chain)",
				"  [--max-rounds <n>]          worker/verifier retry cap (default 2)",
				"  [--plan]                    tailor per-role instructions first",
				"  [--json|-q]                 full transcript as JSON / answer-only",
				"  exit: 0 = verified, 3 = not verified, 1 = error",
			}},
			{"research", "deep-research: decompose, gather web sources, synthesize a cited answer, verify each claim", []string{
				`research "<question>"         gather + synthesize + adversarially verify`,
				"  [--max-sources <n>]         cap sources gathered (default 8, max 20)",
				"  [--max-sub-questions <n>]   cap sub-questions explored (default 3, max 8)",
				"  [--max-verify-claims <n>]   cap claims verified (default 6, max 12)",
				"  [--no-verify]               skip the adversarial claim check",
				"  [--json|-q]                 full report (sources + claims) as JSON / answer-only",
			}},
			{"approvals", "list pending human-in-the-loop approval requests", []string{
				"approvals [--json]",
			}},
			{"approve", "grant a pending approval", []string{"approve <id> [reason]"}},
			{"deny", "deny a pending approval", []string{"deny <id> [reason]"}},
			{"whoami", "which identity (primary/tenant) this client authenticates as", []string{
				"whoami [--tenant <id>] [--json]",
				"  set AGEZT_TOKEN=<tenant-token> + --tenant <id> to authenticate as a tenant",
			}},
		}},
		{"Plans & automation", []commandHelp{
			{"plan", "generate / validate / visualize / execute DAG plans", []string{
				"plan <file.json>                 execute a pre-built DAG plan",
				`plan generate "<intent>"         LLM-generate a plan; print JSON`,
				`plan run [--dry-run] "<intent>"  generate AND execute (--dry-run: preview + cost only)`,
				`plan refine <file> --feedback "..."   revise an existing plan`,
				"plan validate <file.json>        verify a hand-authored plan (client-side)",
				"plan visualize <file.json> [--raw]    render as Mermaid graph TD",
				"plan cost <file.json> --model <id>    estimate plan cost in USD (client-side)",
			}},
			{"schedule", "typed cron/event jobs", []string{
				`schedule add "<agent task|label>" --every <dur>    also: workflow/system-task/tool targets`,
				`schedule add --system-task catalog_sync --every 24h    sync models.dev/api.json without waking an agent`,
				`schedule add "<agent task>" --continuous <dur>     cycle loop; re-wakes after each completed run`,
				`schedule edit <id> --continuous <dur>              convert an existing job into a cycle loop`,
			}},
			{"standing", "durable wake rules for agents", []string{
				"standing <list|add|pause|resume|remove>",
			}},
			{"workflow", "author, save and run node-graph workflows", []string{
				"workflow <list|show|save|draft|refine|run|enable|disable|remove>",
				`workflow draft "DESCRIPTION" [--name N] [--save]    LLM-author a workflow`,
				"workflow save --file GRAPH.json",
			}},
			{"workboard", "durable typed task queue for multi-agent work", []string{
				"workboard <list|lanes|show|create|claim|heartbeat|comment|block|fail|unblock|complete|prove|archive|link|policy|depend|reclaim|sweep|dispatch|watch>",
				"workboard create --title T [--criterion \"tests pass\"] [--assignee A] [--priority N] [--max-attempts N]",
				"workboard prove <id>  (judge acceptance criteria → done if satisfied, else review)",
				"workboard dispatch <id> [--agent A] | workboard sweep [--stale-after 10m]",
			}},
			{"okr", "objectives + key results that roll up proven workboard tasks", []string{
				"okr <list|show|create|kr|link|unlink|archive>",
				"okr create --title T [--owner O] | okr kr <id> --title T [--target N]",
				"okr link <id> --kr KR --task TASK  (roll a task's completion into a key result)",
			}},
			{"taste", "curated \"what good looks like\" exemplars injected into runs", []string{
				"taste <list|add|remove>",
				"taste add --title T --body TEXT [--scope AGENT] [--tag X]  (scope empty = every run)",
				"taste list [--scope S] | taste remove <id>",
			}},
			{"seats", "execution seats a workboard task can be dispatched under", []string{
				"seats [list] [--json] | seats add <id> [--exec local|warden|container] [--name N] [--tool X] | seats remove <id>",
				"built-ins: default|reader|builder|isolated; a seat refines model/tool/isolation per task (workboard create --seat / workboard seat)",
			}},
			{"agent", "the named-agent roster (souls, models, budgets, workdirs)", []string{
				"agent <list|add|show|impact|set|task|wake|repair|repair-status|pause|resume|retire|revive|remove>",
				"agent add <slug> [--soul TEXT] [--model M] ...",
			}},
			{"toolforge", "agent-built tools: draft, test, promote to production", []string{
				"toolforge <list|show|draft|edit|test|promote|quarantine|remove>",
			}},
			{"mcp", "attach/detach Model Context Protocol servers at runtime", []string{
				"mcp <list|add|attach|detach|enable|disable|remove>",
				`mcp add <name> (--cmd EXE [--arg A ...] | --url URL [--header "K: V" ...]) [--desc TEXT]`,
			}},
			{"market", "capability marketplace: browse + install packs (skills + MCP + tools)", []string{
				"market <list|search|show|install|uninstall>",
				"market install <pack> [--marketplace M] [--version V]   materialize a pack into the Forge + MCP registry",
				"market <sources|add <url>|remove|sync>                 manage + sync remote marketplaces",
				"market <validate|publish|keygen>                       author, sign, and publish your own packs",
			}},
		}},
		{"Providers & models", []commandHelp{
			{"catalog", "the models.dev provider/model catalog", []string{
				"catalog sync [url] [--local] [--json]   sync from models.dev (--local = offline)",
				"catalog list [--json]                   providers + models + pricing",
				"catalog discover [url]                  auto-discover local Ollama models",
			}},
			{"provider", "credentials, keyrings, live checks, hot reload", []string{
				"provider connect <id> --url <base> --model <m> [--env E --key K] [--default]   register a provider (custom) + key, live",
				"provider chatgpt <login|import|logout|status>   Sign in with ChatGPT (subscription, no API key)",
				"provider creds list|set <NAME> [<value>]|rm <NAME>    the vault (set prompts when value omitted)",
				"provider keys <list|add|activate|rm>    multiple keys per provider, pick the active one",
				"provider setup [provider-id]            prompt for missing keys (offline)",
				"provider import [--from f] [--all] [-y] discover keys already on this machine → vault",
				"provider check [id|--all] [--bench N] [--stream] [--json]   live roundtrip: creds + latency + cost",
				"provider reload                         re-read catalog + vault; rebuild providers in place",
			}},
			{"budget", "current-day spend vs daily + per-task caps", []string{
				"budget [--json]",
			}},
			{"tool", "in-process tools advertised to the model", []string{
				"tool list [--json]",
			}},
			{"cache", "prompt-cache effectiveness (reads, writes, saved $)", []string{
				"cache [--since <dur>] [--tenant <id>] [--json]",
			}},
			{"tenant", "isolated multi-tenant homes (daemon AGEZT_MULTITENANT=on)", []string{
				"tenant create <id> [--json]    create / open a tenant (prints its token)",
				"tenant list|stats|token <id>|release <id>|rm <id>",
			}},
		}},
		{"Memory & knowledge", []commandHelp{
			{"memory", "the agent's long-term memory records", []string{
				"memory add <subject> <content> [--type T] [--evidence E] [--half-life D] [--tag k=v] [--conf F]",
				"memory list | search <query> [N] | get <id> | forget <id>   [--json]",
				"memory audit                          report stale/suspended/conflicting records",
				"memory clean [--execute]              soft-forget low-value automatic memories",
				"memory promote <id>                   move an agent-private record into the shared brain",
				"memory consolidate                    distill the brain (same pass AGEZT_BRAIN_DISTILL_EVERY runs)",
			}},
			{"world", "the entity/relation world model", []string{
				"world add <name> [--kind K] [--alias A ...]",
				"world relate <from> <verb> <to> | resolve <phrase> [N] | neighbors <name>",
				"world list | show <id> | forget <id>   [--json]",
			}},
			{"skill", "learned skills + lifecycle (draft → shadow → active)", []string{
				"skill list | show <id> | history <id>   [--json]",
				"skill promote <id> | quarantine <id> [--reason R] | archive <id> [--reason R] | revert <id>",
				"skill workshop <list|inspect|scan|diff|curate|apply|reject|quarantine|propose-create|propose-update>",
				"skill share <id> | reassign <id> [--agent S]   (ownership: per-agent ↔ shared)",
			}},
			{"reflect", "reflection passes (decay stale world-model entities)", []string{
				"reflect run [--json] | reflect show [--json]",
			}},
			{"state", "raw key/value state namespaces", []string{
				"state list [<namespace>] [--json] | state get <namespace> <key> [--json]",
			}},
			{"artifact", "fetch a stored artifact's raw bytes by content ref", []string{
				"artifact get <ref> [--out <file>]",
			}},
		}},
		{"Journal & audit", []commandHelp{
			{"journal", "the tamper-evident, hash-chained event journal", []string{
				"journal verify [--bundle <file>]   verify the live chain, or an exported bundle offline",
				"journal tail [N] [--json]          last N events (default 20)",
				"journal grep <pattern> [--kind|--subject|--actor|--correlation]",
				"journal head [--json]              current head seq + chain-tail hash",
				"journal export [--since <dur>] [--out <file>]   re-verifiable bundle (archive/audit)",
				"journal import <bundle> [--home <dir>]          restore into an empty journal (offline)",
				"journal stats [--json]             what's filling the journal",
			}},
			{"pulse", "live tail of the event bus + the proactive heartbeat", []string{
				"pulse [--subject PATTERN] [--kind K] [--json]   live tail (Ctrl+C to exit)",
				"pulse status [--json] | pulse pause | pulse resume",
				"pulse asks [--json] | pulse asks {approve|reject} <issue_key>   ask-mode verdicts",
			}},
			{"changelog", "system-level changes folded from the journal", []string{
				"changelog [N] [--since <dur>] [--json]",
			}},
			{"edict", "the policy engine: levels, hard-denies, dry-runs", []string{
				"edict show [--json]                 loaded policies (ask_policy, levels, hard-deny rules)",
				"edict test <cap> [<input>] [--json] dry-run a decision; exit 3 = deny",
			}},
			{"warden", "shell-isolation profile downgrades and limit breaches", []string{
				"warden log [N] [--issues] [--since <dur>] [--tenant <id>] [--json]",
			}},
			{"exec-profile", "named execution profiles with requested vs effective isolation", []string{
				"exec-profile list [--tenant <id>] [--json]",
				"exec-profile show <id> [--tenant <id>] [--json]",
				"exec-profile check [--tenant <id>] [--json]   routing, policy, downgrade, backend/secret readiness",
			}},
			{"redact", "what the secret-scrubber would do to a string", []string{
				"redact test <string> [--json]",
			}},
			{"compare", "local OpenClaw/Hermes parity evidence audit", []string{
				"compare audit [--target openclaw|hermes|all] [--root <repo>] [--json]",
			}},
			{"netguard", "the egress guard: test a host, list blocked dials", []string{
				"netguard test <host|ip> [--json] | netguard log [N] [--since <dur>] [--json]",
			}},
			{"ratelimit", "throttled calls (per-minute caps)", []string{
				"ratelimit log [N] [--tenant <id>] [--since <dur>] [--json] | ratelimit stats [--json]",
			}},
			{"webhook", "outbound webhook deliveries: test an endpoint, audit sends", []string{
				"webhook test [<url>] [--subject <pat>] [--secret <key>] [--json]",
				"webhook log [N] [--failed] [--since <dur>] [--json] | webhook stats [--json]",
			}},
		}},
		{"Console, config & data", []commandHelp{
			{"config", "the Config Center from the terminal", []string{
				"config show [--json]      resolved config (paths, model, env presence)",
				"config ls | get <ENV> | set <ENV> <value>   read/write settings (secrets → vault)",
				"config schema [register <file> | unregister <id>]",
			}},
			{"web", "console (Web UI) management", []string{
				"web password set [<value>]   set the console password (prompts + confirms when omitted; live when the daemon runs)",
				"web password clear           remove it — the console reverts to token-only",
				"web password status          report whether one is set (never the value)",
			}},
			{"configcenter", "rated config entries for SDK agents (public/internal/restricted/secret)", []string{
				"configcenter set <key> <value> [--rating R] [--description D]",
				"configcenter get <key> | list [--rating R] [--json] | delete <key>",
				"configcenter rating <key> [--rating R] | access-log | audit | health",
			}},
			{"token", "scoped JWT capability tokens for SDK agent subprocesses", []string{
				"token create [--run-id ID] [--caps CAPS] [--max-rpm N] [--burst N] [--expiry DUR]",
				"token validate <TOKEN>     check a token and show its claims",
			}},
			{"vault", "the encrypted credentials vault", []string{
				"vault status     encryption state + path",
				"vault encrypt    migrate a plaintext vault to encrypted",
				"vault decrypt    migrate back to plaintext",
				"vault migrate    upgrade an old encrypted vault to the current KDF policy",
				"vault rotate     re-encrypt under AGEZT_VAULT_PASSPHRASE_NEW",
			}},
			{"backup", "archive the home dir (journal head recorded; creds excluded)", []string{
				"backup [--home <dir>] [--out <file>] | backup inspect <file> [--json]",
			}},
			{"restore", "restore a backup bundle into a fresh home", []string{
				"restore <bundle> --home <fresh-dir>",
			}},
			{"rollback", "local mutation checkpoints and operator restore", []string{
				"rollback list [--run <id>] [--json]",
				"rollback show|dry-run <checkpoint> [--json]",
				"rollback apply <checkpoint> [--json]",
			}},
			{"disk", "what under the home dir takes the space + free headroom", []string{
				"disk [--json]",
			}},
		}},
		{"Channels & integrations", []commandHelp{
			{"inbox", "unified channel conversations (newest first)", []string{
				"inbox [N] [--json]",
			}},
			{"send", "push an outbound message through a channel", []string{
				"send --channel KIND --to ID <text>",
			}},
			{"channel", "communication channels + their connectivity status", []string{
				"channel list [--json]    every channel: live / configured / roundtrip readiness",
			}},
			{"ha", "operator-facing Home Assistant client", []string{
				"ha <states|services|call>",
			}},
			{"transcribe", "speech-to-text an audio file (→ agent with --run)", []string{
				"transcribe <file> [--run]",
			}},
			{"listen", "record the mic, transcribe it (→ agent with --run)", []string{
				"listen [--seconds N] [--run]",
			}},
			{"peers", "configured peer nodes + their REST health", []string{
				"peers [--json]",
				"peers models [<name>] [--json]",
				"peers route <model> [--json]",
				"peers run <peer> <corr> [--json]",
				"peers artifacts <peer> <corr> [--json]",
				"peers artifact-get <peer> <artifact_id> <out_file> [--json]",
			}},
			{"acp", "Agent Client Protocol server over stdio (point Zed/an IDE at it)", []string{
				"acp",
			}},
			{"overseer", "Fleet supervisory commands — status, agents, runs, cancel, halt, resume, pause, unpause, retire, revive, delete, get, impact", []string{
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
			}},
			{"plugin", "external tool plugins the daemon spawned", []string{
				"plugin list [--json] | plugin hash <path>   (BLAKE3 digest for AGEZT_PLUGIN_PINS)",
			}},
		}},
		{"Daemon", []commandHelp{
			{"shutdown", "ask the daemon to exit gracefully (same path as SIGTERM)", []string{
				"shutdown [--json]",
			}},
		}},
	}
}

// printHelp renders the grouped one-line-per-command overview.
