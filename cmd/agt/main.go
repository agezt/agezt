// SPDX-License-Identifier: MIT
//
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
//
// This file holds the entry point (main + run) + resolveRunIntent +
// cmdRun (the big `agt run` sub-command). The other sub-commands
// (`journal`, `approvals`, `plan`, `catalog`, `decide`, `simple`)
// live in main_run_modes.go and main_approvals_plan.go.
// Split from main.go during Day 211 god-file refactor (#42).
// Public API unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/assure"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printHelp(stdout)
		return 0
	}
	// Uniform `-h` (M936): `agt <cmd> -h|--help` answers from the help table
	// for EVERY command, before the command's own code runs. This is a safety
	// property, not just consistency — commands that treat their first arg as
	// data would otherwise EXECUTE on "-h" (`agt run -h` used to send "-h" to
	// the live agent as an intent and bill a completion for it).
	if len(args) >= 2 && (args[1] == "-h" || args[1] == "--help") && helpHas(args[0]) {
		return cmdHelp(args[:1], stdout, stderr)
	}
	switch args[0] {
	case "-v", "--version", "version":
		fmt.Fprintf(stdout, "%s %s (protocol v%d)\n", brand.CLI, brand.Version, brand.ProtocolVersion)
		return 0
	case "-h", "--help", "help":
		return cmdHelp(args[1:], stdout, stderr)
	default:
		return ExecuteCommand(args[0], args[1:], stdout, stderr)
	}
}

// dial and dialBase were 3-line shims that delegated to
// cmd/agt/dial. They existed only during the rollout of the
// dial package (Day 5 → Day 7); the bulk rewrite completed in
// Day 7 and the shims were deleted. Code outside cmd/agt that
// needs a control-plane client uses cmd/agt/dial.New directly;
// the legacy bare `dialpkg.New(stderr)` and `dialpkg.NewAtBase(base, stderr)`
// calls in cmd/agt/ are gone.

