// SPDX-License-Identifier: MIT

package main


// agt rollback command: dispatcher + List + Show subcommands.
// Carved out of rollback.go during the Day 160 god-file split so the
// apply / checkpoint logic and the catalog I/O can live in focused files.
// Public API unchanged.

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
)

const (
	rollbackCatalogVersion       = 1
	rollbackCheckpointKindSkill  = "skill.status"
	rollbackCheckpointKindFlow   = "workflow.snapshot"
	rollbackCheckpointKindFile   = "file.snapshot"
	rollbackCheckpointKindConfig = "config.setting"
	rollbackCatalogRelativePath  = "rollback/checkpoints.json"
	rollbackDefaultApplyTimeout  = 5 * time.Second
	rollbackDefaultRenderTimeFmt = time.RFC3339
)

type rollbackCatalog struct {
	Version     int                  `json:"version"`
	Checkpoints []rollbackCheckpoint `json:"checkpoints"`
}

type rollbackCheckpoint struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	Action       string         `json:"action"`
	RunID        string         `json:"run_id,omitempty"`
	SubjectID    string         `json:"subject_id"`
	SubjectName  string         `json:"subject_name,omitempty"`
	BeforeStatus string         `json:"before_status,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	Before       map[string]any `json:"before,omitempty"`
	CreatedMS    int64          `json:"created_ms"`
	AppliedMS    int64          `json:"applied_ms,omitempty"`
}

func cmdRollback(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return rollbackUsage(stderr)
	}
	switch args[0] {
	case "list", "ls":
		return cmdRollbackList(args[1:], stdout, stderr)
	case "show", "dry-run", "preview":
		return cmdRollbackShow(args[0], args[1:], stdout, stderr)
	case "apply":
		return cmdRollbackApply(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		return rollbackUsage(stdout)
	default:
		fmt.Fprintf(stderr, "%s rollback: unknown subcommand %q\n", brand.CLI, args[0])
		return rollbackUsage(stderr)
	}
}

func rollbackUsage(w io.Writer) int {
	fmt.Fprintf(w, "usage: %s rollback <list|show|dry-run|apply>\n", brand.CLI)
	fmt.Fprintf(w, "  list [--run <id>] [--json]  list local mutation checkpoints\n")
	fmt.Fprintf(w, "  show <checkpoint> [--json]   inspect a checkpoint without changing state\n")
	fmt.Fprintf(w, "  dry-run <checkpoint> [--json] preview the restore target\n")
	fmt.Fprintf(w, "  apply <checkpoint> [--json]  restore the checkpointed state and mark it applied\n")
	return 0
}

func cmdRollbackList(args []string, stdout, stderr io.Writer) int {
	if rollbackHelpRequested(args) {
		fmt.Fprintf(stdout, "usage: %s rollback list [--run <id>] [--json]\n", brand.CLI)
		return 0
	}
	asJSON, runID, ok := parseRollbackListArgs(args, stdout, stderr)
	if !ok {
		return 2
	}
	cat, err := loadRollbackCatalog()
	if err != nil {
		fmt.Fprintf(stderr, "%s rollback list: %v\n", brand.CLI, err)
		return 1
	}
	checkpoints := append([]rollbackCheckpoint(nil), cat.Checkpoints...)
	if checkpoints == nil {
		checkpoints = []rollbackCheckpoint{}
	}
	if runID != "" {
		filtered := checkpoints[:0]
		for _, cp := range checkpoints {
			if cp.RunID == runID {
				filtered = append(filtered, cp)
			}
		}
		checkpoints = filtered
	}
	sort.SliceStable(checkpoints, func(i, j int) bool {
		return checkpoints[i].CreatedMS > checkpoints[j].CreatedMS
	})
	out := map[string]any{"checkpoints": checkpoints, "count": len(checkpoints)}
	if runID != "" {
		out["run_id"] = runID
	}
	if asJSON {
		return jsonout.Write(stdout, out)
	}
	if len(checkpoints) == 0 {
		if runID != "" {
			fmt.Fprintf(stdout, "rollback: no checkpoints for run %s\n", runID)
		} else {
			fmt.Fprintf(stdout, "rollback: no checkpoints\n")
		}
		return 0
	}
	if runID != "" {
		fmt.Fprintf(stdout, "rollback checkpoints for run %s:\n", runID)
	} else {
		fmt.Fprintf(stdout, "rollback checkpoints:\n")
	}
	for _, cp := range checkpoints {
		renderRollbackCheckpointLine(stdout, cp)
	}
	return 0
}

func cmdRollbackShow(cmd string, args []string, stdout, stderr io.Writer) int {
	if rollbackHelpRequested(args) {
		fmt.Fprintf(stdout, "usage: %s rollback %s <checkpoint> [--json]\n", brand.CLI, cmd)
		return 0
	}
	id, asJSON, ok := parseRollbackIDJSON(args, cmd, stdout, stderr)
	if !ok {
		return 2
	}
	cat, err := loadRollbackCatalog()
	if err != nil {
		fmt.Fprintf(stderr, "%s rollback %s: %v\n", brand.CLI, cmd, err)
		return 1
	}
	_, cp := findRollbackCheckpoint(cat, id)
	if cp == nil {
		fmt.Fprintf(stderr, "%s rollback %s: checkpoint %s not found\n", brand.CLI, cmd, id)
		return 3
	}
	out := map[string]any{"checkpoint": *cp, "dry_run": true}
	if asJSON {
		return jsonout.Write(stdout, out)
	}
	renderRollbackCheckpoint(stdout, *cp)
	return 0
}

