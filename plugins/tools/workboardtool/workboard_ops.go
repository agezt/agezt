// SPDX-License-Identifier: MIT
//
// Workboard tool: list + create (the list/create handlers) +
// taskOrError + taskDecisionOrError (the result wrappers) +
// retryPolicyFromInput (the retry policy mapper) + taskView (the wire
// serializer) + okJSON + errResult (the result constructors).
// Extracted from workboard.go during the Day-206 god-file split.
// Public API unchanged.
package workboardtool

import (
	"encoding/json"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/workboard"
)

func (t *Tool) list(k Kernel, in input) (agent.Result, error) {
	var st workboard.Status
	if strings.TrimSpace(in.Status) != "" {
		parsed, err := workboard.ParseStatus(in.Status)
		if err != nil {
			return errResult(err.Error()), nil
		}
		st = parsed
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	tasks := k.Workboard().List(workboard.Filter{
		Status:          st,
		Tenant:          strings.TrimSpace(in.Tenant),
		Assignee:        strings.TrimSpace(in.Assignee),
		IncludeArchived: in.IncludeArchived,
		Limit:           limit,
	})
	out := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, taskView(task))
	}
	return okJSON(map[string]any{"count": len(out), "tasks": out}), nil
}

func (t *Tool) create(k Kernel, corr string, in input) (agent.Result, error) {
	var st workboard.Status
	if strings.TrimSpace(in.Status) != "" {
		parsed, err := workboard.ParseStatus(in.Status)
		if err != nil {
			return errResult(err.Error()), nil
		}
		st = parsed
	}
	task, created, err := k.CreateWorkboardTask(corr, workboard.CreateSpec{
		Title:          in.Title,
		Description:    in.Description,
		Status:         st,
		Priority:       in.Priority,
		Tenant:         in.Tenant,
		Assignee:       in.Assignee,
		Owner:          in.Owner,
		IdempotencyKey: in.IdempotencyKey,
		Tags:           in.Tags,
		Artifacts:      in.Artifacts,
		RetryPolicy:    retryPolicyFromInput(in),
	})
	if err != nil {
		return errResult(err.Error()), nil
	}
	return okJSON(map[string]any{"created": created, "task": taskView(task)}), nil
}

func taskOrError(action string, task workboard.Task, err error) (agent.Result, error) {
	if err != nil {
		return errResult(err.Error()), nil
	}
	return okJSON(map[string]any{"action": action, "task": taskView(task)}), nil
}

func taskDecisionOrError(action string, task workboard.Task, decision workboard.RetryDecision, err error) (agent.Result, error) {
	if err != nil {
		return errResult(err.Error()), nil
	}
	return okJSON(map[string]any{"action": action, "task": taskView(task), "decision": decision}), nil
}

func retryPolicyFromInput(in input) *workboard.RetryPolicy {
	if in.MaxAttempts <= 0 && strings.TrimSpace(in.EscalateTo) == "" {
		return nil
	}
	return &workboard.RetryPolicy{MaxAttempts: in.MaxAttempts, EscalateTo: in.EscalateTo}
}

func taskView(t workboard.Task) map[string]any {
	v := map[string]any{
		"id":         t.ID,
		"title":      t.Title,
		"status":     string(t.Status),
		"priority":   t.Priority,
		"assignee":   t.Assignee,
		"tenant":     t.Tenant,
		"owner":      t.Owner,
		"created_ms": t.CreatedMS,
		"updated_ms": t.UpdatedMS,
	}
	if t.Description != "" {
		v["description"] = t.Description
	}
	if t.IdempotencyKey != "" {
		v["idempotency_key"] = t.IdempotencyKey
	}
	if len(t.Tags) > 0 {
		v["tags"] = append([]string(nil), t.Tags...)
	}
	if len(t.Artifacts) > 0 {
		v["artifacts"] = append([]string(nil), t.Artifacts...)
	}
	if t.RetryPolicy != nil {
		v["retry_policy"] = t.RetryPolicy
	}
	if failures := workboard.FailedAttemptCount(t); failures > 0 {
		v["failed_attempt_count"] = failures
	}
	if len(t.Dependencies) > 0 {
		v["dependencies"] = t.Dependencies
	}
	if t.Claim != nil {
		v["claim"] = t.Claim
	}
	if len(t.Attempts) > 0 {
		v["attempts"] = t.Attempts
	}
	if len(t.Comments) > 0 {
		v["comments"] = t.Comments
	}
	if len(t.Links) > 0 {
		v["links"] = t.Links
	}
	if t.BlockReason != "" {
		v["block_reason"] = t.BlockReason
	}
	if t.CompletedMS > 0 {
		v["completed_ms"] = t.CompletedMS
	}
	if t.ArchivedMS > 0 {
		v["archived_ms"] = t.ArchivedMS
	}
	return v
}

func okJSON(v any) agent.Result {
	enc, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error())
	}
	return agent.Result{Output: string(enc)}
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "workboard: " + msg, IsError: true}
}
