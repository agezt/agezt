// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdSchedule dispatches `agt schedule <subcommand>` — the operator's
// management path into the persistent scheduled-intents store (autonomy). The
// cadence resident fires due schedules through the same governed loop as
// `agt run`; these commands add/list/remove/trigger them.
func cmdSchedule(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s schedule: subcommand required (add|edit|list|rm|run|pause|resume)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "add":
		return cmdScheduleAdd(args[1:], stdout, stderr)
	case "edit":
		return cmdScheduleEdit(args[1:], stdout, stderr)
	case "list", "ls":
		return cmdScheduleList(args[1:], stdout, stderr)
	case "fires", "history":
		return cmdScheduleFires(args[1:], stdout, stderr)
	case "stats":
		return cmdScheduleStats(args[1:], stdout, stderr)
	case "rm", "remove":
		return cmdScheduleRemove(args[1:], stdout, stderr)
	case "run", "trigger":
		return cmdScheduleRun(args[1:], stdout, stderr)
	case "pause":
		return cmdScheduleEnable(args[1:], stdout, stderr, false)
	case "resume":
		return cmdScheduleEnable(args[1:], stdout, stderr, true)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s schedule <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  add \"<intent>\" --every <dur> [--between HH:MM-HH:MM [--days <spec>]]  interval, optionally windowed\n")
		fmt.Fprintf(stdout, "  add \"<intent>\" --at <HH:MM> [--days <spec>] [--model <id>]    daily at a wall-clock time\n")
		fmt.Fprintf(stdout, "  add \"<intent>\" --in <dur> | --once --at <HH:MM>              one-shot (fires once, then removed)\n")
		fmt.Fprintf(stdout, "  edit <id> [--intent <s>] [--model <id>] [<cadence flag>]      change a schedule in place\n")
		fmt.Fprintf(stdout, "  list [--json]                                                list all schedules\n")
		fmt.Fprintf(stdout, "  fires [N] [--id <sched>] [--json]                            recent scheduled firings + outcomes\n")
		fmt.Fprintf(stdout, "  stats [--id <sched>] [--since <dur>] [--json]                aggregate firing health (counts, success, spend)\n")
		fmt.Fprintf(stdout, "  rm <id> [--json]                                             delete a schedule\n")
		fmt.Fprintf(stdout, "  run <id> [--json]                                            fire a schedule now (next tick)\n")
		fmt.Fprintf(stdout, "  pause <id> / resume <id> [--json]                            disable / re-enable without deleting\n")
		fmt.Fprintf(stdout, "  <dur> is a Go duration (30m, 1h, 24h); <HH:MM> is 24h time.\n")
		fmt.Fprintf(stdout, "  <spec> is weekdays | weekends | a list/range like mon,wed,fri or mon-fri.\n")
		fmt.Fprintf(stdout, "  --tz <IANA> (e.g. America/New_York) sets the zone for --at / --between wall-clock times.\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s schedule: unknown subcommand %q (add|list|rm|run|pause|resume)\n", brand.CLI, args[0])
		return 2
	}
}

// parseHHMM converts "HH:MM" (24h) to minutes since midnight.
func parseHHMM(s string) (int, error) {
	h, m, ok := strings.Cut(s, ":")
	if !ok {
		return 0, fmt.Errorf("expected HH:MM")
	}
	hh, err1 := strconv.Atoi(strings.TrimSpace(h))
	mm, err2 := strconv.Atoi(strings.TrimSpace(m))
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, fmt.Errorf("expected HH:MM in 00:00..23:59")
	}
	return hh*60 + mm, nil
}

// parseWindow parses a "HH:MM-HH:MM" window into start/end minutes since
// midnight, requiring end strictly after start.
func parseWindow(s string) (start, end int, err error) {
	lo, hi, ok := strings.Cut(s, "-")
	if !ok {
		return 0, 0, fmt.Errorf("expected HH:MM-HH:MM")
	}
	start, err = parseHHMM(strings.TrimSpace(lo))
	if err != nil {
		return 0, 0, err
	}
	end, err = parseHHMM(strings.TrimSpace(hi))
	if err != nil {
		return 0, 0, err
	}
	if end <= start {
		return 0, 0, fmt.Errorf("window end must be after its start")
	}
	return start, end, nil
}

