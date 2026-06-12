// SPDX-License-Identifier: MIT

package main

// The agt help system (M935). The old help was one flat 100+-line dump that
// listed every subcommand flag of SOME commands and omitted ~20 commands
// entirely (backup, warden, standing, workflow, …) — it rotted because nothing
// tied the dispatch table to the help text. This file makes help DATA:
//
//   - `agt` / `agt help` / `agt --help`  → a grouped one-line-per-command
//     overview that fits on a screen;
//   - `agt help <command>`               → that command's full usage (the
//     detail lines that used to bloat the overview live here);
//   - TestHelp_CoversEveryCommand        → every dispatch case in main.go must
//     have a help entry, so a new command can't ship invisible again.
//
// Per-command -h flags keep working as before; `agt help <cmd>` reads from
// this table and never executes the command (so `agt help halt` can't halt
// anything).

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

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
			{"doctor", "preflight checklist (base dir, daemon, journal, tools); exit 1 = a check failed", []string{
				"doctor [--json]",
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
			{"schedule", "recurring autonomous intents", []string{
				`schedule add "<intent>" --every <dur>    also: list | rm <id> | run <id>`,
			}},
			{"standing", "standing orders: event + cron triggered intents", []string{
				"standing <list|add|pause|resume|remove>",
			}},
			{"workflow", "author, save and run node-graph workflows", []string{
				"workflow <list|show|save|draft|refine|run|enable|disable|remove>",
				`workflow draft "DESCRIPTION" [--name N] [--save]    LLM-author a workflow`,
				"workflow save --file GRAPH.json",
			}},
			{"agent", "the named-agent roster (souls, models, budgets, workdirs)", []string{
				"agent <list|add|show|set|pause|resume|retire|remove>",
				"agent add <slug> [--soul PROMPT] [--model M] ...",
			}},
			{"toolforge", "agent-built tools: draft, test, promote to production", []string{
				"toolforge <list|show|draft|edit|test|promote|quarantine|remove>",
			}},
			{"mcp", "attach/detach Model Context Protocol servers at runtime", []string{
				"mcp <list|add|attach|detach|enable|disable|remove>",
				`mcp add <name> (--cmd EXE [--arg A ...] | --url URL [--header "K: V" ...]) [--desc TEXT]`,
			}},
		}},
		{"Providers & models", []commandHelp{
			{"catalog", "the models.dev provider/model catalog", []string{
				"catalog sync [url] [--local] [--json]   sync from models.dev (--local = offline)",
				"catalog list [--json]                   providers + models + pricing",
				"catalog discover [url]                  auto-discover local Ollama models",
			}},
			{"provider", "credentials, keyrings, live checks, hot reload", []string{
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
				"memory add <subject> <content> [--type T] [--tag k=v] [--conf F]",
				"memory list | search <query> [N] | get <id> | forget <id>   [--json]",
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
				"skill promote <id> | quarantine <id> [--reason R] | revert <id>",
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
			{"redact", "what the secret-scrubber would do to a string", []string{
				"redact test <string> [--json]",
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
			}},
			{"acp", "Agent Client Protocol server over stdio (point Zed/an IDE at it)", []string{
				"acp",
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
func printHelp(w io.Writer) {
	fmt.Fprintf(w, "usage: %s <command> [args...]\n", brand.CLI)
	fmt.Fprintf(w, "       %s help <command>   full usage for one command (also: %s <command> -h)\n", brand.CLI, brand.CLI)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "New here? Run `%s quickstart` — it syncs the catalog, adds a provider\n", brand.CLI)
	fmt.Fprintf(w, "key, and prints the exact command to start the daemon.\n")
	for _, g := range helpGroups() {
		fmt.Fprintf(w, "\n%s:\n", g.title)
		for _, c := range g.commands {
			fmt.Fprintf(w, "  %-12s %s\n", c.name, c.summary)
		}
	}
}

// helpHas reports whether the help table documents the command — the gate for
// the uniform `agt <cmd> -h` interception in main.go.
func helpHas(name string) bool {
	for _, g := range helpGroups() {
		for _, c := range g.commands {
			if c.name == name {
				return true
			}
		}
	}
	return false
}

// cmdHelp implements `agt help [<command>]`: the overview without an argument,
// one command's detail block with one. Reads only the table — it never runs
// the command, so `agt help halt` can't halt anything.
func cmdHelp(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printHelp(stdout)
		return 0
	}
	want := strings.TrimSpace(args[0])
	for _, g := range helpGroups() {
		for _, c := range g.commands {
			if c.name != want {
				continue
			}
			fmt.Fprintf(stdout, "%s %s — %s\n\n", brand.CLI, c.name, c.summary)
			for _, d := range c.detail {
				// Lines starting with a space are flag/continuation lines of the
				// usage above them — indent, don't re-prefix with the binary name.
				if strings.HasPrefix(d, " ") {
					fmt.Fprintf(stdout, "     %s\n", d)
				} else {
					fmt.Fprintf(stdout, "  %s %s\n", brand.CLI, d)
				}
			}
			return 0
		}
	}
	fmt.Fprintf(stderr, "%s help: unknown command %q", brand.CLI, want)
	if sug := suggestCommands(want); len(sug) > 0 {
		fmt.Fprintf(stderr, " — did you mean %s?", strings.Join(sug, ", "))
	}
	fmt.Fprintf(stderr, "\n")
	return 2
}

// suggestCommands offers near-misses for an unknown command: prefix/substring
// matches plus a small edit-distance pass (≤2) so "jurnal" finds "journal".
func suggestCommands(typo string) []string {
	typo = strings.ToLower(typo)
	if typo == "" {
		return nil
	}
	var out []string
	for _, g := range helpGroups() {
		for _, c := range g.commands {
			if strings.HasPrefix(c.name, typo) || strings.Contains(c.name, typo) ||
				strings.HasPrefix(typo, c.name) || editDistance(typo, c.name) <= 2 {
				out = append(out, c.name)
			}
		}
	}
	sort.Strings(out)
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}

// editDistance is plain Levenshtein over two short ASCII-ish strings — small
// inputs, no allocation concerns beyond two rows.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, min(cur[j-1]+1, prev[j-1]+cost))
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
