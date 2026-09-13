// SPDX-License-Identifier: MIT

package main

// agt rollback CATALOG I/O + display + argument parsing: appendRollbackCheckpoint,
// rollbackCatalogPath, loadRollbackCatalog(+At), writeRollbackCatalogAt,
// findRollbackCheckpoint, parseRollbackListArgs, parseRollbackIDJSON,
// renderRollbackCheckpointLine, renderRollbackCheckpoint,
// rollbackSubjectLabel, rollbackApplySummary, writeRollbackFile.
// Carved out of rollback.go during the Day 160 god-file split.
// Public API unchanged.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
)
func appendRollbackCheckpoint(cp rollbackCheckpoint) error {
	path, err := rollbackCatalogPath()
	if err != nil {
		return err
	}
	cat, err := loadRollbackCatalogAt(path)
	if err != nil {
		return err
	}
	cat.Checkpoints = append(cat.Checkpoints, cp)
	return writeRollbackCatalogAt(path, cat)
}

func rollbackCatalogPath() (string, error) {
	base, err := paths.BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, filepath.FromSlash(rollbackCatalogRelativePath)), nil
}

func loadRollbackCatalog() (rollbackCatalog, error) {
	path, err := rollbackCatalogPath()
	if err != nil {
		return rollbackCatalog{}, err
	}
	return loadRollbackCatalogAt(path)
}

func loadRollbackCatalogAt(path string) (rollbackCatalog, error) {
	cat := rollbackCatalog{Version: rollbackCatalogVersion}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cat, nil
		}
		return cat, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return cat, nil
	}
	if err := json.Unmarshal(raw, &cat); err != nil {
		return cat, err
	}
	if cat.Version == 0 {
		cat.Version = rollbackCatalogVersion
	}
	return cat, nil
}

func writeRollbackCatalogAt(path string, cat rollbackCatalog) error {
	if cat.Version == 0 {
		cat.Version = rollbackCatalogVersion
	}
	body, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			if removeErr := os.Remove(path); removeErr == nil {
				if retryErr := os.Rename(tmp, path); retryErr == nil {
					return nil
				} else {
					err = retryErr
				}
			}
		}
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func findRollbackCheckpoint(cat rollbackCatalog, id string) (int, *rollbackCheckpoint) {
	for i := range cat.Checkpoints {
		if cat.Checkpoints[i].ID == id {
			return i, &cat.Checkpoints[i]
		}
	}
	return -1, nil
}

func rollbackHelpRequested(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

func parseRollbackListArgs(args []string, stdout, stderr io.Writer) (bool, string, bool) {
	asJSON := false
	runID := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--run":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				fmt.Fprintf(stderr, "%s rollback list: --run requires an id\n", brand.CLI)
				return false, "", false
			}
			if runID != "" {
				fmt.Fprintf(stderr, "%s rollback list: duplicate --run\n", brand.CLI)
				return false, "", false
			}
			i++
			runID = strings.TrimSpace(args[i])
		case strings.HasPrefix(a, "--run="):
			if runID != "" {
				fmt.Fprintf(stderr, "%s rollback list: duplicate --run\n", brand.CLI)
				return false, "", false
			}
			runID = strings.TrimSpace(strings.TrimPrefix(a, "--run="))
			if runID == "" {
				fmt.Fprintf(stderr, "%s rollback list: --run requires an id\n", brand.CLI)
				return false, "", false
			}
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s rollback list: unexpected flag %q\n", brand.CLI, a)
			} else {
				fmt.Fprintf(stderr, "%s rollback list: unexpected arg %q\n", brand.CLI, a)
			}
			return false, "", false
		}
	}
	return asJSON, runID, true
}

func parseRollbackIDJSON(args []string, cmd string, stdout, stderr io.Writer) (string, bool, bool) {
	id := ""
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s rollback %s: unexpected flag %q\n", brand.CLI, cmd, a)
				return "", false, false
			}
			if id != "" {
				fmt.Fprintf(stderr, "%s rollback %s: unexpected arg %q\n", brand.CLI, cmd, a)
				return "", false, false
			}
			id = a
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s rollback %s: checkpoint id required\n", brand.CLI, cmd)
		return "", false, false
	}
	return id, asJSON, true
}

