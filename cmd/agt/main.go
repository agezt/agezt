// SPDX-License-Identifier: MIT

// Command agt is the Agezt command-line client.
//
// Subcommands:
//
//	agt run "<intent>"        run an intent end-to-end; streams events
//	agt halt                  freeze all in-flight runs
//	agt resume                clear the halt flag
//	agt why <event_id>        list the events sharing an event's correlation
//	agt journal verify        verify the BLAKE3 hash chain
//	agt version               show client version
//	agt help                  this help
//
// All commands connect to a running agezt daemon via the local control
// plane (TCP localhost + token file in $AGEZT_HOME/runtime/).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printHelp(stdout)
		return 0
	}
	switch args[0] {
	case "-v", "--version", "version":
		fmt.Fprintf(stdout, "%s %s (protocol v%d)\n", brand.CLI, brand.Version, brand.ProtocolVersion)
		return 0
	case "-h", "--help", "help":
		printHelp(stdout)
		return 0
	case "run":
		return cmdRun(args[1:], stdout, stderr)
	case "halt":
		return cmdHaltResume("halt", args[1:], stdout, stderr)
	case "resume":
		return cmdHaltResume("resume", args[1:], stdout, stderr)
	case "why":
		return cmdWhy(args[1:], stdout, stderr)
	case "journal":
		return cmdJournal(args[1:], stdout, stderr)
	case "approvals":
		return cmdApprovals(args[1:], stdout, stderr)
	case "approve":
		return cmdDecide("grant", args[1:], stdout, stderr)
	case "deny":
		return cmdDecide("deny", args[1:], stdout, stderr)
	case "plan":
		return cmdPlan(args[1:], stdout, stderr)
	case "catalog":
		return cmdCatalog(args[1:], stdout, stderr)
	case "provider":
		return cmdProvider(args[1:], stdout, stderr)
	case "pulse":
		return cmdPulse(args[1:], stdout, stderr)
	case "vault":
		return cmdVault(args[1:], stdout, stderr)
	case "plugin":
		return cmdPlugin(args[1:], stdout, stderr)
	case "budget":
		return cmdBudget(args[1:], stdout, stderr)
	case "tool":
		return cmdTool(args[1:], stdout, stderr)
	case "status":
		return cmdStatus(args[1:], stdout, stderr)
	case "shutdown":
		return cmdShutdown(args[1:], stdout, stderr)
	case "edict":
		return cmdEdict(args[1:], stdout, stderr)
	case "state":
		return cmdState(args[1:], stdout, stderr)
	case "runs":
		return cmdRuns(args[1:], stdout, stderr)
	case "config":
		return cmdConfig(args[1:], stdout, stderr)
	case "memory":
		return cmdMemory(args[1:], stdout, stderr)
	case "world":
		return cmdWorld(args[1:], stdout, stderr)
	case "inbox":
		return cmdInbox(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s: unknown command %q\n", brand.CLI, args[0])
		printHelp(stderr)
		return 2
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintf(w, "usage: %s <command> [args...]\n", brand.CLI)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Commands:\n")
	fmt.Fprintf(w, "  run \"<intent>\" [--json]   run an intent end-to-end (JSON = ndjson stream)\n")
	fmt.Fprintf(w, "  halt [--reason \"...\"] [--json]  freeze all in-flight runs (reason is journaled)\n")
	fmt.Fprintf(w, "  resume [--reason \"...\"] [--json] clear the halt flag (reason is journaled)\n")
	fmt.Fprintf(w, "  why <event_id> [--json|--payload]  list events sharing an event's correlation\n")
	fmt.Fprintf(w, "  journal verify                        verify the BLAKE3 hash chain\n")
	fmt.Fprintf(w, "  journal tail [N] [--json]             snapshot of the last N events (default 20)\n")
	fmt.Fprintf(w, "  journal grep <pattern> [filters]      server-side filter (--kind/--subject/--actor/--correlation)\n")
	fmt.Fprintf(w, "  journal head [--json]                 print current head seq + chain-tail hash\n")
	fmt.Fprintf(w, "  approvals [--json]                    list pending HITL approval requests\n")
	fmt.Fprintf(w, "  approve <id> [reason]  grant a pending approval\n")
	fmt.Fprintf(w, "  deny    <id> [reason]  deny a pending approval\n")
	fmt.Fprintf(w, "  plan <file.json>            execute a pre-built DAG plan\n")
	fmt.Fprintf(w, "  plan generate \"<intent>\"    LLM-generate a plan; print JSON (pipe to a file)\n")
	fmt.Fprintf(w, "  plan run      \"<intent>\"    LLM-generate AND execute a plan in one go\n")
	fmt.Fprintf(w, "  plan run --dry-run \"<intent>\" [--model <id>]\n")
	fmt.Fprintf(w, "                              preview only: generate, validate, visualize (+cost); no execution\n")
	fmt.Fprintf(w, "  plan refine <file> --feedback \"...\"\n")
	fmt.Fprintf(w, "                              revise an existing plan with operator feedback\n")
	fmt.Fprintf(w, "  plan validate <file.json>   verify a hand-authored plan (client-side, no daemon)\n")
	fmt.Fprintf(w, "  plan visualize <file.json> [--raw]\n")
	fmt.Fprintf(w, "                              render plan as Mermaid graph TD (pasteable into markdown)\n")
	fmt.Fprintf(w, "  plan cost <file.json> --model <id>\n")
	fmt.Fprintf(w, "                              estimate plan cost in USD (client-side)\n")
	fmt.Fprintf(w, "  catalog sync [url]                    sync provider/model catalog from models.dev\n")
	fmt.Fprintf(w, "  catalog list [--json]                 list synced providers + models + pricing\n")
	fmt.Fprintf(w, "  catalog discover [url]                auto-discover local Ollama models\n")
	fmt.Fprintf(w, "  provider creds list                   list credential env vars in the vault\n")
	fmt.Fprintf(w, "  provider creds set <NAME> [<value>]   store a credential (prompts if value omitted)\n")
	fmt.Fprintf(w, "  provider creds rm <NAME>              remove a credential\n")
	fmt.Fprintf(w, "  provider check [id]                   live roundtrip to verify creds + latency + cost\n")
	fmt.Fprintf(w, "  provider check --all                  probe every credentialed provider; summary table\n")
	fmt.Fprintf(w, "  provider check --bench N [id]         run N probes; report min/p50/p95/max latencies\n")
	fmt.Fprintf(w, "  provider check --json [id|--all]      machine-readable output (CI-friendly)\n")
	fmt.Fprintf(w, "  provider check --stream [id]          live SSE roundtrip; renders tokens inline\n")
	fmt.Fprintf(w, "  provider reload                       re-read catalog + vault; rebuild primary in place\n")
	fmt.Fprintf(w, "  pulse [--subject PATTERN] [--kind K]  live tail of the daemon's event bus (Ctrl+C to exit)\n")
	fmt.Fprintf(w, "  pulse --json                          one JSON event per line (pipe to jq, etc.)\n")
	fmt.Fprintf(w, "  pulse status [--json]                 proactive engine: beats, observers, dial\n")
	fmt.Fprintf(w, "  pulse pause | pulse resume            suspend/resume the proactive heartbeat\n")
	fmt.Fprintf(w, "  budget [--json]                       show current-day spend vs daily + per-task-type caps\n")
	fmt.Fprintf(w, "  tool list [--json]                    list in-process tools advertised to the model\n")
	fmt.Fprintf(w, "  status [--json]                       daemon health overview (version skew, uptime, runs)\n")
	fmt.Fprintf(w, "  plugin hash <path>                    BLAKE3 hex digest of a plugin binary (for AGEZT_PLUGIN_PINS)\n")
	fmt.Fprintf(w, "  plugin list [--json]                  list external plugins the daemon spawned\n")
	fmt.Fprintf(w, "  shutdown [--json]                     ask the daemon to exit gracefully (same path as SIGTERM)\n")
	fmt.Fprintf(w, "  edict show [--json]                   show loaded policies (ask_policy, levels, hard-deny rules)\n")
	fmt.Fprintf(w, "  edict test <cap> [<input>] [--json]   dry-run a policy decision; exit 3 = deny\n")
	fmt.Fprintf(w, "  state list [<namespace>] [--json]     enumerate state namespaces or keys\n")
	fmt.Fprintf(w, "  state get <namespace> <key> [--json]  read one state value (exit 3 = absent)\n")
	fmt.Fprintf(w, "  runs list [N] [--json]                list the last N agent runs (task-level summary)\n")
	fmt.Fprintf(w, "  runs show <correlation> [--json]      render one run as a task arc\n")
	fmt.Fprintf(w, "  runs last [--json]                    render the most-recent run as a task arc\n")
	fmt.Fprintf(w, "  config show [--json]                  daemon's resolved config (paths, model, env presence)\n")
	fmt.Fprintf(w, "  memory add <subject> <content> [--type T] [--tag k=v] [--conf F] [--json]\n")
	fmt.Fprintf(w, "                              store a fact in memory (the agent reads it as context)\n")
	fmt.Fprintf(w, "  memory list [--json]                  list active memory records\n")
	fmt.Fprintf(w, "  memory search <query> [N] [--json]    rank records by keyword×confidence×recency\n")
	fmt.Fprintf(w, "  memory get <id> [--json]              read one record (exit 3 = absent)\n")
	fmt.Fprintf(w, "  memory forget <id> [--json]           tombstone a record (reversible, journaled)\n")
	fmt.Fprintf(w, "  world add <name> [--kind K] [--alias A ...] [--json]\n")
	fmt.Fprintf(w, "                              record an entity in the world model (the agent resolves references to it)\n")
	fmt.Fprintf(w, "  world relate <from> <verb> <to> [--json]   link two entities\n")
	fmt.Fprintf(w, "  world resolve <phrase> [N] [--json]   what does a phrase refer to?\n")
	fmt.Fprintf(w, "  world neighbors <name> [--json]       what an entity connects to\n")
	fmt.Fprintf(w, "  world list [--json]                   list active entities + relation count\n")
	fmt.Fprintf(w, "  world show <id> [--json]              read one entity (exit 3 = absent)\n")
	fmt.Fprintf(w, "  inbox [N] [--json]                    unified channel conversations (newest first)\n")
	fmt.Fprintf(w, "  vault status                          show vault encryption state + path\n")
	fmt.Fprintf(w, "  vault encrypt                         migrate plaintext vault to encrypted (set AGEZT_VAULT_PASSPHRASE)\n")
	fmt.Fprintf(w, "  vault decrypt                         migrate encrypted vault back to plaintext\n")
	fmt.Fprintf(w, "  version           show client version\n")
	fmt.Fprintf(w, "  help              show this help\n")
}

func dial(stderr io.Writer) *controlplane.Client {
	base, err := paths.BaseDir()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return nil
	}
	c, err := controlplane.NewClient(base)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		fmt.Fprintf(stderr, "Hint: start the daemon with `%s` in another terminal.\n", brand.Binary)
		return nil
	}
	return c
}

