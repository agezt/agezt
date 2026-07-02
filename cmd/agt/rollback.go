// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/controlplane"
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
		return encodeJSON(stdout, out)
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
		return encodeJSON(stdout, out)
	}
	renderRollbackCheckpoint(stdout, *cp)
	return 0
}

func cmdRollbackApply(args []string, stdout, stderr io.Writer) int {
	if rollbackHelpRequested(args) {
		fmt.Fprintf(stdout, "usage: %s rollback apply <checkpoint> [--json]\n", brand.CLI)
		return 0
	}
	id, asJSON, ok := parseRollbackIDJSON(args, "apply", stdout, stderr)
	if !ok {
		return 2
	}
	path, err := rollbackCatalogPath()
	if err != nil {
		fmt.Fprintf(stderr, "%s rollback apply: %v\n", brand.CLI, err)
		return 1
	}
	cat, err := loadRollbackCatalogAt(path)
	if err != nil {
		fmt.Fprintf(stderr, "%s rollback apply: %v\n", brand.CLI, err)
		return 1
	}
	idx, cp := findRollbackCheckpoint(cat, id)
	if cp == nil {
		fmt.Fprintf(stderr, "%s rollback apply: checkpoint %s not found\n", brand.CLI, id)
		return 3
	}
	if cp.AppliedMS > 0 {
		out := map[string]any{"checkpoint": *cp, "applied": false, "reason": "already applied"}
		if asJSON {
			return encodeJSON(stdout, out)
		}
		fmt.Fprintf(stdout, "rollback %s already applied at %s\n", cp.ID, time.UnixMilli(cp.AppliedMS).Format(rollbackDefaultRenderTimeFmt))
		return 0
	}
	reason := fmt.Sprintf("rollback checkpoint %s", cp.ID)
	if cp.Action != "" {
		reason += " (" + cp.Action + ")"
	}
	res, err := applyRollbackCheckpoint(*cp, reason, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "%s rollback apply: %v\n", brand.CLI, err)
		return 1
	}
	cat.Checkpoints[idx].AppliedMS = time.Now().UnixMilli()
	if err := writeRollbackCatalogAt(path, cat); err != nil {
		fmt.Fprintf(stderr, "%s rollback apply: mark applied: %v\n", brand.CLI, err)
		return 1
	}
	cp = &cat.Checkpoints[idx]
	out := map[string]any{"checkpoint": *cp, "applied": true, "result": res}
	if asJSON {
		return encodeJSON(stdout, out)
	}
	fmt.Fprintf(stdout, "rolled back %s: %s\n", cp.ID, rollbackApplySummary(*cp))
	return 0
}

func applyRollbackCheckpoint(cp rollbackCheckpoint, reason string, stderr io.Writer) (map[string]any, error) {
	switch cp.Kind {
	case rollbackCheckpointKindSkill:
		if strings.TrimSpace(cp.SubjectID) == "" || strings.TrimSpace(cp.BeforeStatus) == "" {
			return nil, fmt.Errorf("checkpoint %s is missing skill restore data", cp.ID)
		}
		c := dial(stderr)
		if c == nil {
			return nil, errors.New("daemon unavailable")
		}
		ctx, cancel := context.WithTimeout(context.Background(), rollbackDefaultApplyTimeout)
		defer cancel()
		return c.Call(ctx, controlplane.CmdSkillRestore, map[string]any{
			"id": cp.SubjectID, "status": cp.BeforeStatus, "reason": reason,
		})
	case rollbackCheckpointKindFlow:
		if len(cp.Before) == 0 {
			return nil, fmt.Errorf("checkpoint %s is missing workflow snapshot data", cp.ID)
		}
		c := dial(stderr)
		if c == nil {
			return nil, errors.New("daemon unavailable")
		}
		ctx, cancel := context.WithTimeout(context.Background(), rollbackDefaultApplyTimeout)
		defer cancel()
		return c.Call(ctx, controlplane.CmdWorkflowRestore, map[string]any{
			"workflow": cp.Before, "reason": reason,
		})
	case rollbackCheckpointKindFile:
		return applyFileSnapshotCheckpoint(cp)
	case rollbackCheckpointKindConfig:
		if rollbackable, ok := cp.Before["rollbackable"].(bool); ok && !rollbackable {
			if why := str(cp.Before["non_rollbackable_reason"]); why != "" {
				return nil, fmt.Errorf("checkpoint %s is audit-only: %s", cp.ID, why)
			}
			return nil, fmt.Errorf("checkpoint %s is audit-only", cp.ID)
		}
		env := str(cp.Before["env"])
		if env == "" {
			env = cp.SubjectID
		}
		if env == "" {
			return nil, fmt.Errorf("checkpoint %s is missing config env", cp.ID)
		}
		value := ""
		if set, _ := cp.Before["set"].(bool); set {
			value = str(cp.Before["value"])
		}
		c := dial(stderr)
		if c == nil {
			return nil, errors.New("daemon unavailable")
		}
		ctx, cancel := context.WithTimeout(context.Background(), rollbackDefaultApplyTimeout)
		defer cancel()
		return c.Call(ctx, controlplane.CmdConfigSet, map[string]any{"name": env, "value": value})
	default:
		return nil, fmt.Errorf("checkpoint kind %q is not supported yet", cp.Kind)
	}
}