// nextWallclock returns the next local occurrence of mins-past-midnight strictly
// after now (today if still ahead, else tomorrow) — used for one-shot --once --at.
func nextWallclock(now time.Time, mins int) time.Time {
	y, m, d := now.Date()
	cand := time.Date(y, m, d, mins/60, mins%60, 0, 0, now.Location())
	if !cand.After(now) {
		cand = time.Date(y, m, d+1, mins/60, mins%60, 0, 0, now.Location())
	}
	return cand
}

func cmdScheduleAdd(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	once := false
	var every, at, in, between, days, tz, model string
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--once":
			once = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s schedule add \"<intent>\" (--every <dur> [--between <HH:MM-HH:MM> [--days <spec>]] | --at <HH:MM> [--days <spec>] | --once --at <HH:MM> | --in <dur>) [--tz <IANA>] [--model <id>] [--json]\n", brand.CLI)
			return 0
		case "--tz":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --tz needs an IANA zone (e.g. America/New_York)\n", brand.CLI)
				return 2
			}
			i++
			tz = args[i]
		case "--between":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --between needs a HH:MM-HH:MM window\n", brand.CLI)
				return 2
			}
			i++
			between = args[i]
		case "--every":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --every needs a duration\n", brand.CLI)
				return 2
			}
			i++
			every = args[i]
		case "--at":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --at needs a HH:MM time\n", brand.CLI)
				return 2
			}
			i++
			at = args[i]
		case "--in":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --in needs a duration\n", brand.CLI)
				return 2
			}
			i++
			in = args[i]
		case "--days":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --days needs a spec (e.g. mon-fri, weekends)\n", brand.CLI)
				return 2
			}
			i++
			days = args[i]
		case "--model":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --model needs a value\n", brand.CLI)
				return 2
			}
			i++
			model = args[i]
		default:
			positional = append(positional, a)
		}
	}
	intent := strings.TrimSpace(strings.Join(positional, " "))
	if intent == "" {
		fmt.Fprintf(stderr, "%s schedule add: an intent is required\n", brand.CLI)
		return 2
	}
	// Exactly one cadence source: --every (interval), --at (daily/one-shot), or
	// --in (one-shot relative).
	sources := 0
	for _, s := range []string{every, at, in} {
		if s != "" {
			sources++
		}
	}
	if sources != 1 {
		fmt.Fprintf(stderr, "%s schedule add: pass exactly one of --every <dur>, --at <HH:MM>, or --in <dur>\n", brand.CLI)
		return 2
	}
	if days != "" && at == "" && between == "" {
		fmt.Fprintf(stderr, "%s schedule add: --days applies to --at (daily) or --every+--between (windowed) schedules\n", brand.CLI)
		return 2
	}
	if between != "" && every == "" {
		fmt.Fprintf(stderr, "%s schedule add: --between requires --every <dur> (a windowed interval)\n", brand.CLI)
		return 2
	}
	if tz != "" && !((at != "" && !once) || between != "") {
		fmt.Fprintf(stderr, "%s schedule add: --tz applies to --at (daily) or --every+--between (windowed) schedules\n", brand.CLI)
		return 2
	}
	if once && at == "" {
		fmt.Fprintf(stderr, "%s schedule add: --once requires --at <HH:MM> (use --in for a relative one-shot)\n", brand.CLI)
		return 2
	}
	if once && days != "" {
		fmt.Fprintf(stderr, "%s schedule add: --days cannot combine with --once (a one-shot has no recurrence)\n", brand.CLI)
		return 2
	}

	callArgs := map[string]any{"intent": intent}
	var human string
	switch {
	case in != "":
		d, err := time.ParseDuration(in)
		if err != nil {
			fmt.Fprintf(stderr, "%s schedule add: bad --in duration %q: %v\n", brand.CLI, in, err)
			return 2
		}
		if d < time.Second {
			fmt.Fprintf(stderr, "%s schedule add: --in must be at least 1s\n", brand.CLI)
			return 2
		}
		fireAt := time.Now().Add(d)
		callArgs["once_at_unix"] = fireAt.Unix()
		human = "once at " + fireAt.Format("2006-01-02 15:04")
	case at != "" && once:
		mins, err := parseHHMM(at)
		if err != nil {
			fmt.Fprintf(stderr, "%s schedule add: bad --at %q: %v\n", brand.CLI, at, err)
			return 2
		}
		fireAt := nextWallclock(time.Now(), mins)
		callArgs["once_at_unix"] = fireAt.Unix()
		human = "once at " + fireAt.Format("2006-01-02 15:04")
	case at != "":
		mins, err := parseHHMM(at)
		if err != nil {
			fmt.Fprintf(stderr, "%s schedule add: bad --at %q: %v\n", brand.CLI, at, err)
			return 2
		}
		callArgs["at_minutes"] = mins
		human = "daily at " + at
		if days != "" {
			mask, err := cadence.ParseDays(days)
			if err != nil {
				fmt.Fprintf(stderr, "%s schedule add: bad --days %q: %v\n", brand.CLI, days, err)
				return 2
			}
			callArgs["days"] = mask
			if label := cadence.FormatDays(mask); label != "" {
				human = label + " at " + at
			}
		}
		if tz != "" {
			callArgs["tz"] = tz
			human += " " + tz
		}
	default: // every (plain interval, or windowed when --between is set)
		d, err := time.ParseDuration(every)
		if err != nil {
			fmt.Fprintf(stderr, "%s schedule add: bad --every duration %q: %v\n", brand.CLI, every, err)
			return 2
		}
		if d < time.Second {
			fmt.Fprintf(stderr, "%s schedule add: interval must be at least 1s\n", brand.CLI)
			return 2
		}
		callArgs["interval_sec"] = int64(d / time.Second)
		if between != "" {
			start, end, err := parseWindow(between)
			if err != nil {
				fmt.Fprintf(stderr, "%s schedule add: bad --between %q: %v\n", brand.CLI, between, err)
				return 2
			}
			callArgs["window_start"] = start
			callArgs["window_end"] = end
			human = fmt.Sprintf("every %s %s", d, between)
			if days != "" {
				mask, err := cadence.ParseDays(days)
				if err != nil {
					fmt.Fprintf(stderr, "%s schedule add: bad --days %q: %v\n", brand.CLI, days, err)
					return 2
				}
				callArgs["days"] = mask
				if label := cadence.FormatDays(mask); label != "" {
					human += " " + label
				}
			}
			if tz != "" {
				callArgs["tz"] = tz
				human += " " + tz
			}
		} else {
			human = "every " + d.String()
		}
	}
	if model != "" {
		callArgs["model"] = model
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdScheduleAdd, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule add: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	id, _ := res["id"].(string)
	fmt.Fprintf(stdout, "scheduled %s (%s)\n", id, human)
	return 0
}