func cmdRun(args []string, stdout, stderr io.Writer) int {
	// Strip --json early so it works in any position: `agt run
	// "..." --json` or `agt run --json "..."` both compose.
	asJSON := false
	var intentParts []string
	for _, a := range args {
		if a == "--json" {
			asJSON = true
			continue
		}
		intentParts = append(intentParts, a)
	}
	intent := strings.TrimSpace(strings.Join(intentParts, " "))
	if intent == "" {
		fmt.Fprintf(stderr, "%s run: intent required (quote it as one argument)\n", brand.CLI)
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	if asJSON {
		return runJSONMode(ctx, c, intent, stdout, stderr)
	}

	// Stream-aware renderer: KindLLMToken events carry partial text
	// the model is generating live (ephemeral; never journaled — see
	// bus.PublishStreaming). Render those inline so the operator sees
	// progress instead of a frozen prompt. Every other event keeps
	// the existing `[evt seq=...]` summary line.
	inStream := false
	closeStream := func() {
		if inStream {
			fmt.Fprintln(stdout)
			inStream = false
		}
	}
	result, err := c.Stream(ctx, controlplane.CmdRun, map[string]any{"intent": intent}, func(ev *event.Event) {
		if ev.Kind == event.KindLLMToken {
			var p struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(ev.Payload, &p)
			if p.Text != "" {
				if !inStream {
					fmt.Fprint(stdout, "  ")
					inStream = true
				}
				fmt.Fprint(stdout, p.Text)
			}
			return
		}
		closeStream()
		fmt.Fprintf(stdout, "  [evt seq=%d kind=%s]\n", ev.Seq, ev.Kind)
	})
	closeStream()
	if err != nil {
		fmt.Fprintf(stderr, "%s run: %v\n", brand.CLI, err)
		return 1
	}
	corr, _ := result["correlation_id"].(string)
	ans, _ := result["answer"].(string)
	fmt.Fprintf(stdout, "\n--- final answer ---\n%s\n", ans)
	fmt.Fprintf(stdout, "(correlation_id: %s; use `%s why <event_id>` to walk the chain)\n", corr, brand.CLI)
	return 0
}

// runJSONMode implements `agt run --json`. Output is one JSON
// object per line — every streamed event as `{"type":"event","event":{...}}`
// (matching the control-plane Response shape), then a final
// `{"type":"result","result":{...}}` line. This is JSON-Lines /
// ndjson — operators pipe into `jq -c` for filtering, or read
// line-by-line in any language.
//
// Ephemeral token events (KindLLMToken) are emitted alongside
// journaled events; consumers that only want stable data should
// filter `.event.seq > 0`. Including them keeps streaming
// consumers responsive (e.g. live UIs that mirror agt run).
func runJSONMode(ctx context.Context, c *controlplane.Client, intent string, stdout, stderr io.Writer) int {
	enc := json.NewEncoder(stdout)
	// Compact one-object-per-line is the convention for ndjson;
	// jq -c handles it natively. Using NewEncoder also flushes
	// after every Encode call, so each line streams as it arrives.
	result, err := c.Stream(ctx, controlplane.CmdRun, map[string]any{"intent": intent}, func(ev *event.Event) {
		_ = enc.Encode(map[string]any{"type": "event", "event": ev})
	})
	if err != nil {
		// Final line: error envelope. Exit 1 so CI scripts
		// distinguish failure from success.
		_ = enc.Encode(map[string]any{"type": "error", "error": err.Error()})
		return 1
	}
	_ = enc.Encode(map[string]any{"type": "result", "result": result})
	return 0
}

func cmdSimple(cmd string, args map[string]any, stdout, stderr io.Writer) int {
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, args)
	if err != nil {
		fmt.Fprintf(stderr, "%s %s: %v\n", brand.CLI, cmd, err)
		return 1
	}
	enc, _ := json.MarshalIndent(res, "", "  ")
	fmt.Fprintf(stdout, "%s\n", enc)
	return 0
}

