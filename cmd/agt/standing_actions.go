// SPDX-License-Identifier: MIT

// agt standing command: Add/SetEnabled/Remove mutators.
// Code extracted from standing.go during the Day-106 god-file split.
// Public API unchanged.
package main


import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdStandingAdd(args []string, stdout, stderr io.Writer) int {
	var name, cron, event, plan, mode, maxTrust, channel, budget, scope, agentSlug, cooldownRaw string
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
		case "--cooldown":
			i++
			if i < len(args) {
				cooldownRaw = args[i]
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
	if strings.TrimSpace(cooldownRaw) != "" {
		d, err := time.ParseDuration(cooldownRaw)
		if err != nil || d < 0 {
			fmt.Fprintf(stderr, "%s standing add: --cooldown must be a non-negative duration like 15m or 1h\n", brand.CLI)
			return 2
		}
		if d > 0 {
			order["cooldown_sec"] = int64(d / time.Second)
		}
	}

	c := dialpkg.New(stderr)
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
	c := dialpkg.New(stderr)
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
	c := dialpkg.New(stderr)
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
