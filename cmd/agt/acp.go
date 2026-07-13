// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/acp"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
)

// cmdACP runs the Agent Client Protocol bridge (SPEC-15 §3): it speaks ACP
// JSON-RPC 2.0 on stdin/stdout so an IDE (Zed, …) can drive Agezt as an agent
// backend. Each prompt is forwarded to the running daemon over the control
// plane as a normal `run`, so it passes through the same tool-loop + Edict +
// journal — the editor does not bypass governance.
//
// The IDE spawns this process; it is not interactive for humans. Run it as the
// editor's configured ACP agent command: `agt acp`.
func cmdACP(args []string, stdout, stderr io.Writer) int {
	if len(args) >= 1 && args[0] == "agents" {
		return cmdACPAgents(args[1:], stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "config" {
		return cmdACPConfig(args[1:], stdout, stderr)
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintf(stdout, "usage: %s acp [--tenant <id>]\n", brand.CLI)
		fmt.Fprintf(stdout, "       %s acp agents [--json]\n", brand.CLI)
		fmt.Fprintf(stdout, "Agent Client Protocol server over stdio (JSON-RPC 2.0) — configure your IDE\n")
		fmt.Fprintf(stdout, "(e.g. Zed) to launch `%s acp` as its agent backend. Requires a running daemon.\n", brand.CLI)
		fmt.Fprintf(stdout, "  --tenant <id>   route every prompt to an isolated tenant kernel (daemon\n")
		fmt.Fprintf(stdout, "                  AGEZT_MULTITENANT=on); omit for the primary kernel.\n")
		fmt.Fprintf(stdout, "  agents          list ACP coding agents installed on this host\n")
		return 0
	}
	tenant := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--tenant":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s acp: --tenant requires an id\n", brand.CLI)
				return 2
			}
			tenant = args[i+1]
			i++
		default:
			fmt.Fprintf(stderr, "%s acp: unexpected argument %q\n", brand.CLI, args[i])
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	srv := acp.New(controlPlaneRunner{c: c, tenant: tenant}, os.Stdin, stdout)
	if err := srv.Serve(context.Background()); err != nil {
		fmt.Fprintf(stderr, "%s acp: %v\n", brand.CLI, err)
		return 1
	}
	return 0
}

// cmdACPConfig emits the IDE-facing configuration JSON so an editor (Zed, …)
// can auto-discover how to launch Agezt as its ACP agent backend. It does not
// require a running daemon — it's a static config emitter.
func cmdACPConfig(args []string, stdout, stderr io.Writer) int {
	tenant := ""
	asJSON := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--tenant":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s acp: --tenant requires an id\n", brand.CLI)
				return 2
			}
			tenant = args[i+1]
			i++
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s acp config --tenant <id> [--json]\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s acp config: unexpected argument %q\n", brand.CLI, args[i])
			return 2
		}
	}
	_ = asJSON // always JSON for now; flag reserved for future plain-text mode

	cfg := struct {
		Name    string   `json:"name"`
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}{
		Name:    brand.Name,
		Command: brand.CLI,
		Args:    []string{"acp", "--tenant", tenant},
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		fmt.Fprintf(stderr, "%s acp config: %v\n", brand.CLI, err)
		return 1
	}
	return 0
}

// cmdACPAgents lists the ACP coding agents discovered on the host (installed /
// missing / configured default) via the daemon's read-only inventory.
func cmdACPAgents(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		} else if a == "-h" || a == "--help" {
			fmt.Fprintf(stdout, "usage: %s acp agents [--json]\n", brand.CLI)
			return 0
		}
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdACPAgents, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s acp agents: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	if active, _ := res["active_command"].(string); active != "" {
		fmt.Fprintf(stdout, "default ACP command: %s\n", active)
	}
	agents, _ := res["agents"].([]any)
	if len(agents) == 0 {
		fmt.Fprintln(stdout, "no ACP agents in the catalog")
		return 0
	}
	for _, raw := range agents {
		a, _ := raw.(map[string]any)
		if a == nil {
			continue
		}
		mark := "  "
		if installed, _ := a["installed"].(bool); installed {
			mark = "✓ "
		} else {
			mark = "· "
		}
		fmt.Fprintf(stdout, "%s%-13s %s", mark, str(a["slug"]), str(a["name"]))
		if active, _ := a["active"].(bool); active {
			fmt.Fprintf(stdout, " [default]")
		}
		fmt.Fprintln(stdout)
		if installed, _ := a["installed"].(bool); installed {
			fmt.Fprintf(stdout, "     command: %s", str(a["command"]))
			if v := str(a["version"]); v != "" {
				fmt.Fprintf(stdout, "  (%s)", v)
			}
			fmt.Fprintln(stdout)
		} else if hint := str(a["install"]); hint != "" {
			fmt.Fprintf(stdout, "     not installed — install: %s\n", hint)
		}
	}
	installedN, _ := res["installed_count"].(float64)
	fmt.Fprintf(stdout, "%d of %d catalog agents installed\n", int(installedN), len(agents))
	return 0
}

// runStreamer is the slice of *controlplane.Client the ACP runner needs. An
// interface keeps the tenant-arg forwarding unit-testable with a fake (no daemon).
type runStreamer interface {
	Stream(ctx context.Context, cmd string, args map[string]any, onEvent func(*event.Event)) (map[string]any, error)
}

// controlPlaneRunner adapts the control-plane streaming client to acp.Runner:
// one ACP prompt becomes one streamed `run`, with llm.token events relayed as
// ACP message chunks.
type controlPlaneRunner struct {
	c      runStreamer
	tenant string // empty = primary kernel; else route the run to this tenant
}

func (r controlPlaneRunner) Prompt(ctx context.Context, cwd, intent string, onChunk func(acp.ChunkKind, string)) (string, error) {
	runArgs := map[string]any{"intent": intent}
	if r.tenant != "" {
		runArgs["tenant"] = r.tenant
	}
	res, err := r.c.Stream(ctx, controlplane.CmdRun, runArgs, func(ev *event.Event) {
		// Relay answer tokens as message chunks and reasoning deltas (M322) as
		// thought chunks; the ACP server maps each to the matching sessionUpdate.
		var kind acp.ChunkKind
		switch ev.Kind {
		case event.KindLLMToken:
			kind = acp.ChunkMessage
		case event.KindLLMReasoning:
			kind = acp.ChunkThought
		default:
			return
		}
		var p struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(ev.Payload, &p) == nil && p.Text != "" {
			onChunk(kind, p.Text)
		}
	})
	if err != nil {
		return "", err
	}
	answer, _ := res["answer"].(string)
	return answer, nil
}
