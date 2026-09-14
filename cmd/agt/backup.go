// SPDX-License-Identifier: MIT

// Package main: `agt backup` (the CLI dispatcher) + `agt backup inspect` —
// backupManifest + backupEntry types. The read/build helpers (inspectBackup
// + createBackup + writeTarFile) moved to backup_lib.go. Day-211 god-file
// split. Public API unchanged.
package main


import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
)
// backupManifest is the metadata entry written at the root of a backup archive
// (M113). It records what the bundle holds and the journal head at backup time
// so a restore can sanity-check it.
type backupManifest struct {
	Tool           string   `json:"tool"`
	FormatVersion  int      `json:"format_version"`
	CreatedUnixMS  int64    `json:"created_unix_ms"`
	Includes       []string `json:"includes"`
	JournalHeadSeq int64    `json:"journal_head_seq"`
	JournalHeadHsh string   `json:"journal_head_hash"`
}

const backupManifestName = "backup-manifest.json"

// backupIncludeDirs are the home subtrees a backup captures. CRITICAL: only
// non-secret, hard-to-rebuild state is listed. The journal is the source of
// truth; the catalog is network-synced and not in the journal. Secrets
// (creds.json at the home root, runtime/control.token) live OUTSIDE these
// subtrees, so they are excluded by construction — a backup can never leak them.
var backupIncludeDirs = []string{"journal", "catalog"}

// cmdBackup implements `agt backup [--home <dir>] [--out <file>]` (M113) — a
// portable, secret-free snapshot of the home for node migration / archival.
// Runs offline (no daemon). Captures journal/ + catalog/; projections rebuild
// from the journal on the next boot.
func cmdBackup(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "inspect" {
		return cmdBackupInspect(args[1:], stdout, stderr)
	}
	homeOverride := ""
	outPath := "agezt-backup.tar.gz"
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--home":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s backup: --home needs a directory\n", brand.CLI)
				return 2
			}
			i++
			homeOverride = args[i]
		case strings.HasPrefix(a, "--home="):
			homeOverride = strings.TrimPrefix(a, "--home=")
		case a == "--out":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s backup: --out needs a file path\n", brand.CLI)
				return 2
			}
			i++
			outPath = args[i]
		case strings.HasPrefix(a, "--out="):
			outPath = strings.TrimPrefix(a, "--out=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s backup [--home <dir>] [--out <file>]\n", brand.CLI)
			fmt.Fprintf(stdout, "       %s backup inspect <file> [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "write a portable, secret-free home snapshot (journal + catalog) for migration\n")
			fmt.Fprintf(stdout, "  --out <file>  archive path (default agezt-backup.tar.gz)\n")
			fmt.Fprintf(stdout, "  inspect       show a bundle's manifest + contents without restoring it\n")
			fmt.Fprintf(stdout, "secrets (creds, tokens) are never included; run with the daemon stopped\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s backup: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	home, err := resolveHome(homeOverride)
	if err != nil {
		fmt.Fprintf(stderr, "%s backup: %v\n", brand.CLI, err)
		return 1
	}

	// Verify the journal chain before backing up a corrupt one.
	headSeq, headHash, verr := verifyHomeJournal(home)
	if verr != nil {
		fmt.Fprintf(stderr, "%s backup: %v\n", brand.CLI, verr)
		return 1
	}

	f, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(stderr, "%s backup: create %s: %v\n", brand.CLI, outPath, err)
		return 1
	}
	man, werr := createBackup(home, f, headSeq, headHash, time.Now())
	closeErr := f.Close()
	if werr != nil {
		os.Remove(outPath)
		fmt.Fprintf(stderr, "%s backup: %v\n", brand.CLI, werr)
		return 1
	}
	if closeErr != nil {
		fmt.Fprintf(stderr, "%s backup: close %s: %v\n", brand.CLI, outPath, closeErr)
		return 1
	}
	fmt.Fprintf(stdout, "backed up %s → %s\n", strings.Join(man.Includes, " + "), outPath)
	fmt.Fprintf(stdout, "  journal head: seq=%d hash=%s\n", man.JournalHeadSeq, shortHash(man.JournalHeadHsh))
	fmt.Fprintf(stdout, "  restore with: %s restore %s --home <fresh-dir>\n", brand.CLI, outPath)
	return 0
}

