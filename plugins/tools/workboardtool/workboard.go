// SPDX-License-Identifier: MIT

// Package workboardtool is the agent-facing wrapper over kernel/workboard:
// durable, typed work items that agents can list, create, claim, block,
// complete, comment on, and link to artifacts/workflows/runs.
package workboardtool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/workboard"
)

type Kernel interface {
	Workboard() *workboard.Store
	CreateWorkboardTask(corr string, spec workboard.CreateSpec) (workboard.Task, bool, error)
	ClaimWorkboardTask(corr, id, agent, runID string) (workboard.Task, error)
	HeartbeatWorkboardTask(corr, id, agent, runID string) (workboard.Task, error)
	CommentWorkboardTask(corr, id, author, body string) (workboard.Task, error)
	BlockWorkboardTask(corr, id, actor, reason string) (workboard.Task, error)
	FailWorkboardTask(corr, id, actor, reason string) (workboard.Task, workboard.RetryDecision, error)
	UnblockWorkboardTask(corr, id, actor string) (workboard.Task, error)
	CompleteWorkboardTask(corr, id, actor string) (workboard.Task, error)
	ArchiveWorkboardTask(corr, id, actor string) (workboard.Task, error)
	LinkWorkboardTask(corr, id, typ, target string) (workboard.Task, error)
	SetWorkboardRetryPolicy(corr, id, actor string, policy *workboard.RetryPolicy) (workboard.Task, error)
	AddWorkboardDependency(corr, id, dependsOn string) (workboard.Task, error)
	ReclaimStaleWorkboardTask(corr, id, actor string, staleAfter time.Duration) (workboard.Task, error)
}

type Tool struct {
	mu sync.RWMutex
	k  Kernel
}

func New() *Tool { return &Tool{} }

func (t *Tool) Bind(k Kernel) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.k = k
}

func (t *Tool) current() Kernel {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.k
}

func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "workboard",
		Description: "Use AGEZT's durable typed workboard: list/show tasks, create new tasks, claim work, heartbeat while running, comment, block/unblock, complete, archive, link runs/artifacts/workflows, declare dependencies, and reclaim stale claims. " +
			"Tasks are not agents; they are visible durable work records with status, priority, assignee, tenant, idempotency key, comments, links, claims, and journaled transitions. " +
			"Use this when work must survive restarts, be picked up by another agent, or be reviewed later instead of hiding it in chat.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["op"],
  "properties": {
    "op": {"type":"string", "enum":["list","show","create","claim","heartbeat","comment","block","fail","unblock","complete","archive","link","policy","depend","reclaim"]},
    "id": {"type":"string", "description":"Task id for show/claim/heartbeat/comment/block/fail/unblock/complete/archive/link/policy/depend/reclaim."},
    "title": {"type":"string", "description":"For create: task title."},
    "description": {"type":"string", "description":"For create: task details."},
    "status": {"type":"string", "enum":["triage","todo","ready","running","blocked","review","done","archived"], "description":"For list filter or create initial status."},
    "priority": {"type":"integer", "description":"For create: higher sorts first."},
    "tenant": {"type":"string", "description":"For create/list: tenant label."},
    "assignee": {"type":"string", "description":"For create/list: assigned agent/person."},
    "owner": {"type":"string", "description":"For create: owner. Defaults to the acting agent when available."},
    "idempotency_key": {"type":"string", "description":"For create: prevents duplicate tasks for the same external/job key."},
    "tags": {"type":"array", "items":{"type":"string"}},
    "artifacts": {"type":"array", "items":{"type":"string"}},
    "agent": {"type":"string", "description":"For claim/heartbeat. Defaults to the acting agent when available."},
    "run_id": {"type":"string", "description":"For claim/heartbeat. Defaults to the current run correlation when available."},
    "body": {"type":"string", "description":"For comment."},
    "reason": {"type":"string", "description":"For block/fail."},
    "type": {"type":"string", "description":"For link: link type, e.g. run, workflow, artifact, url, depends_on."},
    "target": {"type":"string", "description":"For link: linked id/ref/url."},
    "depends_on": {"type":"string", "description":"For depend: prerequisite task id that must complete before this task is dispatched."},
    "max_attempts": {"type":"integer", "description":"For create/policy: max Workboard attempts before escalation/blocking."},
    "escalate_to": {"type":"string", "description":"For create/policy: operator, owner, or agent to escalate to when attempts are exhausted."},
    "clear": {"type":"boolean", "description":"For policy: clear the task retry policy."},
    "stale_after_sec": {"type":"integer", "description":"For reclaim: reclaim only if the current claim heartbeat is older than this many seconds."},
    "limit": {"type":"integer", "description":"For list: max tasks, default 20, max 100."},
    "include_archived": {"type":"boolean", "description":"For list: include archived tasks."}
  }
}`),
		Effect: agent.ToolEffect{
			Class: agent.EffectCompensable,
			PredictedEffects: []string{
				"Read or mutate durable typed workboard tasks.",
				"Created and mutated tasks are persisted and journaled as workboard.task.* events.",
			},
			AffectedResources: []string{"workboard task store", "task claims", "task comments", "task links"},
			RollbackNotes:     "Workboard mutations are compensable by adding corrective comments, moving status, archiving duplicate tasks, or creating follow-up tasks; exact per-field rollback is not yet exposed.",
			Confidence:        0.8,
		},
	}
}

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