func applyFileSnapshotCheckpoint(cp rollbackCheckpoint) (map[string]any, error) {
	before := cp.Before
	if len(before) == 0 {
		return nil, fmt.Errorf("checkpoint %s is missing file snapshot data", cp.ID)
	}
	path := str(before["abs_path"])
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("checkpoint %s is missing abs_path", cp.ID)
	}
	exists, _ := before["exists"].(bool)
	if li, err := os.Lstat(path); err == nil {
		if li.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("refusing to restore through symlink at %s", path)
		}
		if li.IsDir() {
			return nil, fmt.Errorf("refusing to restore over directory at %s", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if !exists {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return map[string]any{"path": path, "restored": "absent"}, nil
	}
	encoded := str(before["content_b64"])
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode content_b64: %w", err)
	}
	perm := os.FileMode(0o644)
	if n := intNumber(before["mode_perm"]); n > 0 {
		perm = os.FileMode(n)
	}
	if err := writeRollbackFile(path, data, perm); err != nil {
		return nil, err
	}
	return map[string]any{"path": path, "restored": "content", "bytes": len(data)}, nil
}

func saveSkillStatusRollbackCheckpoint(ctx context.Context, c *controlplane.Client, action, id, reason string) (*rollbackCheckpoint, error) {
	sk, found, err := workshopFetchSkill(ctx, c, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%s not found", id)
	}
	if err := validateSkillStatusCheckpointAction(action, str(sk["status"])); err != nil {
		return nil, err
	}
	cp := newSkillStatusRollbackCheckpoint(action, reason, sk, time.Now())
	if err := appendRollbackCheckpoint(cp); err != nil {
		return nil, err
	}
	return &cp, nil
}

func newSkillStatusRollbackCheckpoint(action, reason string, sk map[string]any, now time.Time) rollbackCheckpoint {
	id := str(sk["id"])
	before := make(map[string]any, len(sk))
	for k, v := range sk {
		before[k] = v
	}
	return rollbackCheckpoint{
		ID:           rollbackCheckpointID(now, id),
		Kind:         rollbackCheckpointKindSkill,
		Action:       action,
		SubjectID:    id,
		SubjectName:  str(sk["name"]),
		BeforeStatus: str(sk["status"]),
		Reason:       reason,
		Before:       before,
		CreatedMS:    now.UnixMilli(),
	}
}

func saveWorkflowSnapshotRollbackCheckpointIfFound(ctx context.Context, c *controlplane.Client, action, ref, reason string) (*rollbackCheckpoint, bool, error) {
	w, found, err := fetchWorkflowSnapshot(ctx, c, ref)
	if err != nil || !found {
		return nil, found, err
	}
	cp := newWorkflowSnapshotRollbackCheckpoint(action, reason, w, time.Now())
	if err := appendRollbackCheckpoint(cp); err != nil {
		return nil, true, err
	}
	return &cp, true, nil
}

