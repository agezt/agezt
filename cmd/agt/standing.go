// SPDX-License-Identifier: MIT

// agt standing command: dispatcher + Edit/Why/List + render/init helpers.
// Code extracted from standing.go during the Day-106 god-file split.
// Public API unchanged.
package main


import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)


// cmdStanding dispatches `agt standing <subcommand>` — the management surface for
// durable event/cron wake rules. Standing orders are triggers, not agent
// identities; every mutation is journaled (standing.*) so it's auditable like any
// other event.
func cmdStanding(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return standingUsage(stderr)
	}
	switch args[0] {
	case "list":
		return cmdStandingList(args[1:], stdout, stderr)
	case "add":
		return cmdStandingAdd(args[1:], stdout, stderr)
	case "edit", "set":
		return cmdStandingEdit(args[1:], stdout, stderr)
	case "pause":
		return cmdStandingSetEnabled(args[1:], stdout, stderr, false)
	case "resume":
		return cmdStandingSetEnabled(args[1:], stdout, stderr, true)
	case "remove", "rm":
		return cmdStandingRemove(args[1:], stdout, stderr)
	case "why":
		return cmdStandingWhy(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		return standingUsage(stdout)
	default:
		fmt.Fprintf(stderr, "%s standing: unknown subcommand %q\n", brand.CLI, args[0])
		return standingUsage(stderr)
	}
}

func standingUsage(w io.Writer) int {
	fmt.Fprintf(w, "usage: %s standing <list|add|edit|pause|resume|remove>\n", brand.CLI)
	fmt.Fprintf(w, "  list [--json]                                  show standing wake rules\n")
	fmt.Fprintf(w, "  add --name N (--cron \"SCHED\" | --event SUBJ) [--plan TEXT]\n")
	fmt.Fprintf(w, "      [--agent SLUG]  run each firing AS that named agent (soul/model/memory/budget)\n")
	fmt.Fprintf(w, "      [--mode inform_only|ask|act_or_ask] [--max-trust L0..L4] [--budget USD]\n")
	fmt.Fprintf(w, "      [--scope ent1,ent2] [--channel C] [--cooldown 15m]\n")
	fmt.Fprintf(w, "  edit <id> [--name N] [--plan TEXT] [--agent SLUG|--clear-agent]\n")
	fmt.Fprintf(w, "      [--mode inform_only|ask|act_or_ask] [--max-trust L0..L4] [--assure N] [--cooldown 15m]\n")
	fmt.Fprintf(w, "  pause <id>                                     disable an order\n")
	fmt.Fprintf(w, "  resume <id>                                    re-enable an order\n")
	fmt.Fprintf(w, "  remove <id>                                    delete an order\n")
	fmt.Fprintf(w, "  why <id> [--json]                              an order's life story (fires, pauses)\n")
	return 0
}

