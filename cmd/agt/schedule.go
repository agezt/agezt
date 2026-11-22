package main

// Provenance: SPDX-License-Identifier: MIT cmd/agt schedule entry + parse helpers
//             (scheduleSystemTaskUsage + parseHHMM + parseWindow + nextWallclock).
//             Split from schedule.go during Day 211 god-file refactor (#37). Public
//             API unchanged.

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
	if sub, ok := cliSchedule.lookup(args[0]); ok {
		return sub.run(nil, args, stdout, stderr)
	}
	fmt.Fprintf(stderr, "%s schedule: unknown subcommand %q (add|list|rm|run|pause|resume)\n", brand.CLI, args[0])
	return 2
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