func cmdJournal(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s journal: subcommand required (verify|tail)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "verify":
		return cmdSimple(controlplane.CmdJournalVerify, nil, stdout, stderr)
	case "tail":
		return cmdJournalTail(args[1:], stdout, stderr)
	case "grep":
		return cmdJournalGrep(args[1:], stdout, stderr)
	case "head":
		return cmdJournalHead(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s journal: unknown subcommand %q (verify|tail|grep|head)\n", brand.CLI, args[0])
		return 2
	}
}

func cmdApprovals(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s approvals [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "list pending HITL approval requests\n")
			fmt.Fprintf(stdout, "  --json   emit the full pending array (CI/automation pipelines)\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s approvals: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdApprovals, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s approvals: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		// Pass the full response shape through — exits 0 even when
		// pending is empty (the empty array is the correct, valid
		// machine answer; jq pipelines should not need to special-
		// case "no approvals" via stderr scraping).
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	pending, _ := res["pending"].([]any)
	if len(pending) == 0 {
		fmt.Fprintf(stdout, "no pending approvals\n")
		return 0
	}
	fmt.Fprintf(stdout, "%d pending approval(s):\n", len(pending))
	for _, raw := range pending {
		m, _ := raw.(map[string]any)
		fmt.Fprintf(stdout, "\n  id         : %v\n", m["id"])
		fmt.Fprintf(stdout, "  capability : %v\n", m["capability"])
		fmt.Fprintf(stdout, "  tool       : %v\n", m["tool_name"])
		fmt.Fprintf(stdout, "  reason     : %v\n", m["reason"])
		fmt.Fprintf(stdout, "  actor      : %v\n", m["actor"])
		fmt.Fprintf(stdout, "  input      : %v\n", m["input"])
		fmt.Fprintf(stdout, "  timeout    : unix %v\n", m["timeout_unix"])
	}
	fmt.Fprintf(stdout, "\nResolve with: %s approve <id> [reason]  |  %s deny <id> [reason]\n", brand.CLI, brand.CLI)
	return 0
}

func cmdPlan(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s plan: subcommand required (generate|run|<file.json>)\n", brand.CLI)
		fmt.Fprintf(stderr, "  generate \"<intent>\"  — ask the planner LLM for a plan; print JSON\n")
		fmt.Fprintf(stderr, "  run      \"<intent>\"  — generate, then execute the plan\n")
		fmt.Fprintf(stderr, "  <file.json>          — execute a hand-authored plan\n")
		return 2
	}
	switch args[0] {
	case "generate", "gen":
		return cmdPlanGenerate(args[1:], stdout, stderr)
	case "run":
		return cmdPlanRun(args[1:], stdout, stderr)
	case "cost":
		return cmdPlanCost(args[1:], stdout, stderr)
	case "refine":
		return cmdPlanRefine(args[1:], stdout, stderr)
	case "validate":
		return cmdPlanValidate(args[1:], stdout, stderr)
	case "visualize", "viz":
		return cmdPlanVisualize(args[1:], stdout, stderr)
	default:
		// Backwards-compatible: `agt plan <file.json>` still executes
		// a hand-authored plan. Detected by checking if the arg is a
		// file path (existence test); falls through to the historical
		// CmdPlan handler.
		return cmdPlanExecuteFile(args, stdout, stderr)
	}
}

