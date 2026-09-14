// SPDX-License-Identifier: MIT

// Package main: backup read/build helpers (inspectBackup + createBackup +
// writeTarFile). inspectBackup parses a tar.gz backup; createBackup writes
// a fresh tar.gz of the data tree; writeTarFile is the tar-entry writer.
// Extracted from backup.go during the Day-211 god-file split. Public API
// unchanged.
package main


import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/agezt/agezt/internal/brand"
)

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