// cmdScheduleEdit changes an existing schedule in place: any of --intent,
// --model, and at most one new cadence (--every | --at [--days] | --in | --once
// --at). The id is preserved; the cadence change recomputes the next run.
func cmdScheduleEdit(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	once := false
	var id, intent, model, every, at, in, between, days, tz string
	var setIntent, setModel bool
	for i := 0; i < len(args); i++ {
		a := args[i]
		needVal := func(flag string) (string, bool) {
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule edit: %s needs a value\n", brand.CLI, flag)
				return "", false
			}
			i++
			return args[i], true
		}
		switch a {
		case "--json":
			asJSON = true
		case "--once":
			once = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s schedule edit <id> [--intent <s>] [--model <id>] [--every <dur> | --at <HH:MM> [--days <spec>] | --in <dur> | --once --at <HH:MM>] [--json]\n", brand.CLI)
			return 0
		case "--intent":
			v, ok := needVal("--intent")
			if !ok {
				return 2
			}
			intent, setIntent = v, true
		case "--model":
			v, ok := needVal("--model")
			if !ok {
				return 2
			}
			model, setModel = v, true
		case "--every":
			v, ok := needVal("--every")
			if !ok {
				return 2
			}
			every = v
		case "--at":
			v, ok := needVal("--at")
			if !ok {
				return 2
			}
			at = v
		case "--in":
			v, ok := needVal("--in")
			if !ok {
				return 2
			}
			in = v
		case "--between":
			v, ok := needVal("--between")
			if !ok {
				return 2
			}
			between = v
		case "--tz":
			v, ok := needVal("--tz")
			if !ok {
				return 2
			}
			tz = v
		case "--days":
			v, ok := needVal("--days")
			if !ok {
				return 2
			}
			days = v
		default:
			if id == "" && !strings.HasPrefix(a, "-") {
				id = a
			} else {
				fmt.Fprintf(stderr, "%s schedule edit: unexpected arg %q\n", brand.CLI, a)
				return 2
			}
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s schedule edit: an id is required\n", brand.CLI)
		return 2
	}
	sources := 0
	for _, s := range []string{every, at, in} {
		if s != "" {
			sources++
		}
	}
	if sources > 1 {
		fmt.Fprintf(stderr, "%s schedule edit: pass at most one of --every, --at, or --in\n", brand.CLI)
		return 2
	}
	if days != "" && at == "" && between == "" {
		fmt.Fprintf(stderr, "%s schedule edit: --days applies to --at (daily) or --every+--between (windowed) schedules\n", brand.CLI)
		return 2
	}
	if between != "" && every == "" {
		fmt.Fprintf(stderr, "%s schedule edit: --between requires --every <dur>\n", brand.CLI)
		return 2
	}
	if tz != "" && !((at != "" && !once) || between != "") {
		fmt.Fprintf(stderr, "%s schedule edit: --tz applies to --at (daily) or --every+--between (windowed) schedules\n", brand.CLI)
		return 2
	}
	if once && at == "" {
		fmt.Fprintf(stderr, "%s schedule edit: --once requires --at <HH:MM>\n", brand.CLI)
		return 2
	}
	if once && days != "" {
		fmt.Fprintf(stderr, "%s schedule edit: --days cannot combine with --once\n", brand.CLI)
		return 2
	}
	if !setIntent && !setModel && sources == 0 {
		fmt.Fprintf(stderr, "%s schedule edit: nothing to change (pass --intent, --model, or a cadence flag)\n", brand.CLI)
		return 2
	}

	callArgs := map[string]any{"id": id}
	if setIntent {
		if strings.TrimSpace(intent) == "" {
			fmt.Fprintf(stderr, "%s schedule edit: --intent cannot be empty\n", brand.CLI)
			return 2
		}
		callArgs["intent"] = intent
	}
	if setModel {
		callArgs["model"] = model
	}
	switch {
	case in != "":
		d, err := time.ParseDuration(in)
		if err != nil || d < time.Second {
			fmt.Fprintf(stderr, "%s schedule edit: bad --in duration %q\n", brand.CLI, in)
			return 2
		}
		callArgs["once_at_unix"] = time.Now().Add(d).Unix()
	case at != "" && once:
		mins, err := parseHHMM(at)
		if err != nil {
			fmt.Fprintf(stderr, "%s schedule edit: bad --at %q: %v\n", brand.CLI, at, err)
			return 2
		}
		callArgs["once_at_unix"] = nextWallclock(time.Now(), mins).Unix()
	case at != "":
		mins, err := parseHHMM(at)
		if err != nil {
			fmt.Fprintf(stderr, "%s schedule edit: bad --at %q: %v\n", brand.CLI, at, err)
			return 2
		}
		callArgs["at_minutes"] = mins
		if days != "" {
			mask, err := cadence.ParseDays(days)
			if err != nil {
				fmt.Fprintf(stderr, "%s schedule edit: bad --days %q: %v\n", brand.CLI, days, err)
				return 2
			}
			callArgs["days"] = mask
		}
		if tz != "" {
			callArgs["tz"] = tz
		}
	case every != "":
		d, err := time.ParseDuration(every)
		if err != nil || d < time.Second {
			fmt.Fprintf(stderr, "%s schedule edit: bad --every duration %q\n", brand.CLI, every)
			return 2
		}
		callArgs["interval_sec"] = int64(d / time.Second)
		if between != "" {
			start, end, err := parseWindow(between)
			if err != nil {
				fmt.Fprintf(stderr, "%s schedule edit: bad --between %q: %v\n", brand.CLI, between, err)
				return 2
			}
			callArgs["window_start"] = start
			callArgs["window_end"] = end
			if days != "" {
				mask, err := cadence.ParseDays(days)
				if err != nil {
					fmt.Fprintf(stderr, "%s schedule edit: bad --days %q: %v\n", brand.CLI, days, err)
					return 2
				}
				callArgs["days"] = mask
			}
			if tz != "" {
				callArgs["tz"] = tz
			}
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdScheduleEdit, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule edit: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	if updated, _ := res["updated"].(bool); !updated {
		fmt.Fprintf(stderr, "%s schedule edit: not found (%s)\n", brand.CLI, id)
		return 3
	}
	cad, _ := res["cadence"].(string)
	fmt.Fprintf(stdout, "%s updated (%s)\n", id, cad)
	return 0
}

// cmdScheduleEnable backs `schedule pause` (enabled=false) and `resume`
// (enabled=true).
func cmdScheduleEnable(args []string, stdout, stderr io.Writer, enabled bool) int {
	verb := "resume"
	if !enabled {
		verb = "pause"
	}
	asJSON := false
	var id string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s schedule %s <id> [--json]\n", brand.CLI, verb)
			return 0
		default:
			if id == "" {
				id = a
			}
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s schedule %s: an id is required\n", brand.CLI, verb)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdScheduleEnable, map[string]any{"id": id, "enabled": enabled})
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule %s: %v\n", brand.CLI, verb, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	if updated, _ := res["updated"].(bool); !updated {
		fmt.Fprintf(stderr, "%s schedule %s: not found (%s)\n", brand.CLI, verb, id)
		return 3
	}
	fmt.Fprintf(stdout, "%s %sd\n", id, verb)
	return 0
}

func cmdScheduleList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s schedule list [--json]\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s schedule list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdScheduleList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule list: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	list, _ := res["schedules"].([]any)
	if len(list) == 0 {
		fmt.Fprintf(stdout, "no schedules. Add one with `%s schedule add \"<intent>\" --every 1h`.\n", brand.CLI)
		return 0
	}
	for _, item := range list {
		m, _ := item.(map[string]any)
		id, _ := m["id"].(string)
		intent, _ := m["intent"].(string)
		cadence, _ := m["cadence"].(string)
		source, _ := m["source"].(string)
		enabled, _ := m["enabled"].(bool)
		next, _ := m["next_run_unix"].(float64)
		state := "enabled"
		if !enabled {
			state = "paused"
		}
		nextStr := "—"
		if next > 0 {
			nextStr = time.Unix(int64(next), 0).Format("2006-01-02 15:04")
		}
		fmt.Fprintf(stdout, "  %-22s %-16s [%s,%s] next %s  %q",
			id, cadence, source, state, nextStr, intent)
		// Last-firing outcome (M56) — how the schedule last went, when known.
		lastStatus, _ := m["last_status"].(string)
		if lastStatus != "" {
			lastReason, _ := m["last_reason"].(string)
			if lastStatus == "failed" && lastReason != "" {
				lastStatus = "failed (" + lastReason + ")"
			}
			lastWhen := ""
			if lf, ok := m["last_fired_unix_ms"].(float64); ok && lf > 0 {
				lastWhen = " " + time.UnixMilli(int64(lf)).Format("01-02 15:04")
			}
			fmt.Fprintf(stdout, "  last: %s%s", lastStatus, lastWhen)
		}
		fmt.Fprintln(stdout)
	}
	return 0
}

