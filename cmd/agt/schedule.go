// SPDX-License-Identifier: MIT
//
// cmd/agt schedule entry + parse helpers (scheduleSystemTaskUsage +
// parseHHMM + parseWindow + nextWallclock). Split from schedule.go during
// Day 211 god-file refactor (#37). Public API unchanged.
package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/cadence"
)

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
	case "test", "preview":
		return cmdScheduleTest(args[1:], stdout, stderr)
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
		fmt.Fprintf(stdout, "  add \"<agent task>\" --every <dur> [--between HH:MM-HH:MM [--days <spec>]]  wake an agent/task on an interval\n")
		fmt.Fprintf(stdout, "  add \"<agent task>\" --at <HH:MM> [--days <spec>] [--agent <slug>] [--model <id>]    wake at a wall-clock time\n")
		fmt.Fprintf(stdout, "  add [\"<label>\"] --workflow <ref> [--payload JSON] --every <dur>|--at <HH:MM>|--in <dur>\n")
		fmt.Fprintf(stdout, "  add [\"<label>\"] --system-task %s --every <dur>|--at <HH:MM>|--in <dur>\n", scheduleSystemTaskUsage())
		fmt.Fprintf(stdout, "  add [\"<label>\"] --tool <name> [--payload JSON] --every <dur>|--at <HH:MM>|--in <dur>\n")
		fmt.Fprintf(stdout, "  add \"<agent task>\" --continuous <dur> [--agent <slug>]      cycle loop; re-wakes after each completed run plus cooldown\n")
		fmt.Fprintf(stdout, "  add \"<agent task>\" --in <dur> | --once --at <HH:MM>          one-shot (fires once, then removed)\n")
		fmt.Fprintf(stdout, "  edit <id> [--intent <task/label>] [--model <id>] [<cadence flag>]  change a schedule in place\n")
		fmt.Fprintf(stdout, "  list [--json]                                                list all schedules\n")
		fmt.Fprintf(stdout, "  fires [N] [--id <sched>] [--json]                            recent scheduled firings + outcomes\n")
		fmt.Fprintf(stdout, "  stats [--id <sched>] [--since <dur>] [--json]                aggregate firing health (counts, success, spend)\n")
		fmt.Fprintf(stdout, "  test <id> [--count N] [--json]                               preview the next N fire times (dry-run)\n")
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

func scheduleSystemTaskUsage() string {
	return strings.Join(cadence.SystemTasks(), "|")
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
