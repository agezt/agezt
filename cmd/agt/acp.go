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
		fmt.Fprintf(stdout, "usage: %s acp\n", brand.CLI)
		fmt.Fprintf(stdout, "Agent Client Protocol server over stdio (JSON-RPC 2.0) — configure your IDE\n")
		fmt.Fprintf(stdout, "(e.g. Zed) to launch `%s acp` as its agent backend. Requires a running daemon.\n", brand.CLI)
		return 0
	}
	if len(args) > 0 {
		fmt.Fprintf(stderr, "%s acp: unexpected argument %q\n", brand.CLI, args[0])
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	srv := acp.New(controlPlaneRunner{c: c}, os.Stdin, stdout)
	if err := srv.Serve(context.Background()); err != nil {
		fmt.Fprintf(stderr, "%s acp: %v\n", brand.CLI, err)
		return 1
	}
	return 0
}

// controlPlaneRunner adapts the control-plane streaming client to acp.Runner:
// one ACP prompt becomes one streamed `run`, with llm.token events relayed as
// ACP message chunks.
type controlPlaneRunner struct {
	c *controlplane.Client
}

func (r controlPlaneRunner) Prompt(ctx context.Context, cwd, intent string, onChunk func(string)) (string, error) {
	res, err := r.c.Stream(ctx, controlplane.CmdRun, map[string]any{"intent": intent}, func(ev *event.Event) {
		if ev.Kind != event.KindLLMToken {
			return
		}
		var p struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(ev.Payload, &p) == nil && p.Text != "" {
			onChunk(p.Text)
		}
	})
	if err != nil {
		return "", err
	}
	answer, _ := res["answer"].(string)
	return answer, nil
}
