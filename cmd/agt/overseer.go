// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdOverseer dispatches `agt overseer <subcommand>` — the CLI gateway to the
// same fleet supervisory operations the agent-facing overseer tool provides.
// It mirrors the tool's ops through the daemon's control-plane RPC so operators
// have the same 19-command palette on the terminal.
func cmdOverseer(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return overseerUsage(stderr)
	}
	switch strings.TrimSpace(args[0]) {
	case "status":
		return overseerStatus(stdout, stderr)
	case "agents":
		return overseerAgents(stdout, stderr)
	case "runs":
		return overseerRuns(stdout, stderr)
	case "cancel":
		return overseerCancel(args[1:], stdout, stderr)
	case "halt":
		return overseerHalt(stdout, stderr)
	case "resume":
		return overseerResume(stdout, stderr)
	case "pause":
		return overseerPauseResume(args[1:], stdout, stderr, false)
	case "unpause":
		return overseerPauseResume(args[1:], stdout, stderr, true)
	case "impact":
		return overseerAgentImpact(args[1:], stdout, stderr)
	case "retire":
		return overseerRetire(args[1:], stdout, stderr)
	case "revive":
		return overseerRevive(args[1:], stdout, stderr)
	case "get":
		return overseerGet(args[1:], stdout, stderr)
	case "delete", "rm":
		return overseerDelete(args[1:], stdout, stderr)
	case "bulk":
		return overseerBulk(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		return overseerUsage(stdout)
	default:
		fmt.Fprintf(stderr, "%s overseer: unknown subcommand %q\n", brand.CLI, args[0])
		return 2
	}
}

func overseerUsage(w io.Writer) int {
	fmt.Fprintf(w, `usage: %s overseer <subcommand> [args]

Fleet supervisory commands — mirror the agent-facing overseer tool.

Read:
  status                              daemon health summary
  agents                              list all agents
  runs                                active runs
  get <slug|id>                       full agent profile

Act:
  cancel <corr-id>                    cancel an active run
  halt [reason]                       stop all runs (freeze)
  resume [reason]                     unfreeze
  pause <slug>                        disable an agent
  unpause <slug>                      enable an agent
  retire <slug> [reason]              move an agent to the graveyard
  revive <slug>                       restore from graveyard
  delete|rm <slug>                    permanently remove an agent (irreversible)

Batch:
  bulk pause|unpause <slug1,slug2>    pause/resume multiple agents
  bulk retire|revive <slug1,slug2>    retire/revive multiple agents
  bulk delete <slug1,slug2>           delete multiple agents (irreversible)

Diagnose:
  impact <slug>                       what depends on the agent
  help                                this message
`, brand.CLI)
	return 0
}

