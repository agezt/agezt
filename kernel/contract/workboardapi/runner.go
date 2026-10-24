// SPDX-License-Identifier: MIT

package workboardapi

import "time"

// Runner is the port the workboard tool drives: the durable, journaled
// runtime path for typed work items — the same path the CLI and the control
// plane use, instead of hiding state in chat.
//
// The tool is a PLUGIN, and the registry hands it the port — so the port
// lives in the contract layer, not in the tool's package (W3.39c). The rows
// it names are the contract's own: `Task`, `CreateSpec`, `RetryPolicy` and
// `RetryDecision` are the same types the module keeps.
//
// It is a port and not the store: the store is the read side
// (`Store.Get`, `Store.List`), and this is the move-and-claim side. The tool
// needs both, and the daemon hands over both.
type Runner interface {
	// CreateWorkboardTask creates a task. The bool is false when the
	// reference already names another task.
	CreateWorkboardTask(corr string, spec CreateSpec) (Task, bool, error)
	// ClaimWorkboardTask takes a task for an agent.
	ClaimWorkboardTask(corr, id, agent, runID string) (Task, error)
	// HeartbeatWorkboardTask reports that the claimed task is still alive.
	HeartbeatWorkboardTask(corr, id, agent, runID string) (Task, error)
	// CommentWorkboardTask appends a comment to a task.
	CommentWorkboardTask(corr, id, author, body string) (Task, error)
	// BlockWorkboardTask blocks a task and says why.
	BlockWorkboardTask(corr, id, actor, reason string) (Task, error)
	// FailWorkboardTask fails a task and says why; the decision says
	// whether it will be retried.
	FailWorkboardTask(corr, id, actor, reason string) (Task, RetryDecision, error)
	// UnblockWorkboardTask releases a blocked task.
	UnblockWorkboardTask(corr, id, actor string) (Task, error)
	// CompleteWorkboardTask finishes a task.
	CompleteWorkboardTask(corr, id, actor string) (Task, error)
	// ArchiveWorkboardTask archives a finished task.
	ArchiveWorkboardTask(corr, id, actor string) (Task, error)
	// LinkWorkboardTask links a task to a typed target.
	LinkWorkboardTask(corr, id, typ, target string) (Task, error)
	// SetWorkboardRetryPolicy sets (or clears) a task's retry policy.
	SetWorkboardRetryPolicy(corr, id, actor string, policy *RetryPolicy) (Task, error)
	// AddWorkboardDependency makes a task depend on another one.
	AddWorkboardDependency(corr, id, dependsOn string) (Task, error)
	// ReclaimStaleWorkboardTask takes back a task whose worker went quiet.
	ReclaimStaleWorkboardTask(corr, id, actor string, staleAfter time.Duration) (Task, error)
}