func cmdPlanExecuteFile(args []string, stdout, stderr io.Writer) int {
	body, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "%s plan: read %s: %v\n", brand.CLI, args[0], err)
		return 1
	}
	return runPlanJSON(string(body), stdout, stderr)
}

// cmdPlanGenerate runs `agt plan generate "<intent>"` — calls the
// daemon's CmdPlanGenerate and prints the JSON to stdout. Operator
// can pipe to a file (`agt plan generate "X" > plan.json`) or to
// jq for inspection (`agt plan generate "X" | jq`).
func cmdPlanGenerate(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || strings.TrimSpace(strings.Join(args, " ")) == "" {
		fmt.Fprintf(stderr, "%s plan generate: intent required (quote it as one argument)\n", brand.CLI)
		return 2
	}
	intent := strings.Join(args, " ")
	c := dial(stderr)
	if c == nil {
		return 1
	}
	// Planner is one LLM call; even a slow model fits in 2 minutes.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdPlanGenerate, map[string]any{"intent": intent})
	if err != nil {
		fmt.Fprintf(stderr, "%s plan generate: %v\n", brand.CLI, err)
		return 1
	}
	planJSON, _ := res["plan_json"].(string)
	if planJSON == "" {
		fmt.Fprintf(stderr, "%s plan generate: daemon returned empty plan_json\n", brand.CLI)
		return 1
	}
	fmt.Fprintln(stdout, planJSON)
	return 0
}