// backupEntry is one file recorded inside a backup archive, as surfaced by
// `agt backup inspect`. OK is false for an entry outside the known include
// subtrees (a sign of a tampered or foreign archive).
type backupEntry struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	OK   bool   `json:"within_known_subtree"`
}

// cmdBackupInspect implements `agt backup inspect <file> [--json]` (M266) — an
// OFFLINE read of a backup bundle's manifest and contents WITHOUT unpacking it,
// so an operator can confirm which home/journal-head a bundle holds (and that it
// is not tampered) before restoring it onto a fresh host. Mirrors
// `agt journal verify --bundle` for the whole-home backup format.
func cmdBackupInspect(args []string, stdout, stderr io.Writer) int {
	bundlePath := ""
	asJSON := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s backup inspect <file> [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show a backup bundle's manifest and contents without restoring it\n")
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s backup inspect: unexpected flag %q\n", brand.CLI, a)
			return 2
		default:
			if bundlePath != "" {
				fmt.Fprintf(stderr, "%s backup inspect: unexpected arg %q (one bundle path)\n", brand.CLI, a)
				return 2
			}
			bundlePath = a
		}
	}
	if bundlePath == "" {
		fmt.Fprintf(stderr, "%s backup inspect: a bundle path is required\n", brand.CLI)
		return 2
	}

	f, err := os.Open(bundlePath)
	if err != nil {
		fmt.Fprintf(stderr, "%s backup inspect: open %s: %v\n", brand.CLI, bundlePath, err)
		return 1
	}
	defer f.Close()
	man, entries, ierr := inspectBackup(f)
	if ierr != nil {
		fmt.Fprintf(stderr, "%s backup inspect: %v\n", brand.CLI, ierr)
		return 1
	}

	var total int64
	suspicious := 0
	for _, e := range entries {
		total += e.Size
		if !e.OK {
			suspicious++
		}
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{
			"path":               bundlePath,
			"manifest":           man,
			"entries":            entries,
			"file_count":         len(entries),
			"total_bytes":        total,
			"suspicious_entries": suspicious,
		})
		return 0
	}

	fmt.Fprintf(stdout, "backup:       %s\n", bundlePath)
	tool := man.Tool
	if tool == "" {
		tool = "(unknown)"
	}
	fmt.Fprintf(stdout, "tool:         %s (format v%d)\n", tool, man.FormatVersion)
	if man.CreatedUnixMS > 0 {
		fmt.Fprintf(stdout, "created:      %s\n", time.UnixMilli(man.CreatedUnixMS).Format("2006-01-02 15:04:05"))
	}
	fmt.Fprintf(stdout, "journal head: seq=%d hash=%s\n", man.JournalHeadSeq, shortHash(man.JournalHeadHsh))
	if len(man.Includes) > 0 {
		fmt.Fprintf(stdout, "includes:     %s\n", strings.Join(man.Includes, " + "))
	}
	fmt.Fprintf(stdout, "contents:     %d file(s), %s\n", len(entries), humanBytes(total))

	const maxList = 20
	for i, e := range entries {
		if i >= maxList {
			fmt.Fprintf(stdout, "  ... and %d more\n", len(entries)-maxList)
			break
		}
		flag := ""
		if !e.OK {
			flag = "  (!) unexpected path"
		}
		fmt.Fprintf(stdout, "  %-34s %10s%s\n", e.Name, humanBytes(e.Size), flag)
	}

	if suspicious > 0 {
		fmt.Fprintf(stderr, "%s backup inspect: %d entry(ies) outside the known subtrees — this bundle may be tampered; `%s restore` will refuse it\n",
			brand.CLI, suspicious, brand.CLI)
		return 1
	}
	fmt.Fprintf(stdout, "restore with: %s restore %s --home <fresh-dir>\n", brand.CLI, bundlePath)
	return 0
}