// cmdScheduleFires implements `agt schedule fires [N] [--json]` — the autonomy
// analogue of `agt runs list`. `schedule list` shows what's scheduled; this
// shows what actually FIRED and how it turned out (status, duration, spend),
// joined server-side from the schedule.fired events and the run outcomes (M54).
// Drill into any firing with `agt runs show <correlation>`.
func cmdScheduleFires(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	limit := 0
	id := ""
	status := ""
	intent := ""
	sinceMS := int64(0)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--intent":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule fires: --intent needs a substring\n", brand.CLI)
				return 2
			}
			i++
			intent = args[i]
		case strings.HasPrefix(a, "--intent="):
			intent = strings.TrimPrefix(a, "--intent=")
		case a == "--id":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule fires: --id needs a schedule id\n", brand.CLI)
				return 2
			}
			i++
			id = args[i]
		case strings.HasPrefix(a, "--id="):
			id = strings.TrimPrefix(a, "--id=")
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule fires: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s schedule fires: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s schedule fires: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
		case a == "--failed":
			status = "failed"
		case a == "--status":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule fires: --status needs a value\n", brand.CLI)
				return 2
			}
			i++
			status = args[i]
		case strings.HasPrefix(a, "--status="):
			status = strings.TrimPrefix(a, "--status=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s schedule fires [N] [--id <sched>] [--status <s>|--failed] [--since <dur>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show recent scheduled-run firings and their outcomes (status, duration, spend)\n")
			fmt.Fprintf(stdout, "  --id <sched>   only this schedule's firings\n")
			fmt.Fprintf(stdout, "  --status <s>   only firings with this status (completed|failed|running|abandoned)\n")
			fmt.Fprintf(stdout, "  --failed       shorthand for --status failed\n")
			fmt.Fprintf(stdout, "  --intent <substr> only firings whose intent contains <substr>\n")
			fmt.Fprintf(stdout, "drill into a firing with `%s runs show <correlation>`\n", brand.CLI)
			return 0
		default:
			if n, err := strconv.Atoi(a); err == nil && n > 0 {
				limit = n
				continue
			}
			fmt.Fprintf(stderr, "%s schedule fires: unexpected arg %q (expected N, --id <sched>, --status <s>, or --json)\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	callArgs := map[string]any{}
	if limit > 0 {
		callArgs["limit"] = limit
	}
	if id != "" {
		callArgs["id"] = id // M55: filter to one schedule's firings
	}
	if status != "" {
		callArgs["status"] = status // M61: filter by firing status
	}
	if intent != "" {
		callArgs["intent"] = intent // M80: intent substring filter
	}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS // M65: time window
	}
	res, err := c.Call(ctx, controlplane.CmdScheduleFires, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule fires: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	fires, _ := res["fires"].([]any)
	if len(fires) == 0 {
		fmt.Fprintf(stdout, "no scheduled firings yet (schedules fire on their cadence; see `%s schedule list`).\n", brand.CLI)
		return 0
	}
	for _, item := range fires {
		m, _ := item.(map[string]any)
		corr, _ := m["correlation_id"].(string)
		intent, _ := m["intent"].(string)
		status, _ := m["status"].(string)
		reason, _ := m["reason"].(string)
		fired := intOfStatus(m["fired_unix_ms"])
		dur := intOfStatus(m["duration_ms"])
		spent := mcFromAny(m["spent_mc"])

		statusDisp := status
		if status == "failed" && reason != "" {
			statusDisp = "failed (" + reason + ")"
		}
		firedStr := "—"
		if fired > 0 {
			firedStr = time.UnixMilli(fired).Format("2006-01-02 15:04:05")
		}
		// Duration + spend only for terminal firings (running ones have neither).
		meta := ""
		if status == "completed" || status == "failed" {
			meta = " (" + fmtDuration(dur)
			if spent > 0 {
				meta += ", " + fmtUSD(spent)
			}
			meta += ")"
		}
		fmt.Fprintf(stdout, "  %s  %-18s%s  %s  %q\n", firedStr, statusDisp, meta, corr, intent)
	}
	return 0
}

