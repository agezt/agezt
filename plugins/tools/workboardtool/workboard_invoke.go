// SPDX-License-Identifier: MIT
//
// Workboard tool: the input struct + Invoke (the dispatched entry) +
// applyContextDefaults (the input normalizer).
// Extracted from workboard.go during the Day-206 god-file split.
// Public API unchanged.
package workboardtool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/workboard"
)

type input struct {
	Op              string   `json:"op"`
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	Status          string   `json:"status"`
	Priority        int      `json:"priority"`
	Tenant          string   `json:"tenant"`
	Assignee        string   `json:"assignee"`
	Owner           string   `json:"owner"`
	IdempotencyKey  string   `json:"idempotency_key"`
	Tags            []string `json:"tags"`
	Artifacts       []string `json:"artifacts"`
	Agent           string   `json:"agent"`
	RunID           string   `json:"run_id"`
	Body            string   `json:"body"`
	Reason          string   `json:"reason"`
	Type            string   `json:"type"`
	Target          string   `json:"target"`
	DependsOn       string   `json:"depends_on"`
	MaxAttempts     int      `json:"max_attempts"`
	EscalateTo      string   `json:"escalate_to"`
	Clear           bool     `json:"clear"`
	StaleAfterSec   int      `json:"stale_after_sec"`
	Limit           int      `json:"limit"`
	IncludeArchived bool     `json:"include_archived"`
}

func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("workboard: parse input: %w", err)
	}
	k := t.current()
	if k == nil || k.Workboard() == nil {
		return errResult("workboard is not available on this daemon"), nil
	}
	corr := agent.CorrelationFromContext(ctx)
	actor := strings.TrimSpace(agent.AgentFromContext(ctx))
	in.applyContextDefaults(actor, corr)

	switch strings.ToLower(strings.TrimSpace(in.Op)) {
	case "list":
		return t.list(k, in)
	case "show":
		if strings.TrimSpace(in.ID) == "" {
			return errResult(`op=show needs "id"`), nil
		}
		task, found := k.Workboard().Get(strings.TrimSpace(in.ID))
		if !found {
			return errResult("no workboard task " + in.ID), nil
		}
		return okJSON(map[string]any{"task": taskView(task)}), nil
	case "create":
		return t.create(k, corr, in)
	case "claim":
		task, err := k.ClaimWorkboardTask(corr, in.ID, in.Agent, in.RunID)
		return taskOrError("claimed", task, err)
	case "heartbeat":
		task, err := k.HeartbeatWorkboardTask(corr, in.ID, in.Agent, in.RunID)
		return taskOrError("heartbeat", task, err)
	case "comment":
		task, err := k.CommentWorkboardTask(corr, in.ID, in.Owner, in.Body)
		return taskOrError("commented", task, err)
	case "block":
		task, err := k.BlockWorkboardTask(corr, in.ID, in.Owner, in.Reason)
		return taskOrError("blocked", task, err)
	case "fail":
		task, decision, err := k.FailWorkboardTask(corr, in.ID, in.Owner, in.Reason)
		return taskDecisionOrError("failed", task, decision, err)
	case "unblock":
		task, err := k.UnblockWorkboardTask(corr, in.ID, in.Owner)
		return taskOrError("unblocked", task, err)
	case "complete":
		task, err := k.CompleteWorkboardTask(corr, in.ID, in.Owner)
		return taskOrError("completed", task, err)
	case "archive":
		task, err := k.ArchiveWorkboardTask(corr, in.ID, in.Owner)
		return taskOrError("archived", task, err)
	case "link":
		task, err := k.LinkWorkboardTask(corr, in.ID, in.Type, in.Target)
		return taskOrError("linked", task, err)
	case "policy":
		var policy *workboard.RetryPolicy
		if !in.Clear {
			policy = &workboard.RetryPolicy{MaxAttempts: in.MaxAttempts, EscalateTo: in.EscalateTo}
		}
		task, err := k.SetWorkboardRetryPolicy(corr, in.ID, in.Owner, policy)
		return taskOrError("retry_policy", task, err)
	case "depend":
		task, err := k.AddWorkboardDependency(corr, in.ID, in.DependsOn)
		return taskOrError("dependency_added", task, err)
	case "reclaim":
		staleAfter := time.Duration(in.StaleAfterSec) * time.Second
		if staleAfter <= 0 {
			staleAfter = 10 * time.Minute
		}
		task, err := k.ReclaimStaleWorkboardTask(corr, in.ID, in.Owner, staleAfter)
		return taskOrError("reclaimed", task, err)
	case "":
		return errResult("op required (list|show|create|claim|heartbeat|comment|block|fail|unblock|complete|archive|link|policy|depend|reclaim)"), nil
	default:
		return errResult("unknown op " + in.Op), nil
	}
}

func (in *input) applyContextDefaults(actor, corr string) {
	if strings.TrimSpace(in.Owner) == "" {
		in.Owner = actor
	}
	if strings.TrimSpace(in.Agent) == "" {
		in.Agent = actor
	}
	if strings.TrimSpace(in.RunID) == "" {
		in.RunID = corr
	}
}