func cmdStandingEdit(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintf(stderr, "usage: %s standing edit <id> [--name N] [--plan TEXT] [--agent SLUG|--clear-agent] [--mode MODE] [--max-trust L0..L4] [--assure N] [--cooldown 15m]\n", brand.CLI)
		return 2
	}
	payload := map[string]any{"id": args[0]}
	changed := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--name":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s standing edit: --name needs a value\n", brand.CLI)
				return 2
			}
			payload["name"] = args[i]
			changed = true
		case "--plan":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s standing edit: --plan needs a value\n", brand.CLI)
				return 2
			}
			payload["plan"] = args[i]
			changed = true
		case "--agent":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s standing edit: --agent needs a value\n", brand.CLI)
				return 2
			}
			payload["agent"] = args[i]
			changed = true
		case "--clear-agent":
			payload["agent"] = ""
			changed = true
		case "--mode":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s standing edit: --mode needs a value\n", brand.CLI)
				return 2
			}
			payload["mode"] = args[i]
			changed = true
		case "--max-trust":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s standing edit: --max-trust needs a value\n", brand.CLI)
				return 2
			}
			payload["max_trust"] = args[i]
			changed = true
		case "--assure":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s standing edit: --assure needs a value\n", brand.CLI)
				return 2
			}
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 0 {
				fmt.Fprintf(stderr, "%s standing edit: --assure must be a non-negative integer\n", brand.CLI)
				return 2
			}
			payload["assure"] = n
			changed = true
		case "--cooldown":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s standing edit: --cooldown needs a value\n", brand.CLI)
				return 2
			}
			d, err := time.ParseDuration(args[i])
			if err != nil || d < 0 {
				fmt.Fprintf(stderr, "%s standing edit: --cooldown must be a non-negative duration like 15m or 1h\n", brand.CLI)
				return 2
			}
			payload["cooldown_sec"] = int64(d / time.Second)
			changed = true
		default:
			fmt.Fprintf(stderr, "%s standing edit: unexpected arg %q\n", brand.CLI, args[i])
			return 2
		}
	}
	if !changed {
		fmt.Fprintf(stderr, "%s standing edit: at least one field flag is required\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdStandingEdit, payload)
	if err != nil {
		fmt.Fprintf(stderr, "%s standing edit: %v\n", brand.CLI, err)
		return 1
	}
	if updated, _ := res["updated"].(bool); !updated {
		fmt.Fprintf(stderr, "%s standing edit: no order with id %s\n", brand.CLI, args[0])
		return 1
	}
	o, _ := res["order"].(map[string]any)
	name, _ := o["name"].(string)
	if name == "" {
		name = args[0]
	}
	fmt.Fprintf(stdout, "standing order %q updated\n", name)
	return 0
}

func cmdStandingWhy(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var id string
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		} else if id == "" {
			id = a
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s standing why: an order id is required\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdStandingWhy, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s standing why: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	events, _ := res["events"].([]any)
	if len(events) == 0 {
		fmt.Fprintln(stdout, "no events for this standing order")
		return 0
	}
	fmt.Fprintf(stdout, "%d event(s):\n", len(events))
	for _, raw := range events {
		e, _ := raw.(map[string]any)
		kind, _ := e["kind"].(string)
		seq, _ := e["seq"].(float64)
		p, _ := e["payload"].(map[string]any)
		line := fmt.Sprintf("  seq=%d  %s", int(seq), kind)
		if action, _ := p["action"].(string); action != "" {
			line += " (" + action + ")"
		}
		if subj, _ := p["trigger_subject"].(string); subj != "" {
			line += " ← " + subj
		}
		fmt.Fprintln(stdout, line)
	}
	return 0
}

func cmdStandingList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdStandingList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s standing list: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	orders, _ := res["orders"].([]any)
	if len(orders) == 0 {
		fmt.Fprintln(stdout, "no standing orders")
		return 0
	}
	enabled, _ := res["enabled_count"].(float64)
	fmt.Fprintf(stdout, "%d standing order(s), %d enabled:\n", len(orders), int(enabled))
	for _, raw := range orders {
		if o, ok := raw.(map[string]any); ok {
			fmt.Fprintln(stdout, renderStandingLine(o))
		}
	}
	return 0
}

// renderStandingLine formats one order map as "<id12> [on|off] name — triggers".
func renderStandingLine(o map[string]any) string {
	id, _ := o["id"].(string)
	if len(id) > 12 {
		id = id[:12]
	}
	name, _ := o["name"].(string)
	state := "off"
	if en, _ := o["enabled"].(bool); en {
		state = "on"
	}
	trigs, _ := o["triggers"].([]any)
	line := fmt.Sprintf("  %s [%s] %s", id, state, name)
	if n := len(trigs); n > 0 {
		line += fmt.Sprintf("  · %d trigger(s)", n)
	}
	if mode := initiativeMode(o); mode != "" {
		line += "  · " + mode
	}
	if status := standingTargetStatus(o); status != "" {
		line += "  · " + status
	}
	return line
}

func standingTargetStatus(o map[string]any) string {
	errText, _ := o["target_error"].(string)
	if errText = strings.TrimSpace(errText); errText != "" {
		return "target:blocked (" + errText + ")"
	}
	status, _ := o["target_status"].(string)
	status = strings.TrimSpace(status)
	if status == "" {
		return ""
	}
	return "target:" + status
}

func initiativeMode(o map[string]any) string {
	ini, _ := o["initiative"].(map[string]any)
	if ini == nil {
		return ""
	}
	m, _ := ini["mode"].(string)
	return m
}

