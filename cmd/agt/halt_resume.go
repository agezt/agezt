// SPDX-License-Identifier: MIT

package main

// `agt halt [--reason "..."] [--json]` and the symmetric resume.
// Halt cancels every in-flight run and refuses new ones until
// resume. Recording a reason is optional but encouraged — it
// shows up on the kernel.halt journal event so a postmortem can
// answer "why was the daemon halted at 14:32?".

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdHaltResume implements both `agt halt` and `agt resume`. The
// shapes are identical (same flags, same response), so one handler
// keeps the surface symmetric.
func cmdHaltResume(action string, args []string, stdout, stderr io.Writer) int {
	if action != "halt" && action != "resume" {
		// Programmer error — caller passes the literal string.
		fmt.Fprintf(stderr, "%s: internal: unknown action %q\n", brand.CLI, action)
		return 1
	}

	asJSON := false
	var reasonParts []string
	expectingReason := false
	for _, a := range args {
		if expectingReason {
			reasonParts = append(reasonParts, a)
			expectingReason = false
			continue
		}
		switch a {
		case "--json":
			asJSON = true
		case "--reason", "-r":
			expectingReason = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s %s [--reason \"...\"] [--json]\n", brand.CLI, action)
			if action == "halt" {
				fmt.Fprintf(stdout, "cancel every in-flight run and refuse new ones; reason is journaled\n")
			} else {
				fmt.Fprintf(stdout, "clear the halt flag (already-cancelled runs stay cancelled); reason is journaled\n")
			}
			return 0
		default:
			fmt.Fprintf(stderr, "%s %s: unexpected arg %q\n", brand.CLI, action, a)
			return 2
		}
	}
	if expectingReason {
		fmt.Fprintf(stderr, "%s %s: --reason needs a value\n", brand.CLI, action)
		return 2
	}
	reason := strings.Join(reasonParts, " ")

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := controlplane.CmdHalt
	if action == "resume" {
		cmd = controlplane.CmdResume
	}
	callArgs := map[string]any{}
	if reason != "" {
		callArgs["reason"] = reason
	}
	res, err := c.Call(ctx, cmd, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s %s: %v\n", brand.CLI, action, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	if action == "halt" {
		fmt.Fprintf(stdout, "halted (reason: %s)\n", reasonOrPlaceholder(reason))
	} else {
		fmt.Fprintf(stdout, "resumed (reason: %s)\n", reasonOrPlaceholder(reason))
	}
	return 0
}

func reasonOrPlaceholder(r string) string {
	if r == "" {
		return "—"
	}
	return r
}