func fetchWorkflowSnapshot(ctx context.Context, c *controlplane.Client, ref string) (map[string]any, bool, error) {
	res, err := c.Call(ctx, controlplane.CmdWorkflowShow, map[string]any{"ref": ref})
	if err != nil {
		if strings.Contains(err.Error(), "unknown workflow") {
			return nil, false, nil
		}
		return nil, false, err
	}
	w, _ := res["workflow"].(map[string]any)
	if w == nil {
		return nil, false, nil
	}
	return w, true, nil
}

func newWorkflowSnapshotRollbackCheckpoint(action, reason string, w map[string]any, now time.Time) rollbackCheckpoint {
	id := str(w["id"])
	before := make(map[string]any, len(w))
	for k, v := range w {
		before[k] = v
	}
	return rollbackCheckpoint{
		ID:          rollbackCheckpointID(now, id),
		Kind:        rollbackCheckpointKindFlow,
		Action:      action,
		SubjectID:   id,
		SubjectName: str(w["name"]),
		Reason:      reason,
		Before:      before,
		CreatedMS:   now.UnixMilli(),
	}
}

func saveConfigSettingRollbackCheckpoint(ctx context.Context, c *controlplane.Client, action, env string) (*rollbackCheckpoint, error) {
	fields, err := fetchConfigValueFields(ctx, c)
	if err != nil {
		return nil, err
	}
	for _, raw := range fields {
		m, _ := raw.(map[string]any)
		if m == nil || str(m["env"]) != env {
			continue
		}
		cp := newConfigSettingRollbackCheckpoint(action, m, time.Now())
		if err := appendRollbackCheckpoint(cp); err != nil {
			return nil, err
		}
		return &cp, nil
	}
	return nil, fmt.Errorf("unknown setting %s", env)
}

func fetchConfigValueFields(ctx context.Context, c *controlplane.Client) ([]any, error) {
	res, err := c.Call(ctx, controlplane.CmdConfigValues, nil)
	if err != nil {
		return nil, err
	}
	fields, _ := res["fields"].([]any)
	return fields, nil
}

func newConfigSettingRollbackCheckpoint(action string, field map[string]any, now time.Time) rollbackCheckpoint {
	env := str(field["env"])
	secret, _ := field["secret"].(bool)
	set, _ := field["set"].(bool)
	before := map[string]any{
		"env":          env,
		"secret":       secret,
		"set":          set,
		"env_pinned":   field["env_pinned"],
		"rollbackable": true,
	}
	if secret {
		if set {
			before["rollbackable"] = false
			before["non_rollbackable_reason"] = "previous secret value is masked by the daemon"
		}
	} else {
		before["value"] = str(field["value"])
	}
	return rollbackCheckpoint{
		ID:          rollbackCheckpointID(now, env),
		Kind:        rollbackCheckpointKindConfig,
		Action:      action,
		SubjectID:   env,
		SubjectName: env,
		Before:      before,
		CreatedMS:   now.UnixMilli(),
	}
}

func rollbackCheckpointID(now time.Time, subjectID string) string {
	short := shortID(subjectID)
	if short == "" {
		short = "unknown"
	}
	return fmt.Sprintf("rb-%d-%s", now.UnixNano(), short)
}

func validateSkillStatusCheckpointAction(action, status string) error {
	switch action {
	case "apply":
		if status == "draft" || status == "shadow" || status == "quarantined" {
			return nil
		}
	case "reject":
		if status == "draft" || status == "shadow" {
			return nil
		}
	case "quarantine":
		if status == "active" || status == "shadow" {
			return nil
		}
	case "curate.quarantine":
		if status == "active" {
			return nil
		}
	default:
		return nil
	}
	return fmt.Errorf("%s cannot checkpoint %s skill", action, status)
}

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

func parseRollbackOnlyJSON(args []string, cmd string, stdout, stderr io.Writer) (bool, bool) {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		default:
			fmt.Fprintf(stderr, "%s rollback %s: unexpected arg %q\n", brand.CLI, cmd, a)
			return false, false
		}
	}
	return asJSON, true
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
