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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
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