// cmdPlanRun is `agt plan run "<intent>"` — generate then execute
// in one operator-facing command. The Generate call returns the
// JSON; we then forward it to CmdPlan via the same machinery
// `agt plan <file>` uses. Keeps server endpoints single-purpose.
//
// Flags:
//
//	--dry-run            preview only: generate + validate + visualize
//	                     (+ cost if --model set), then exit without
//	                     executing. CI / human-review workflow.
//	--model <id>         cost-estimation model id (only meaningful
//	                     with --dry-run)
func cmdPlanRun(args []string, stdout, stderr io.Writer) int {
	dryRun := false
	model := ""
	var intentParts []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--dry-run":
			dryRun = true
		case "--model", "-m":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s plan run: --model needs a value\n", brand.CLI)
				return 2
			}
			model = args[i]
		default:
			intentParts = append(intentParts, a)
		}
	}
	intent := strings.TrimSpace(strings.Join(intentParts, " "))
	if intent == "" {
		fmt.Fprintf(stderr, "%s plan run: intent required (quote it as one argument)\n", brand.CLI)
		return 2
	}
	if model != "" && !dryRun {
		// --model only makes sense in dry-run mode (execution uses
		// the governor's primary). Warn rather than fail — operator
		// intent is clear; ignoring silently would be worse.
		fmt.Fprintf(stderr, "%s plan run: --model is only used with --dry-run; ignored for execution\n", brand.CLI)
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdPlanGenerate, map[string]any{"intent": intent})
	if err != nil {
		fmt.Fprintf(stderr, "%s plan run (generate): %v\n", brand.CLI, err)
		return 1
	}
	planJSON, _ := res["plan_json"].(string)
	nodeCount, _ := res["node_count"].(float64)
	if planJSON == "" {
		fmt.Fprintf(stderr, "%s plan run: daemon returned empty plan_json\n", brand.CLI)
		return 1
	}

	if dryRun {
		return runDryRunPreview(planJSON, int(nodeCount), model, stdout, stderr)
	}

	fmt.Fprintf(stdout, "generated %d-node plan:\n%s\n\n--- executing ---\n", int(nodeCount), planJSON)
	return runPlanJSON(planJSON, stdout, stderr)
}

