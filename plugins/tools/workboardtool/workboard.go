package workboardtool

// Provenance: SPDX-License-Identifier: MIT Workboard tool: the Kernel interface +
//             the Tool struct + the lifecycle (New + Bind + current) + Definition
//             (the toolapi.ToolDef JSON schema). The Invoke entry point lives in
//             workboard_invoke.go; the actual operations + helpers live in
//             workboard_ops.go. Extracted from workboard.go during the Day-206
//             god-file split. Public API unchanged.

import (
	"encoding/json"
	"sync"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/edict"
)

// Kernel is the slice of the runtime kernel this tool drives — the durable,
// journaled path for typed work items.
//
// The rows it carries are the contract's (kernel/contract/workboardapi): a
// plugin may reach the contract layer and nothing else, so the tool names
// the row, not the module that keeps it.
//
// The PORT itself is the contract's too (W3.39c): the registry hands the
// runner to the tool, so the port it hands must be a contract row — and the
// name stays here as an alias, so every caller and every fake is unchanged.
type Kernel = workboardapi.Runner

type Tool struct {
	mu sync.RWMutex
	k  Kernel
	// store is the READ side of the board (op=list, op=show). It arrives
	// through the contract's port, because the tool is a plugin and may not
	// reach the module that keeps the store.
	store workboardapi.Store
}

func New() *Tool { return &Tool{} }

// Bind wires the live kernel and the store the read ops read through. Called
// once after the kernel opens.
//
// The store is a SEPARATE port on purpose: the kernel's lifecycle methods
// speak the contract's row, but the store is a concrete implementation that
// lives in the module — a plugin may not name it. A nil store stays a nil
// port, so the tool reports that the board is unavailable instead of calling
// into a nil implementation.
func (t *Tool) Bind(k Kernel, store workboardapi.Store) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.k = k
	t.store = store
}

func (t *Tool) current() Kernel {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.k
}

// currentStore returns the bound store, or nil when it was not wired.
func (t *Tool) currentStore() workboardapi.Store {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.store
}

func (t *Tool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{
		Name:       "workboard",
		Capability: toolapi.ToolCapability{Name: string(edict.CapWorkboard)},
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
		Effect: toolapi.ToolEffect{
			Class: toolapi.EffectCompensable,
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
