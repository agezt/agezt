// SPDX-License-Identifier: MIT

package workboard

// The board's RUNNER: the CONTRACT's port, implemented over the board's own
// service.
//
// The contract (kernel/contract/workboardapi.Runner) is the surface the
// agent-facing plugin speaks, and it names the board's operations the way the
// board's users say them: create a workboard task, claim a workboard task,
// comment a workboard task. The DAEMON used to implement that port — as
// sixteen forwarders over the module's own service, one per operation.
//
// A forwarder over a module's own operation is a second spelling of it, and
// the daemon held a second copy of the whole board surface for the plugin and
// the application layer to call through. The board's operations are the
// MODULE's; the module implements the contract's port over them.
//
// One operation, one name: the contract's names are the port's names, and the
// module answers in them.

import (
	"context"
	"time"

	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/modules/work/api"
)

// runner is the contract's runner port over the board's service.
type runner struct{ svc api.BoardService }

// NewRunner builds the contract's runner port over the board's service.
//
// The composition root builds it: the module's IMPLEMENTATION is not
// importable from the host (a module may import another module's api package
// and nothing else), so the host that names the module is the one that
// assembles the port.
func NewRunner(svc api.BoardService) api.BoardRunner {
	return &runner{svc: svc}
}

func (r *runner) CreateWorkboardTask(corr string, spec workboardapi.CreateSpec) (workboardapi.Task, bool, error) {
	return r.svc.Create(corr, spec)
}
func (r *runner) ClaimWorkboardTask(corr, id, agent, runID string) (workboardapi.Task, error) {
	return r.svc.Claim(corr, id, agent, runID)
}
func (r *runner) HeartbeatWorkboardTask(corr, id, agent, runID string) (workboardapi.Task, error) {
	return r.svc.Heartbeat(corr, id, agent, runID)
}
func (r *runner) CommentWorkboardTask(corr, id, author, body string) (workboardapi.Task, error) {
	return r.svc.Comment(corr, id, author, body)
}
func (r *runner) BlockWorkboardTask(corr, id, actor, reason string) (workboardapi.Task, error) {
	return r.svc.Block(corr, id, actor, reason)
}
func (r *runner) FailWorkboardTask(corr, id, actor, reason string) (workboardapi.Task, workboardapi.RetryDecision, error) {
	return r.svc.Fail(corr, id, actor, reason)
}
func (r *runner) UnblockWorkboardTask(corr, id, actor string) (workboardapi.Task, error) {
	return r.svc.Unblock(corr, id, actor)
}
func (r *runner) CompleteWorkboardTask(corr, id, actor string) (workboardapi.Task, error) {
	return r.svc.Complete(corr, id, actor)
}
func (r *runner) ArchiveWorkboardTask(corr, id, actor string) (workboardapi.Task, error) {
	return r.svc.Archive(corr, id, actor)
}
func (r *runner) LinkWorkboardTask(corr, id, typ, target string) (workboardapi.Task, error) {
	return r.svc.Link(corr, id, typ, target)
}
func (r *runner) SetWorkboardRetryPolicy(corr, id, actor string, policy *workboardapi.RetryPolicy) (workboardapi.Task, error) {
	return r.svc.SetRetryPolicy(corr, id, actor, policy)
}
func (r *runner) AddWorkboardDependency(corr, id, dependsOn string) (workboardapi.Task, error) {
	return r.svc.AddDependency(corr, id, dependsOn)
}
func (r *runner) ReclaimStaleWorkboardTask(corr, id, actor string, staleAfter time.Duration) (workboardapi.Task, error) {
	return r.svc.ReclaimStale(corr, id, actor, staleAfter)
}
func (r *runner) ReviewWorkboardTask(corr, id, actor, summary string) (workboardapi.Task, error) {
	return r.svc.Review(corr, id, actor, summary)
}
func (r *runner) SweepStaleWorkboardClaims(corr, actor string, staleAfter time.Duration, limit int) ([]workboardapi.Task, error) {
	return r.svc.SweepStaleClaims(corr, actor, staleAfter, limit)
}
func (r *runner) ProveTask(ctx context.Context, corr, id, answer string) (workboardapi.Task, error) {
	return r.svc.Prove(ctx, corr, id, answer)
}
