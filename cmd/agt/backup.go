// SPDX-License-Identifier: MIT

package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/journal"
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

// inspectBackup reads a gzip+tar backup WITHOUT writing anything, returning its
// manifest and the list of regular-file entries (name + size). It is the
// read-only counterpart to restoreBackup; sizes come from the tar headers, so
// no file body is buffered.
func inspectBackup(r io.Reader) (backupManifest, []backupEntry, error) {
	var man backupManifest
	gz, err := gzip.NewReader(r)
	if err != nil {
		return man, nil, fmt.Errorf("not a gzip archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	var entries []backupEntry
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return man, entries, fmt.Errorf("read archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if hdr.Name == backupManifestName {
			data, rerr := io.ReadAll(tr)
			if rerr != nil {
				return man, entries, fmt.Errorf("read manifest: %w", rerr)
			}
			if uerr := json.Unmarshal(data, &man); uerr != nil {
				return man, entries, fmt.Errorf("parse manifest: %w", uerr)
			}
			continue
		}
		entries = append(entries, backupEntry{
			Name: hdr.Name, Size: hdr.Size, OK: isAllowedBackupPath(hdr.Name),
		})
	}
	return man, entries, nil
}

// createBackup tars+gzips the include subtrees of home into w, prepending a
// manifest. Returns the manifest. Pure enough to test (no daemon).
func createBackup(home string, w io.Writer, headSeq int64, headHash string, now time.Time) (backupManifest, error) {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)

	var included []string
	for _, d := range backupIncludeDirs {
		full := filepath.Join(home, d)
		if st, err := os.Stat(full); err != nil || !st.IsDir() {
			continue // absent subtree (e.g. no catalog yet) is fine
		}
		included = append(included, d)
	}

	man := backupManifest{
		Tool: brand.CLI, FormatVersion: 1, CreatedUnixMS: now.UnixMilli(),
		Includes: included, JournalHeadSeq: headSeq, JournalHeadHsh: headHash,
	}
	manBytes, _ := json.MarshalIndent(man, "", "  ")
	if err := writeTarFile(tw, backupManifestName, manBytes); err != nil {
		return man, err
	}

	for _, d := range included {
		root := filepath.Join(home, d)
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			rel, rerr := filepath.Rel(home, path)
			if rerr != nil {
				return rerr
			}
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			return writeTarFile(tw, filepath.ToSlash(rel), data)
		})
		if err != nil {
			return man, fmt.Errorf("archive %s: %w", d, err)
		}
	}

	if err := tw.Close(); err != nil {
		return man, err
	}
	if err := gz.Close(); err != nil {
		return man, err
	}
	return man, nil
}

func writeTarFile(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// cmdRestore implements `agt restore <file> [--home <dir>]` (M113) — the
// read-back half: unpack a backup into an EMPTY home (never clobbers an existing
// journal), path-traversal-safe, then confirm the restored journal boots.
func cmdRestore(args []string, stdout, stderr io.Writer) int {
	homeOverride := ""
	archivePath := ""
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
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s restore <file> [--home <dir>]\n", brand.CLI)
			fmt.Fprintf(stdout, "unpack a backup into an EMPTY home; re-provision credentials afterwards\n")
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