func overseerStatus(stdout, stderr io.Writer) int {
	c := dial(stderr)
	if c == nil {
		return 1
	}
	res, err := c.Call(nil, controlplane.CmdStatus, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Fprintln(stdout, string(b))
	return 0
}

func overseerAgents(stdout, stderr io.Writer) int {
	c := dial(stderr)
	if c == nil {
		return 1
	}
	res, err := c.Call(nil, controlplane.CmdAgentList, map[string]any{})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Fprintln(stdout, string(b))
	return 0
}

func overseerRuns(stdout, stderr io.Writer) int {
	c := dial(stderr)
	if c == nil {
		return 1
	}
	res, err := c.Call(nil, controlplane.CmdStatus, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Fprintln(stdout, string(b))
	return 0
}

func overseerHalt(stdout, stderr io.Writer) int {
	c := dial(stderr)
	if c == nil {
		return 1
	}
	res, err := c.Call(nil, controlplane.CmdHalt, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "%s\n", jsonOrString(res))
	return 0
}

func overseerResume(stdout, stderr io.Writer) int {
	c := dial(stderr)
	if c == nil {
		return 1
	}
	res, err := c.Call(nil, controlplane.CmdResume, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "%s\n", jsonOrString(res))
	return 0
}

func overseerCancel(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintf(stderr, "%s overseer cancel: requires a correlation id\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	res, err := c.Call(nil, controlplane.CmdCancelRun, map[string]any{"correlation": strings.TrimSpace(args[0])})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "%s\n", jsonOrString(res))
	return 0
}

func overseerPauseResume(args []string, stdout, stderr io.Writer, enabled bool) int {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintf(stderr, "%s overseer %s: requires an agent slug\n", brand.CLI, map[bool]string{true: "unpause", false: "pause"}[enabled])
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	res, err := c.Call(nil, controlplane.CmdAgentSetEnabled, map[string]any{"ref": strings.TrimSpace(args[0]), "enabled": enabled})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	if slug, _ := res["slug"].(string); slug != "" {
		fmt.Fprintf(stdout, "agent %s %s\n", slug, map[bool]string{true: "resumed", false: "paused"}[enabled])
	}
	return 0
}

func overseerAgentImpact(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintf(stderr, "%s overseer impact: requires an agent slug\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	res, err := c.Call(nil, controlplane.CmdAgentImpact, map[string]any{"ref": strings.TrimSpace(args[0])})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Fprintln(stdout, string(b))
	return 0
}

func overseerRetire(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintf(stderr, "%s overseer retire: requires an agent slug\n", brand.CLI)
		return 2
	}
	ref := strings.TrimSpace(args[0])
	reason := strings.Join(args[1:], " ")
	c := dial(stderr)
	if c == nil {
		return 1
	}
	res, err := c.Call(nil, controlplane.CmdAgentRetire, map[string]any{"ref": ref, "reason": reason})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	if slug, _ := res["slug"].(string); slug != "" {
		fmt.Fprintf(stdout, "agent %s retired\n", slug)
	}
	return 0
}

func overseerRevive(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintf(stderr, "%s overseer revive: requires an agent slug\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	res, err := c.Call(nil, controlplane.CmdAgentRevive, map[string]any{"ref": strings.TrimSpace(args[0])})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	if slug, _ := res["slug"].(string); slug != "" {
		fmt.Fprintf(stdout, "agent %s revived\n", slug)
	}
	return 0
}

func overseerGet(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintf(stderr, "%s overseer get: requires an agent slug or id\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	res, err := c.Call(nil, controlplane.CmdAgentList, map[string]any{"ref": strings.TrimSpace(args[0])})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Fprintln(stdout, string(b))
	return 0
}

func overseerDelete(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintf(stderr, "%s overseer delete: requires an agent slug\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	res, err := c.Call(nil, controlplane.CmdAgentRemove, map[string]any{"ref": strings.TrimSpace(args[0])})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	if removed, _ := res["removed"].(bool); removed {
		fmt.Fprintf(stdout, "agent %s removed\n", args[0])
	} else {
		fmt.Fprintf(stderr, "%s overseer delete: unknown agent %q\n", brand.CLI, args[0])
		return 1
	}
	return 0
}

func overseerBulk(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintf(stderr, "%s overseer bulk: requires an action (pause|unpause|retire|revive|delete) and comma-separated slugs\n", brand.CLI)
		return 2
	}
	action := strings.TrimSpace(args[0])
	slugs := strings.Split(strings.TrimSpace(args[1]), ",")
	if len(slugs) == 0 || (len(slugs) == 1 && slugs[0] == "") {
		fmt.Fprintf(stderr, "%s overseer bulk: requires at least one slug\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	var cpCmd string
	var payload map[string]any
	switch action {
	case "pause":
		cpCmd = controlplane.CmdAgentSetEnabled
		payload = map[string]any{"refs": slugs, "enabled": false}
	case "unpause":
		cpCmd = controlplane.CmdAgentSetEnabled
		payload = map[string]any{"refs": slugs, "enabled": true}
	case "retire":
		cpCmd = controlplane.CmdAgentRetire
		payload = map[string]any{"refs": slugs}
	case "revive":
		cpCmd = controlplane.CmdAgentRevive
		payload = map[string]any{"refs": slugs}
	case "delete", "rm":
		cpCmd = controlplane.CmdAgentRemove
		payload = map[string]any{"refs": slugs}
	default:
		fmt.Fprintf(stderr, "%s overseer bulk: unknown action %q (pause|unpause|retire|revive|delete)\n", brand.CLI, action)
		return 2
	}
	res, err := c.Call(nil, cpCmd, payload)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Fprintln(stdout, string(b))
	return 0
}

func jsonOrString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
