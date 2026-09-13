// SPDX-License-Identifier: MIT
//
// Workboard tool: the Kernel interface + the Tool struct + the lifecycle
// (New + Bind + current) + Definition (the agent.ToolDef JSON schema).
// The Invoke entry point lives in workboard_invoke.go; the actual
// operations + helpers live in workboard_ops.go.
// Extracted from workboard.go during the Day-206 god-file split.
// Public API unchanged.
package workboardtool

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
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
		Name:       "workboard",
		Capability: agent.ToolCapability{Name: string(edict.CapWorkboard)},
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
