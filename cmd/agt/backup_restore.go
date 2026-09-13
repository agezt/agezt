// SPDX-License-Identifier: MIT

package main

// agt backup restore: cmdRestore + restoreBackup + isAllowedBackupPath +
// resolveHome + verifyHomeJournal + parseAtSpec + pointInTimeRestore.
// Carved out of backup.go during the Day 172 god-file split so the main
// file can stay focused on the backup creation + inspect surface.
// Public API unchanged.

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)
func cmdRestore(args []string, stdout, stderr io.Writer) int {
	homeOverride := ""
	archivePath := ""
	atSpec := "" // point-in-time target (seq or RFC3339 timestamp)
	toDir := ""  // where a point-in-time restore writes the branched home
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--home":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s restore: --home needs a directory\n", brand.CLI)
				return 2
			}
			i++
			homeOverride = args[i]
		case strings.HasPrefix(a, "--home="):
			homeOverride = strings.TrimPrefix(a, "--home=")
		case a == "--at":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s restore: --at needs a seq or RFC3339 timestamp\n", brand.CLI)
				return 2
			}
			i++
			atSpec = args[i]
		case strings.HasPrefix(a, "--at="):
			atSpec = strings.TrimPrefix(a, "--at=")
		case a == "--to":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s restore: --to needs a directory\n", brand.CLI)
				return 2
			}
			i++
			toDir = args[i]
		case strings.HasPrefix(a, "--to="):
			toDir = strings.TrimPrefix(a, "--to=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s restore <file> [--home <dir>]\n", brand.CLI)
			fmt.Fprintf(stdout, "       %s restore --at <seq|RFC3339> --to <dir> [--home <src>]\n", brand.CLI)
			fmt.Fprintf(stdout, "unpack a backup into an EMPTY home; re-provision credentials afterwards.\n")
			fmt.Fprintf(stdout, "--at replays the source journal up to a point in time into a fresh --to home\n")
			fmt.Fprintf(stdout, "(non-destructive: the source is untouched; the journal IS the time machine).\n")
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s restore: unexpected flag %q\n", brand.CLI, a)
			return 2
		default:
			if archivePath != "" {
				fmt.Fprintf(stderr, "%s restore: one archive at a time\n", brand.CLI)
				return 2
			}
			archivePath = a
		}
	}

	// Point-in-time mode (SPEC-09 §5): `--at <seq|timestamp> --to <dir>`.
	if atSpec != "" {
		if archivePath != "" {
			fmt.Fprintf(stderr, "%s restore: --at restores from the journal, not an archive — drop %q\n", brand.CLI, archivePath)
			return 2
		}
		if toDir == "" {
			fmt.Fprintf(stderr, "%s restore: --at needs --to <dir> (a fresh home to write the point-in-time state)\n", brand.CLI)
			return 2
		}
		return pointInTimeRestore(homeOverride, toDir, atSpec, stdout, stderr)
	}
	if toDir != "" {
		fmt.Fprintf(stderr, "%s restore: --to is only valid with --at\n", brand.CLI)
		return 2
	}

	if archivePath == "" {
		fmt.Fprintf(stderr, "%s restore: an archive path is required\n", brand.CLI)
		return 2
	}

	home, err := resolveHome(homeOverride)
	if err != nil {
		fmt.Fprintf(stderr, "%s restore: %v\n", brand.CLI, err)
		return 1
	}
	// Refuse to clobber an existing journal.
	if segs, _ := filepath.Glob(filepath.Join(home, "journal", "*.jsonl")); len(segs) > 0 {
		fmt.Fprintf(stderr, "%s restore: %s already has a journal — restore only into an empty home\n", brand.CLI, home)
		return 1
	}

	f, err := os.Open(archivePath)
	if err != nil {
		fmt.Fprintf(stderr, "%s restore: open %s: %v\n", brand.CLI, archivePath, err)
		return 1
	}
	man, rerr := restoreBackup(f, home)
	f.Close()
	if rerr != nil {
		fmt.Fprintf(stderr, "%s restore: %v\n", brand.CLI, rerr)
		return 1
	}

	// Confirm the restored journal boots + verifies.
	headSeq, headHash, verr := verifyHomeJournal(home)
	if verr != nil {
		fmt.Fprintf(stderr, "%s restore: unpacked, but journal does not verify: %v\n", brand.CLI, verr)
		return 1
	}
	fmt.Fprintf(stdout, "restored %s into %s\n", strings.Join(man.Includes, " + "), home)
	fmt.Fprintf(stdout, "  journal head: seq=%d hash=%s\n", headSeq, shortHash(headHash))
	fmt.Fprintf(stdout, "  re-provision credentials (`%s vault`/provider setup), then start the daemon\n", brand.CLI)
	return 0
}

// restoreBackup unpacks a gzip+tar backup into destHome. Each entry's path is
// validated to stay within destHome and within the known include subtrees, so a
// malicious archive cannot write outside the home (zip-slip) or drop unexpected
// files. Returns the manifest.
func restoreBackup(r io.Reader, destHome string) (backupManifest, error) {
	var man backupManifest
	gz, err := gzip.NewReader(r)
	if err != nil {
		return man, fmt.Errorf("not a gzip archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	cleanDest := filepath.Clean(destHome)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return man, fmt.Errorf("read archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := hdr.Name
		if name == backupManifestName {
			data, _ := io.ReadAll(tr)
			_ = json.Unmarshal(data, &man)
			continue
		}
		// Only known subtrees, no traversal.
		if !isAllowedBackupPath(name) {
			return man, fmt.Errorf("refusing suspicious archive entry %q", name)
		}
		target := filepath.Join(cleanDest, filepath.FromSlash(name))
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
			return man, fmt.Errorf("refusing path-traversal entry %q", name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return man, err
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return man, fmt.Errorf("write %s: %w", name, err)
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return man, err
		}
		if err := out.Close(); err != nil {
			return man, err
		}
	}
	return man, nil
}

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

// resolveHome returns the override if set, else the configured base dir.
func resolveHome(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return paths.BaseDir()
}

// verifyHomeJournal opens the home's journal read-only, verifies its chain, and
// returns the head. Used by both backup (refuse a corrupt source) and restore
// (confirm the result boots).
func verifyHomeJournal(home string) (int64, string, error) {
	dir := filepath.Join(home, "journal")
	if segs, _ := filepath.Glob(filepath.Join(dir, "*.jsonl")); len(segs) == 0 {
		return 0, "", fmt.Errorf("no journal at %s", dir)
	}
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		return 0, "", fmt.Errorf("open journal: %w", err)
	}
	defer j.Close()
	if err := j.Verify(); err != nil {
		return 0, "", fmt.Errorf("journal chain invalid: %w", err)
	}
	seq, hash := j.Head()
	return seq, hash, nil
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