// runPlanJSON forwards a plan JSON to the daemon's CmdPlan and
// renders streamed events + the final result. Shared by the
// `agt plan <file>` (hand-authored) and `agt plan run` (generated)
// paths so both render identically.
func runPlanJSON(planJSON string, stdout, stderr io.Writer) int {
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	result, err := c.Stream(ctx, controlplane.CmdPlan,
		map[string]any{"plan_json": planJSON},
		func(ev *event.Event) {
			fmt.Fprintf(stdout, "  [evt seq=%d kind=%s subject=%s]\n", ev.Seq, ev.Kind, ev.Subject)
		})
	if err != nil {
		fmt.Fprintf(stderr, "%s plan: %v\n", brand.CLI, err)
		return 1
	}
	planID, _ := result["plan_id"].(string)
	outputs, _ := result["node_outputs"].(map[string]any)
	fmt.Fprintf(stdout, "\n--- plan completed ---\nplan_id: %s\n", planID)
	for id, out := range outputs {
		s, _ := out.(string)
		fmt.Fprintf(stdout, "\n[%s]\n%s\n", id, s)
	}
	return 0
}

func cmdCatalog(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s catalog: subcommand required: sync|list|discover\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "sync":
		callArgs := map[string]any{}
		if len(args) > 1 {
			callArgs["url"] = args[1]
		}
		return cmdSimple(controlplane.CmdCatalogSync, callArgs, stdout, stderr)
	case "discover":
		callArgs := map[string]any{}
		if len(args) > 1 {
			callArgs["endpoint"] = args[1]
		}
		return cmdSimple(controlplane.CmdCatalogDiscover, callArgs, stdout, stderr)
	case "list":
		return cmdCatalogList(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s catalog: unknown subcommand %q\n", brand.CLI, args[0])
		return 2
	}
}

func cmdCatalogList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s catalog list [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "list synced providers + models + pricing\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s catalog list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdCatalogList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s catalog list: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	providers, _ := res["providers"].([]any)
	syncedAt, _ := res["api_synced_at"].(string)
	source, _ := res["api_source_url"].(string)

	fmt.Fprintf(stdout, "%d providers (synced %s from %s)\n",
		len(providers), formatTime(syncedAt), source)

	for _, raw := range providers {
		p, _ := raw.(map[string]any)
		credentialed := p["credentialed"] == true
		credBadge := "[no creds]"
		if credentialed {
			credBadge = "[creds OK]"
		}
		fmt.Fprintf(stdout, "\n  %s  (%s, family=%s)  %s\n",
			p["id"], p["name"], p["family"], credBadge)
		if api, _ := p["api"].(string); api != "" {
			fmt.Fprintf(stdout, "    api  : %s\n", api)
		}
		if env, ok := p["env"].([]any); ok && len(env) > 0 {
			fmt.Fprintf(stdout, "    env  : ")
			for i, e := range env {
				if i > 0 {
					fmt.Fprint(stdout, ", ")
				}
				fmt.Fprint(stdout, e)
			}
			fmt.Fprintln(stdout)
		}
		models, _ := p["models"].([]any)
		if len(models) == 0 {
			fmt.Fprintf(stdout, "    (no models)\n")
			continue
		}
		fmt.Fprintf(stdout, "    %d model(s):\n", len(models))
		for _, mraw := range models {
			m, _ := mraw.(map[string]any)
			cost := "free"
			if in, ok := m["cost_input_usd_per_mtok"].(float64); ok {
				out, _ := m["cost_output_usd_per_mtok"].(float64)
				cost = fmt.Sprintf("$%.2f / $%.2f per MTok", in, out)
			}
			fmt.Fprintf(stdout, "      %-40s  %s\n", m["id"], cost)
		}
	}
	return 0
}

func formatTime(s string) string {
	if s == "" || strings.HasPrefix(s, "0001-") {
		return "never"
	}
	return s
}

func cmdDecide(decision string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s %s: id required\n", brand.CLI, decision)
		return 2
	}
	id := args[0]
	reason := strings.Join(args[1:], " ")
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdDecide, map[string]any{
		"id": id, "decision": decision, "reason": reason,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%s %s: %v\n", brand.CLI, decision, err)
		return 1
	}
	enc, _ := json.MarshalIndent(res, "", "  ")
	fmt.Fprintf(stdout, "%s\n", enc)
	return 0
}