// cmdScheduleStats implements `agt schedule stats [--id <sched>] [--since <dur>]
// [--json]` — the autonomy analogue of `agt runs stats`, aggregating scheduled
// firings: counts, success rate, and total spend (M57).
func cmdScheduleStats(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	id := ""
	sinceMS := int64(0)
	sinceLabel := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--id":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule stats: --id needs a schedule id\n", brand.CLI)
				return 2
			}
			i++
			id = args[i]
		case strings.HasPrefix(a, "--id="):
			id = strings.TrimPrefix(a, "--id=")
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule stats: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s schedule stats: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s schedule stats: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s schedule stats [--id <sched>] [--since <dur>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "aggregate scheduled-firing health: counts, success rate, total spend\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s schedule stats: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	callArgs := map[string]any{}
	if id != "" {
		callArgs["id"] = id
	}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	res, err := c.Call(ctx, controlplane.CmdScheduleStats, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule stats: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	total := intOfStatus(res["total"])
	windowSuffix := ""
	if sinceLabel != "" {
		windowSuffix = " in the last " + sinceLabel
	}
	if total == 0 {
		fmt.Fprintf(stdout, "no scheduled firings%s.\n", windowSuffix)
		return 0
	}
	completed := intOfStatus(res["completed"])
	failed := intOfStatus(res["failed"])
	running := intOfStatus(res["running"])
	abandoned := intOfStatus(res["abandoned"])
	terminal := intOfStatus(res["terminal"])
	schedules := intOfStatus(res["schedules"])

	fmt.Fprintf(stdout, "schedule firings (over %d firing(s)%s):\n\n", total, windowSuffix)
	fmt.Fprintf(stdout, "  schedules : %d distinct fired\n", schedules)
	fmt.Fprintf(stdout, "  completed : %d\n", completed)
	failedLine := fmt.Sprintf("%d", failed)
	if br := failedByReasonStr(res["failed_by_reason"]); br != "" {
		failedLine += " (" + br + ")"
	}
	fmt.Fprintf(stdout, "  failed    : %s\n", failedLine)
	fmt.Fprintf(stdout, "  running   : %d\n", running)
	fmt.Fprintf(stdout, "  abandoned : %d\n", abandoned)
	if terminal > 0 {
		rate, _ := res["success_rate"].(float64)
		fmt.Fprintf(stdout, "  success   : %.1f%% (%d/%d terminal)\n", rate*100, completed, terminal)
	} else {
		fmt.Fprintf(stdout, "  success   : n/a (no firing has finished yet)\n")
	}
	if spent := mcFromAny(res["spent_microcents"]); spent > 0 {
		fmt.Fprintf(stdout, "  spend     : %s\n", fmtUSD(spent))
	}
	return 0
}

func cmdScheduleRemove(args []string, stdout, stderr io.Writer) int {
	return scheduleByID(args, stdout, stderr, "rm", controlplane.CmdScheduleRemove, "removed", "removed", "not found")
}

func cmdScheduleRun(args []string, stdout, stderr io.Writer) int {
	return scheduleByID(args, stdout, stderr, "run", controlplane.CmdScheduleRun, "triggered", "triggered (fires on the next tick)", "not found")
}

// scheduleByID factors the rm/run commands: both take a single id, call a
// control-plane command, and report a boolean result key.
func scheduleByID(args []string, stdout, stderr io.Writer, verb, cmd, resultKey, okMsg, missMsg string) int {
	asJSON := false
	var id string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s schedule %s <id> [--json]\n", brand.CLI, verb)
			return 0
		default:
			if id == "" {
				id = a
			}
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s schedule %s: an id is required\n", brand.CLI, verb)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule %s: %v\n", brand.CLI, verb, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	ok, _ := res[resultKey].(bool)
	if !ok {
		fmt.Fprintf(stderr, "%s schedule %s: %s (%s)\n", brand.CLI, verb, missMsg, id)
		return 3
	}
	fmt.Fprintf(stdout, "%s %s\n", id, okMsg)
	return 0
}