func renderRollbackCheckpointLine(w io.Writer, cp rollbackCheckpoint) {
	status := cp.BeforeStatus
	if status == "" {
		status = "unknown"
	}
	applied := ""
	if cp.AppliedMS > 0 {
		applied = " applied"
	}
	run := ""
	if cp.RunID != "" {
		run = " run=" + cp.RunID
	}
	fmt.Fprintf(w, "  %s  %s%s  %s -> %s%s\n", cp.ID, cp.Kind, run, rollbackSubjectLabel(cp), status, applied)
}

func renderRollbackCheckpoint(w io.Writer, cp rollbackCheckpoint) {
	fmt.Fprintf(w, "checkpoint: %s\n", cp.ID)
	fmt.Fprintf(w, "kind:       %s\n", cp.Kind)
	fmt.Fprintf(w, "action:     %s\n", cp.Action)
	if cp.RunID != "" {
		fmt.Fprintf(w, "run:        %s\n", cp.RunID)
	}
	fmt.Fprintf(w, "subject:    %s\n", rollbackSubjectLabel(cp))
	fmt.Fprintf(w, "created:    %s\n", time.UnixMilli(cp.CreatedMS).Format(rollbackDefaultRenderTimeFmt))
	if cp.Reason != "" {
		fmt.Fprintf(w, "reason:     %s\n", cp.Reason)
	}
	if cp.BeforeStatus != "" {
		fmt.Fprintf(w, "restore:    status -> %s\n", cp.BeforeStatus)
	} else if cp.Kind == rollbackCheckpointKindFlow {
		fmt.Fprintf(w, "restore:    workflow snapshot\n")
	} else if cp.Kind == rollbackCheckpointKindFile {
		if exists, _ := cp.Before["exists"].(bool); exists {
			fmt.Fprintf(w, "restore:    file content snapshot\n")
		} else {
			fmt.Fprintf(w, "restore:    remove file created after checkpoint\n")
		}
	} else if cp.Kind == rollbackCheckpointKindConfig {
		if rollbackable, ok := cp.Before["rollbackable"].(bool); ok && !rollbackable {
			fmt.Fprintf(w, "restore:    audit only (%s)\n", str(cp.Before["non_rollbackable_reason"]))
		} else if set, _ := cp.Before["set"].(bool); set {
			fmt.Fprintf(w, "restore:    config value -> previous value\n")
		} else {
			fmt.Fprintf(w, "restore:    config value -> unset\n")
		}
	}
	if cp.AppliedMS > 0 {
		fmt.Fprintf(w, "applied:    %s\n", time.UnixMilli(cp.AppliedMS).Format(rollbackDefaultRenderTimeFmt))
	}
}

func rollbackSubjectLabel(cp rollbackCheckpoint) string {
	if cp.SubjectName != "" {
		return cp.SubjectName + " (" + shortID(cp.SubjectID) + ")"
	}
	return shortID(cp.SubjectID)
}

func rollbackApplySummary(cp rollbackCheckpoint) string {
	if cp.Kind == rollbackCheckpointKindSkill && cp.BeforeStatus != "" {
		return rollbackSubjectLabel(cp) + " -> " + cp.BeforeStatus
	}
	if cp.Kind == rollbackCheckpointKindFlow {
		return rollbackSubjectLabel(cp) + " workflow snapshot"
	}
	if cp.Kind == rollbackCheckpointKindFile {
		return rollbackSubjectLabel(cp) + " file snapshot"
	}
	if cp.Kind == rollbackCheckpointKindConfig {
		return rollbackSubjectLabel(cp) + " config setting"
	}
	return rollbackSubjectLabel(cp)
}

func writeRollbackFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agezt-rollback-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			if removeErr := os.Remove(path); removeErr == nil {
				if retryErr := os.Rename(tmpName, path); retryErr == nil {
					return nil
				} else {
					err = retryErr
				}
			}
		}
		return err
	}
	return nil
}
