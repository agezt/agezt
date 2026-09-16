// SPDX-License-Identifier: MIT

// backup_restore_helpers.go: path-validator + point-in-time restore helpers
// split off from backup_restore.go during the Day 211 god-file refactor (#127).
// Public API unchanged.
package main

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)


// isAllowedBackupPath reports whether a tar entry name is within a known include
// subtree and free of traversal segments.
func isAllowedBackupPath(name string) bool {
	if strings.Contains(name, "..") || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
		return false
	}
	for _, d := range backupIncludeDirs {
		if strings.HasPrefix(name, d+"/") {
			return true
		}
	}
	return false
}

// parseAtSpec interprets a `--at` value as either a journal sequence (a plain
// non-negative integer) or an RFC3339 timestamp. Returns whether it is a seq,
// the seq, and the unix-ms cutoff for the timestamp case.
func parseAtSpec(at string) (isSeq bool, seq, tsMS int64, err error) {
	at = strings.TrimSpace(at)
	if n, e := strconv.ParseInt(at, 10, 64); e == nil {
		if n < 0 {
			return false, 0, 0, fmt.Errorf("seq must be non-negative")
		}
		return true, n, 0, nil
	}
	t, e := time.Parse(time.RFC3339, at)
	if e != nil {
		return false, 0, 0, fmt.Errorf("--at must be a sequence (integer) or an RFC3339 timestamp like 2026-06-04T14:00:00Z")
	}
	return false, 0, t.UnixMilli(), nil
}

// pointInTimeRestore replays the source home's journal up to a point in time
// (seq or timestamp) into a FRESH --to home (SPEC-09 §5). It is non-destructive:
// the source journal is opened read-only and untouched; the result is a separate,
// independently-bootable home holding the genesis→cutoff prefix — "branch a
// recovered state". The journal is the time machine; no special backup needed.
func pointInTimeRestore(homeOverride, toDir, atSpec string, stdout, stderr io.Writer) int {
	isSeq, wantSeq, wantMS, perr := parseAtSpec(atSpec)
	if perr != nil {
		fmt.Fprintf(stderr, "%s restore: %v\n", brand.CLI, perr)
		return 2
	}
	srcHome, err := resolveHome(homeOverride)
	if err != nil {
		fmt.Fprintf(stderr, "%s restore: %v\n", brand.CLI, err)
		return 1
	}
	srcJournal := filepath.Join(srcHome, "journal")
	if segs, _ := filepath.Glob(filepath.Join(srcJournal, "*.jsonl")); len(segs) == 0 {
		fmt.Fprintf(stderr, "%s restore: no journal at %s\n", brand.CLI, srcJournal)
		return 1
	}
	j, err := journal.Open(srcJournal, journal.Options{})
	if err != nil {
		fmt.Fprintf(stderr, "%s restore: open source journal: %v\n", brand.CLI, err)
		return 1
	}
	if verr := j.Verify(); verr != nil {
		j.Close()
		fmt.Fprintf(stderr, "%s restore: source journal chain invalid: %v\n", brand.CLI, verr)
		return 1
	}
	// Collect the contiguous genesis→cutoff prefix. Stop at the FIRST event past
	// the cutoff so the slice stays a valid chain (Restore requires seq to start
	// at 0 and be gap-free), even if timestamps aren't perfectly monotonic.
	var prefix []*event.Event
	done := false
	_ = j.Range(func(e *event.Event) error {
		if done {
			return nil
		}
		over := e.Seq > wantSeq
		if !isSeq {
			over = e.TSUnixMS > wantMS
		}
		if over {
			done = true
			return nil
		}
		prefix = append(prefix, e)
		return nil
	})
	j.Close()
	if len(prefix) == 0 {
		fmt.Fprintf(stderr, "%s restore: no events at or before %s (nothing to restore)\n", brand.CLI, atSpec)
		return 1
	}

	toJournal := filepath.Join(toDir, "journal")
	outSeq, _, rerr := journal.Restore(toJournal, prefix)
	if rerr != nil {
		if errors.Is(rerr, journal.ErrNotEmpty) {
			fmt.Fprintf(stderr, "%s restore: %s already has a journal — point-in-time restore needs an empty --to dir\n", brand.CLI, toJournal)
		} else {
			fmt.Fprintf(stderr, "%s restore: %v\n", brand.CLI, rerr)
		}
		return 1
	}
	// Confirm the branched home boots and its chain verifies.
	if _, _, verr := verifyHomeJournal(toDir); verr != nil {
		fmt.Fprintf(stderr, "%s restore: restored journal did not verify: %v\n", brand.CLI, verr)
		return 1
	}
	cutoff := "seq " + strconv.FormatInt(wantSeq, 10)
	if !isSeq {
		cutoff = atSpec
	}
	fmt.Fprintf(stdout, "restored point-in-time state up to %s: %d event(s), head seq %d → %s\n",
		cutoff, len(prefix), outSeq, toDir)
	fmt.Fprintf(stdout, "(source %s untouched; re-provision credentials before starting a daemon there)\n", srcHome)
	return 0
}