// resolveRunIntent resolves the run intent from positional parts, an optional
// --file path, or stdin. Precedence: --file (read the file); else if the sole
// positional argument is "-" read all of stdin (pipe convention); else the joined
// positional text. All trimmed. Lets long/multi-line prompts come from a file or a
// pipe instead of being quoted on the command line.
func resolveRunIntent(parts []string, file string, stdin io.Reader) (string, error) {
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read --file %q: %w", file, err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	joined := strings.TrimSpace(strings.Join(parts, " "))
	if joined == "-" {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read intent from stdin: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return joined, nil
}

func cmdRun(args []string, stdout, stderr io.Writer) int {
	// Strip flags early so they work in any position: `agt run "..." --json`
	// or `agt run --json "..."` both compose. --tenant <id> routes the run to
	// an isolated tenant's kernel (requires the daemon with AGEZT_MULTITENANT=on).
	asJSON := false
	quiet := false
	tenant := ""
	model := ""
	agentRef := ""
	system := ""
	file := ""
	timeout := ""
	execProfile := ""
	remotePeer := ""
	toolsSet := false
	var toolsList []any
	dryRun := false
	maxCostMicrocents := int64(0)
	assureAttempts := int64(0) // 0 = single pass; >0 = "do-it-for-sure" retry budget
	var images []string
	var intentParts []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--quiet" || a == "-q":
			quiet = true
		case a == "--tenant":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s run: --tenant needs an id\n", brand.CLI)
				return 2
			}
			i++
			tenant = args[i]
		case a == "--model":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s run: --model needs a model id\n", brand.CLI)
				return 2
			}
			i++
			model = args[i]
		case strings.HasPrefix(a, "--model="):
			model = strings.TrimPrefix(a, "--model=")
		case a == "--agent":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s run: --agent needs an agent slug (agt agent list)\n", brand.CLI)
				return 2
			}
			i++
			agentRef = args[i]
		case strings.HasPrefix(a, "--agent="):
			agentRef = strings.TrimPrefix(a, "--agent=")
		case a == "--system":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s run: --system needs a prompt\n", brand.CLI)
				return 2
			}
			i++
			system = args[i]
		case strings.HasPrefix(a, "--system="):
			system = strings.TrimPrefix(a, "--system=")
		case a == "--timeout":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s run: --timeout needs a duration (e.g. 30s, 2m)\n", brand.CLI)
				return 2
			}
			i++
			timeout = args[i]
		case strings.HasPrefix(a, "--timeout="):
			timeout = strings.TrimPrefix(a, "--timeout=")
		case a == "--exec-profile" || a == "--execution-profile":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s run: %s needs a profile id (local, warden, docker, ssh, k8s, modal, daytona, or remote-agezt)\n", brand.CLI, a)
				return 2
			}
			i++
			execProfile = args[i]
		case strings.HasPrefix(a, "--exec-profile="):
			execProfile = strings.TrimPrefix(a, "--exec-profile=")
		case strings.HasPrefix(a, "--execution-profile="):
			execProfile = strings.TrimPrefix(a, "--execution-profile=")
		case a == "--peer" || a == "--remote-peer":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s run: %s needs a peer name\n", brand.CLI, a)
				return 2
			}
			i++
			remotePeer = args[i]
		case strings.HasPrefix(a, "--peer="):
			remotePeer = strings.TrimPrefix(a, "--peer=")
		case strings.HasPrefix(a, "--remote-peer="):
			remotePeer = strings.TrimPrefix(a, "--remote-peer=")
		case a == "--dry-run":
			dryRun = true
		case a == "--assure":
			assureAttempts = int64(assure.DefaultMaxAttempts)
		case strings.HasPrefix(a, "--assure="):
			v := strings.TrimPrefix(a, "--assure=")
			n, perr := strconv.Atoi(v)
			if perr != nil || n < 1 {
				fmt.Fprintf(stderr, "%s run: invalid --assure %q (want a positive integer of attempts)\n", brand.CLI, v)
				return 2
			}
			assureAttempts = int64(n)
		case a == "--max-cost" || strings.HasPrefix(a, "--max-cost="):
			var v string
			if a == "--max-cost" {
				if i+1 >= len(args) {
					fmt.Fprintf(stderr, "%s run: --max-cost needs an amount (e.g. 0.50 or $0.50)\n", brand.CLI)
					return 2
				}
				i++
				v = args[i]
			} else {
				v = strings.TrimPrefix(a, "--max-cost=")
			}
			mc, perr := parseUSDToMicrocents(v)
			if perr != nil {
				fmt.Fprintf(stderr, "%s run: invalid --max-cost %q (want a positive dollar amount like 0.50 or $0.50)\n", brand.CLI, v)
				return 2
			}
			maxCostMicrocents = mc
		case a == "--no-tools":
			toolsSet = true // empty list = no tools
		case a == "--tools" || strings.HasPrefix(a, "--tools="):
			var csv string
			if a == "--tools" {
				if i+1 >= len(args) {
					fmt.Fprintf(stderr, "%s run: --tools needs a comma-separated list\n", brand.CLI)
					return 2
				}
				i++
				csv = args[i]
			} else {
				csv = strings.TrimPrefix(a, "--tools=")
			}
			toolsSet = true
			for _, n := range strings.Split(csv, ",") {
				if n = strings.TrimSpace(n); n != "" {
					toolsList = append(toolsList, n)
				}
			}
		case a == "--image":
			// Attach an image to the run (M91). The daemon gates it against the
			// model's vision capability before any provider call. The CLI reads
			// the bytes here (the file lives on the operator's machine, not the
			// daemon's) and forwards a self-describing data: URL (M241).
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s run: --image needs a path\n", brand.CLI)
				return 2
			}
			i++
			du, err := loadImageDataURL(args[i])
			if err != nil {
				fmt.Fprintf(stderr, "%s run: --image %q: %v\n", brand.CLI, args[i], err)
				return 2
			}
			images = append(images, du)
		case strings.HasPrefix(a, "--image="):
			p := strings.TrimPrefix(a, "--image=")
			du, err := loadImageDataURL(p)
			if err != nil {
				fmt.Fprintf(stderr, "%s run: --image %q: %v\n", brand.CLI, p, err)
				return 2
			}
			images = append(images, du)
		case a == "--file":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s run: --file needs a path\n", brand.CLI)
				return 2
			}
			i++
			file = args[i]
		case strings.HasPrefix(a, "--file="):
			file = strings.TrimPrefix(a, "--file=")
		default:
			intentParts = append(intentParts, a)
		}
	}
	intent, err := resolveRunIntent(intentParts, file, os.Stdin)
	if err != nil {
		fmt.Fprintf(stderr, "%s run: %v\n", brand.CLI, err)
		return 2
	}
	if intent == "" {
		fmt.Fprintf(stderr, "%s run: intent required (quote it as one argument, pipe via `-`, or use --file)\n", brand.CLI)
		return 2
	}

	runArgs := map[string]any{"intent": intent}
	if tenant != "" {
		runArgs["tenant"] = tenant
	}
	if model = strings.TrimSpace(model); model != "" {
		runArgs["model"] = model
	}
	// Run AS a named agent (M783): the daemon resolves the roster profile and
	// applies its soul/model/cost ceiling as defaults (explicit flags win).
	if agentRef = strings.TrimSpace(agentRef); agentRef != "" {
		runArgs["agent"] = agentRef
	}
	if strings.TrimSpace(system) != "" {
		runArgs["system"] = system
	}
	if execProfile = strings.TrimSpace(execProfile); execProfile != "" {
		runArgs["execution_profile"] = execProfile
	}
	if remotePeer = strings.TrimSpace(remotePeer); remotePeer != "" {
		if !strings.EqualFold(execProfile, "remote-agezt") {
			fmt.Fprintf(stderr, "%s run: --peer/--remote-peer requires --exec-profile remote-agezt\n", brand.CLI)
			return 2
		}
		runArgs["remote_peer"] = remotePeer
	}
	// Per-run wall-clock timeout (M154): validated client-side for fast feedback,
	// passed to the server which enforces it on the run.
	clientTimeout := 15 * time.Minute
	if timeout = strings.TrimSpace(timeout); timeout != "" {
		d, perr := time.ParseDuration(timeout)
		if perr != nil || d <= 0 {
			fmt.Fprintf(stderr, "%s run: invalid --timeout %q (want a positive Go duration like 30s, 2m)\n", brand.CLI, timeout)
			return 2
		}
		runArgs["timeout"] = timeout
		// Keep the client connection open at least as long as the run may take.
		if d+30*time.Second > clientTimeout {
			clientTimeout = d + 30*time.Second
		}
	}
	if len(images) > 0 {
		imgs := make([]any, len(images))
		for i, n := range images {
			imgs[i] = n
		}
		runArgs["images"] = imgs
	}
	// Per-run tool restriction (M158): --tools <csv> limits this run to the named
	// tools; --no-tools disables tools entirely. An explicit empty allow-list
	// (toolsSet with no names) is distinct from omitting the arg (full toolset).
	if toolsSet {
		if toolsList == nil {
			toolsList = []any{}
		}
		runArgs["tools"] = toolsList
	}
	// Dry-run (M159): resolve and print what this run WOULD do — effective model,
	// system source, timeout, and the exact tool set after the per-run filter —
	// without starting it or spending a token.
	if dryRun {
		runArgs["dry_run"] = true
	}
	// Per-run cost cap (M166): bound this run's spend. Sent in microcents.
	if maxCostMicrocents > 0 {
		runArgs["max_cost"] = float64(maxCostMicrocents)
	}
	// "Do-it-for-sure" (M651): run, verify completion, retry the gap — up to this
	// many attempts. The daemon runs the loop and streams every attempt's events.
	if assureAttempts > 0 {
		runArgs["assure"] = float64(assureAttempts)
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), clientTimeout)
	defer cancel()

	// Dry-run (M159): a single non-streaming Call returns the resolved plan; no
	// events, no run. Render it (or emit the raw JSON plan with --json).
	if dryRun {
		return runDryRunMode(ctx, c, runArgs, asJSON, stdout, stderr)
	}

	if asJSON {
		return runJSONMode(ctx, c, runArgs, stdout, stderr)
	}

	// Stream-aware renderer: KindLLMToken events carry partial text
	// the model is generating live (ephemeral; never journaled — see
	// bus.PublishStreaming). Render those inline so the operator sees
	// progress instead of a frozen prompt. Every other event keeps
	// the existing `[evt seq=...]` summary line.
	// Stream renderer state. The answer tokens (KindLLMToken) stream inline; a
	// reasoning model's chain-of-thought (KindLLMReasoning) is HIDDEN by default —
	// it's high-rate ephemeral noise (the "[evt seq=0 kind=llm.reasoning]" flood)
	// that the operator rarely wants — and shown, demarcated with 💭, only when
	// AGEZT_SHOW_REASONING=1. streamMode tracks which inline stream is open so we
	// emit a clean newline when switching between thinking, answer, and the
	// per-event summary lines.
	showReasoning := os.Getenv(brand.EnvPrefix+"SHOW_REASONING") == "1"
	streamMode := "" // "", "answer", "reason"
	closeStream := func() {
		if streamMode != "" {
			fmt.Fprintln(stdout)
			streamMode = ""
		}
	}
	streamText := func(mode, prefix, text string) {
		if text == "" {
			return
		}
		if streamMode != mode {
			if streamMode != "" {
				fmt.Fprintln(stdout)
			}
			fmt.Fprint(stdout, prefix)
			streamMode = mode
		}
		fmt.Fprint(stdout, text)
	}
	// --quiet (M156): suppress the live token stream + per-event lines + the
	// usage/correlation footer, printing ONLY the final answer — so scripts can
	// `agt run -q --file spec.md > answer.txt` and get clean output.
	result, err := c.Stream(ctx, controlplane.CmdRun, runArgs, func(ev *event.Event) {
		if quiet {
			return
		}
		switch ev.Kind {
		case event.KindLLMToken:
			var p struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(ev.Payload, &p)
			streamText("answer", "  ", p.Text)
			return
		case event.KindLLMReasoning:
			if !showReasoning {
				return // ephemeral thinking; opt in with AGEZT_SHOW_REASONING=1
			}
			var p struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(ev.Payload, &p)
			streamText("reason", "  💭 ", p.Text)
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
	if quiet {
		fmt.Fprintln(stdout, ans)
		return 0
	}
	fmt.Fprintf(stdout, "\n--- final answer ---\n%s\n", ans)
	fmt.Fprintf(stdout, "(correlation_id: %s; use `%s why <event_id>` to walk the chain)\n", corr, brand.CLI)
	// Usage summary (M146): model · iterations · cost, when the run journaled them
	// (omitted for an unpriced run such as the offline mock).
	var bits []string
	if m, _ := result["model"].(string); m != "" {
		bits = append(bits, m)
	}
	if it := intOfStatus(result["iters"]); it > 0 {
		bits = append(bits, fmt.Sprintf("%d iteration(s)", it))
	}
	if mc := mcFromAny(result["spent_mc"]); mc > 0 {
		bits = append(bits, fmtUSD(mc))
	}
	if len(bits) > 0 {
		fmt.Fprintf(stdout, "usage: %s\n", strings.Join(bits, " · "))
	}
	return 0
}
