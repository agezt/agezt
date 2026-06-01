// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// cmdJournalImport implements `agt journal import <bundle> [--home <dir>]`
// (M102) — the read-back half of `agt journal export`: the disaster-recovery /
// migration restore. It runs OFFLINE (no daemon) and is deliberately strict and
// non-destructive: the bundle must be a FULL export (genesis-anchored), and the
// target journal directory must be EMPTY — restore never clobbers an existing
// chain. After writing it re-opens the journal to confirm it boots cleanly, so
// any chain problem surfaces here, not at the next daemon start.
//
// The daemon for the target home must be stopped: a running daemon holds the
// journal open, and restore targets a fresh/empty home anyway.
func cmdJournalImport(args []string, stdout, stderr io.Writer) int {
	bundlePath := ""
	homeOverride := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--home":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s journal import: --home needs a directory\n", brand.CLI)
				return 2
			}
			i++
			homeOverride = args[i]
		case strings.HasPrefix(a, "--home="):
			homeOverride = strings.TrimPrefix(a, "--home=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s journal import <bundle> [--home <dir>]\n", brand.CLI)
			fmt.Fprintf(stdout, "restore a full `%s journal export` bundle into an EMPTY journal (offline)\n", brand.CLI)
			fmt.Fprintf(stdout, "  --home <dir>  target base dir (default: %sHOME or the user default)\n", brand.EnvPrefix)
			fmt.Fprintf(stdout, "the daemon for the target home must be stopped; restore never clobbers an existing chain\n")
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s journal import: unexpected flag %q\n", brand.CLI, a)
			return 2
		default:
			if bundlePath != "" {
				fmt.Fprintf(stderr, "%s journal import: unexpected arg %q (one bundle path)\n", brand.CLI, a)
				return 2
			}
			bundlePath = a
		}
	}
	if bundlePath == "" {
		fmt.Fprintf(stderr, "%s journal import: a bundle path is required\n", brand.CLI)
		return 2
	}

	data, err := os.ReadFile(bundlePath)
	if err != nil {
		fmt.Fprintf(stderr, "%s journal import: read %s: %v\n", brand.CLI, bundlePath, err)
		return 1
	}
	var b journalBundle
	if err := json.Unmarshal(data, &b); err != nil {
		fmt.Fprintf(stderr, "%s journal import: parse bundle: %v\n", brand.CLI, err)
		return 1
	}
	events := make([]*event.Event, 0, len(b.Events))
	for idx, raw := range b.Events {
		e, derr := event.Decode(raw)
		if derr != nil {
			fmt.Fprintf(stderr, "%s journal import: bundle event %d undecodable: %v\n", brand.CLI, idx, derr)
			return 1
		}
		events = append(events, e)
	}
	// Reject a tail-truncated bundle before touching disk: it would chain-verify
	// as a valid prefix but restore an incomplete history (M103).
	if cerr := checkBundleCompleteness(events, b.Manifest); cerr != nil {
		fmt.Fprintf(stderr, "%s journal import: bundle INCOMPLETE: %v\n", brand.CLI, cerr)
		return 1
	}

	baseDir := homeOverride
	if baseDir == "" {
		baseDir, err = paths.BaseDir()
		if err != nil {
			fmt.Fprintf(stderr, "%s journal import: resolve base dir: %v\n", brand.CLI, err)
			return 1
		}
	}
	journalDir := filepath.Join(baseDir, "journal")

	headSeq, headHash, rerr := journal.Restore(journalDir, events)
	if rerr != nil {
		switch {
		case errors.Is(rerr, journal.ErrNotEmpty):
			fmt.Fprintf(stderr, "%s journal import: %s already has a journal — restore only into an empty home\n", brand.CLI, journalDir)
		case errors.Is(rerr, journal.ErrNotFullExport):
			fmt.Fprintf(stderr, "%s journal import: bundle is not a full export (a --since window cannot seed a journal)\n", brand.CLI)
		default:
			fmt.Fprintf(stderr, "%s journal import: %v\n", brand.CLI, rerr)
		}
		return 1
	}

	// Confirm the restored journal boots cleanly (same scan the daemon runs).
	jj, oerr := journal.Open(journalDir, journal.Options{})
	if oerr != nil {
		fmt.Fprintf(stderr, "%s journal import: restored but journal does not open: %v\n", brand.CLI, oerr)
		return 1
	}
	gotSeq, gotHash := jj.Head()
	_ = jj.Close()
	if gotSeq != headSeq || gotHash != headHash {
		fmt.Fprintf(stderr, "%s journal import: head mismatch after restore (wrote seq=%d, journal reports seq=%d)\n",
			brand.CLI, headSeq, gotSeq)
		return 1
	}

	fmt.Fprintf(stdout, "restored %d event(s) into %s\n", len(events), journalDir)
	fmt.Fprintf(stdout, "  head: seq=%d hash=%s\n", headSeq, shortHash(headHash))
	fmt.Fprintf(stdout, "  start the daemon to rebuild projections from the restored journal\n")
	return 0
}
