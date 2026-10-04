// SPDX-License-Identifier: MIT

package files

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/rollbackstore"
)

const (
	RestoreOperation = "file_restore"
	MarkFailed       = "file_restore_mark_failed"
)

// ApplyRestore resolves trusted checkpoint data from the daemon-owned catalog.
// Only identity/path/existence metadata reaches the invocation audit; snapshot
// bytes remain private to the adapter. AppliedMS changes after successful restore.
func ApplyRestore(ctx context.Context, runner Runner, corr, callID, catalogPath, id string) (map[string]any, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, &Error{Code: InvalidPath, Cause: errors.New("checkpoint id required")}
	}
	cat, err := rollbackstore.LoadAt(catalogPath)
	if err != nil {
		return nil, &Error{Code: IOFailure, Cause: err}
	}
	index, checkpoint := rollbackstore.Find(cat, id)
	if checkpoint == nil {
		return nil, &Error{Code: NotFound, Cause: errors.New("checkpoint not found")}
	}
	if checkpoint.Kind != rollbackstore.KindFile {
		return nil, &Error{Code: InvalidPath, Cause: fmt.Errorf("checkpoint kind %q is not a file snapshot", checkpoint.Kind)}
	}
	if checkpoint.AppliedMS > 0 {
		return map[string]any{"checkpoint": *checkpoint, "applied": false, "reason": "already applied"}, nil
	}
	adapter := &snapshotRestore{checkpoint: *checkpoint}
	exists, _ := checkpoint.Before["exists"].(bool)
	raw, err := json.Marshal(map[string]any{
		"checkpoint_id": checkpoint.ID, "path": rollbackstore.StringValue(checkpoint.Before["abs_path"]), "exists": exists,
	})
	if err != nil {
		return nil, err
	}
	result, err := runner.RunToolWithLookup(ctx, corr, callID, RestoreOperation, raw, adapter)
	if err != nil {
		code := IOFailure
		if strings.HasPrefix(result.Output, "tool call denied by policy:") {
			code = Denied
		}
		return nil, &Error{Code: code, Cause: err}
	}
	if result.IsError {
		return nil, &Error{Code: IOFailure, Cause: errors.New(result.Output)}
	}
	var restored map[string]any
	if err := json.Unmarshal([]byte(result.Output), &restored); err != nil {
		return nil, &Error{Code: Unavailable, Cause: err}
	}
	cat.Checkpoints[index].AppliedMS = time.Now().UnixMilli()
	if err := rollbackstore.WriteAt(catalogPath, cat); err != nil {
		return nil, &Error{Code: MarkFailed, Cause: fmt.Errorf("mark applied: %w", err)}
	}
	return map[string]any{"checkpoint": cat.Checkpoints[index], "applied": true, "result": restored}, nil
}

type snapshotRestore struct{ checkpoint rollbackstore.Checkpoint }

func (s *snapshotRestore) Definition() toolapi.ToolDef {
	capability := "file.write"
	if exists, _ := s.checkpoint.Before["exists"].(bool); !exists {
		capability = "file.delete"
	}
	return toolapi.ToolDef{
		Name: RestoreOperation, Capability: toolapi.ToolCapability{Name: capability},
		Description: "Restore a trusted operator file snapshot.", InputSchema: json.RawMessage(`{"type":"object"}`),
		Effect: toolapi.ToolEffect{
			Class: toolapi.EffectIrreversible, PredictedEffects: []string{"restore file snapshot", "mark checkpoint applied"},
			AffectedResources: []string{rollbackstore.StringValue(s.checkpoint.Before["abs_path"]), "rollback checkpoint catalog"},
			RollbackNotes:     "Restoring a snapshot replaces current file state.", Confidence: 1,
		},
	}
}

func (s *snapshotRestore) LookupTool(name string) (toolapi.Tool, bool) {
	if name != RestoreOperation {
		return nil, false
	}
	return s, true
}

func (s *snapshotRestore) Invoke(ctx context.Context, _ json.RawMessage) (toolapi.Result, error) {
	if err := ctx.Err(); err != nil {
		return toolapi.Result{}, err
	}
	result, err := rollbackstore.RestoreFile(s.checkpoint)
	if err != nil {
		return toolapi.Result{}, err
	}
	encoded, err := json.Marshal(result)
	return toolapi.Result{Output: string(encoded)}, err
}
