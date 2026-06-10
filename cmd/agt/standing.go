// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdStanding dispatches `agt standing <subcommand>` — the management surface for
// Chronos standing orders (SPEC-16 §4): persistent goals that fire on a trigger.
// Every mutation is journaled (standing.*) so it's auditable like any other event.
func cmdStanding(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return standingUsage(stderr)
	}
	switch args[0] {
	case "list":
		return cmdStandingList(args[1:], stdout, stderr)
	case "add":
		return cmdStandingAdd(args[1:], stdout, stderr)
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
	fmt.Fprintf(w, "usage: %s standing <list|add|pause|resume|remove>\n", brand.CLI)
	fmt.Fprintf(w, "  list [--json]                                  show standing orders\n")
	fmt.Fprintf(w, "  add --name N (--cron \"SCHED\" | --event SUBJ) [--plan TEXT]\n")
	fmt.Fprintf(w, "      [--agent SLUG]  run each firing AS that named agent (soul/model/memory/budget)\n")
	fmt.Fprintf(w, "      [--mode inform_only|ask|act_or_ask] [--max-trust L0..L4] [--budget USD]\n")
	fmt.Fprintf(w, "      [--scope ent1,ent2] [--channel C]\n")
	fmt.Fprintf(w, "  pause <id>                                     disable an order\n")
	fmt.Fprintf(w, "  resume <id>                                    re-enable an order\n")
	fmt.Fprintf(w, "  remove <id>                                    delete an order\n")
	fmt.Fprintf(w, "  why <id> [--json]                              an order's life story (fires, pauses)\n")
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
	c := dial(stderr)
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
		return encodeJSON(stdout, res)
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
	c := dial(stderr)
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
		return encodeJSON(stdout, res)
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
	return line
}

func initiativeMode(o map[string]any) string {
	ini, _ := o["initiative"].(map[string]any)
	if ini == nil {
		return ""
	}
	m, _ := ini["mode"].(string)
	return m
}

func cmdStandingAdd(args []string, stdout, stderr io.Writer) int {
	var name, cron, event, plan, mode, maxTrust, channel, budget, scope, agentSlug string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			i++
			if i < len(args) {
				name = args[i]
			}
		case "--scope":
			i++
			if i < len(args) {
				scope = args[i]
			}
		case "--budget":
			i++
			if i < len(args) {
				budget = args[i]
			}
		case "--cron":
			i++
			if i < len(args) {
				cron = args[i]
			}
		case "--event":
			i++
			if i < len(args) {
				event = args[i]
			}
		case "--plan":
			i++
			if i < len(args) {
				plan = args[i]
			}
		case "--agent":
			i++
			if i < len(args) {
				agentSlug = args[i]
			}
		case "--mode":
			i++
			if i < len(args) {
				mode = args[i]
			}
		case "--max-trust":
			i++
			if i < len(args) {
				maxTrust = args[i]
			}
		case "--channel":
			i++
			if i < len(args) {
				channel = args[i]
			}
		default:
			fmt.Fprintf(stderr, "%s standing add: unexpected arg %q\n", brand.CLI, args[i])
			return 2
		}
	}
	if name == "" || (cron == "" && event == "") {
		fmt.Fprintf(stderr, "%s standing add: --name and one of --cron/--event are required\n", brand.CLI)
		return 2
	}
	triggers := []any{}
	if cron != "" {
		triggers = append(triggers, map[string]any{"type": "cron", "schedule": cron})
	}
	if event != "" {
		triggers = append(triggers, map[string]any{"type": "event", "subject": event})
	}
	var budgetMc int64
	if budget != "" {
		mc, berr := usdToMicrocents(budget)
		if berr != nil {
			fmt.Fprintf(stderr, "%s standing add: --budget: %v\n", brand.CLI, berr)
			return 2
		}
		budgetMc = mc
	}

	order := map[string]any{"name": name, "triggers": triggers}
	if plan != "" {
		order["plan"] = plan
	}
	if scope != "" {
		var ents []any
		for _, e := range strings.Split(scope, ",") {
			if e = strings.TrimSpace(e); e != "" {
				ents = append(ents, e)
			}
		}
		if len(ents) > 0 {
			order["scope_entities"] = ents
		}
	}
	if mode != "" || maxTrust != "" || budgetMc > 0 {
		ini := map[string]any{}
		if mode != "" {
			ini["mode"] = mode
		}
		if maxTrust != "" {
			ini["max_trust"] = maxTrust
		}
		if budgetMc > 0 {
			ini["budget_per_run_mc"] = budgetMc
		}
		order["initiative"] = ini
	}
	if channel != "" {
		order["briefing_channel"] = channel
	}
	if agentSlug = strings.TrimSpace(agentSlug); agentSlug != "" {
		order["agent"] = agentSlug // M790: firings run AS this roster agent
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdStandingAdd, map[string]any{"order": order})
	if err != nil {
		fmt.Fprintf(stderr, "%s standing add: %v\n", brand.CLI, err)
		return 1
	}
	o, _ := res["order"].(map[string]any)
	id, _ := o["id"].(string)
	fmt.Fprintf(stdout, "standing order %q added\n", name)
	fmt.Fprintf(stdout, "  id: %s\n", id)
	return 0
}

func cmdStandingSetEnabled(args []string, stdout, stderr io.Writer, enabled bool) int {
	verb := "pause"
	if enabled {
		verb = "resume"
	}
	if len(args) == 0 || args[0] == "" {
		fmt.Fprintf(stderr, "%s standing %s: an order id is required\n", brand.CLI, verb)
		return 2
	}
	id := args[0]
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdStandingSetEnabled, map[string]any{"id": id, "enabled": enabled})
	if err != nil {
		fmt.Fprintf(stderr, "%s standing %s: %v\n", brand.CLI, verb, err)
		return 1
	}
	o, _ := res["order"].(map[string]any)
	name, _ := o["name"].(string)
	fmt.Fprintf(stdout, "standing order %q %sd\n", name, verb)
	return 0
}

func cmdStandingRemove(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "" {
		fmt.Fprintf(stderr, "%s standing remove: an order id is required\n", brand.CLI)
		return 2
	}
	id := args[0]
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdStandingRemove, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s standing remove: %v\n", brand.CLI, err)
		return 1
	}
	if removed, _ := res["removed"].(bool); !removed {
		fmt.Fprintf(stderr, "%s standing remove: no order with id %s\n", brand.CLI, id)
		return 1
	}
	fmt.Fprintf(stdout, "standing order %s removed\n", id)
	return 0
}
