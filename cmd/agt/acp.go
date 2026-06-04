// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

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
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintf(stdout, "usage: %s acp [--tenant <id>]\n", brand.CLI)
		fmt.Fprintf(stdout, "Agent Client Protocol server over stdio (JSON-RPC 2.0) — configure your IDE\n")
		fmt.Fprintf(stdout, "(e.g. Zed) to launch `%s acp` as its agent backend. Requires a running daemon.\n", brand.CLI)
		fmt.Fprintf(stdout, "  --tenant <id>   route every prompt to an isolated tenant kernel (daemon\n")
		fmt.Fprintf(stdout, "                  AGEZT_MULTITENANT=on); omit for the primary kernel.\n")
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
