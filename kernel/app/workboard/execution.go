// SPDX-License-Identifier: MIT

package workboard

import (
	"context"
	"fmt"
	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/seat"
	"github.com/agezt/agezt/kernel/workboard"
	"strings"
)

type ExecutionHost interface {
	CommentWorkboardTask(string, string, string, string) (workboard.Task, error)
	FailWorkboardTask(string, string, string, string) (workboard.Task, workboard.RetryDecision, error)
	BlockWorkboardTask(string, string, string, string) (workboard.Task, error)
	ClaimWorkboardTask(string, string, string, string) (workboard.Task, error)
	ProveTask(context.Context, string, string, string) (workboard.Task, error)
	ReviewWorkboardTask(string, string, string, string) (workboard.Task, error)
}
type ExecutionSeats interface {
	Get(string) (seat.Seat, bool)
}
type ExecutionTasks interface {
	Get(string) (workboard.Task, bool)
}

// Context ports bind the selected agent and native execution context without an
// app-layer dependency on runtime. The app owns seat selection and degradation.
type ExecutionContext interface {
	Agent(context.Context, string, string) context.Context
	Models(context.Context, []string) context.Context
	Tools(context.Context, []string) context.Context
	Isolation(context.Context, string) (context.Context, string, error)
}
type ExecutionAgent struct {
	Slug             string `json:"slug"`
	ExecutionProfile string `json:"execution_profile,omitempty"`
}
type ExecutionInput struct {
	CorrelationID string         `json:"correlation_id"`
	Agent         ExecutionAgent `json:"agent"`
	Task          workboard.Task `json:"task"`
	Intent        string         `json:"intent"`
	Reason        string         `json:"reason"`
}
type ExecutionOutput struct {
	Task  Record `json:"task"`
	Phase string `json:"phase"`
}
type Execution struct {
	host    ExecutionHost
	seats   ExecutionSeats
	tasks   ExecutionTasks
	context ExecutionContext
	run     func(context.Context, string, string) (string, error)
	publish func(string, workboard.Task, string, string, string, string, string)
}

func NewExecution(host ExecutionHost, seats ExecutionSeats, tasks ExecutionTasks, ctx ExecutionContext, run func(context.Context, string, string) (string, error), publish func(string, workboard.Task, string, string, string, string, string)) *Execution {
	return &Execution{host: host, seats: seats, tasks: tasks, context: ctx, run: run, publish: publish}
}
func executionSummary(s string, n int) string {
	return strutil.Ellipsis(strings.TrimSpace(s), n, "…")
}

func (s *Execution) Run(ctx context.Context, in ExecutionInput) (ExecutionOutput, error) {
	corr, p, task, intent, reason := in.CorrelationID, in.Agent, in.Task, in.Intent, in.Reason
	ctx = s.context.Agent(ctx, reason, task.ID)
	// Execution seat: refine HOW this task runs (model tier, tool tier, isolation
	// surface) on top of the assigned agent. Applied after WithAgentProfile so the
	// seat overrides where it sets an axis and the agent supplies the rest.
	// Isolation resolves seat-over-agent: the task's seat wins if it pins one,
	// otherwise the agent's own default execution profile applies.
	isoProfile := strings.TrimSpace(p.ExecutionProfile)
	isoSource := "agent"
	if seatID := strings.TrimSpace(task.Seat); seatID != "" && !strings.EqualFold(seatID, "default") {
		st, ok := s.seats.Get(seatID)
		if !ok {
			// A seat that was removed or mistyped shouldn't vanish silently.
			_, _ = s.host.CommentWorkboardTask(corr, task.ID, "workboard", fmt.Sprintf("seat %q is unknown — running with agent defaults", seatID))
		} else {
			if len(st.ModelChain) > 0 {
				ctx = s.context.Models(ctx, st.ModelChain)
			}
			if st.RestrictTools {
				ctx = s.context.Tools(ctx, st.Tools)
			}
			if st.ExecutionProfile != "" {
				isoProfile = st.ExecutionProfile
				isoSource = "seat " + st.ID
			}
		}
	}
	if isoProfile != "" {
		if nctx, _, perr := s.context.Isolation(ctx, isoProfile); perr != nil {
			// Degrade rather than fail: run with tool defaults and record why.
			_, _ = s.host.CommentWorkboardTask(corr, task.ID, "workboard", fmt.Sprintf("%s isolation %q unavailable (%v) — running with tool defaults", isoSource, isoProfile, perr))
		} else {
			ctx = nctx
		}
	}
	var (
		answer string
		err    error
	)
	answer, err = s.run(ctx, corr, intent)
	if err != nil {
		failed, decision, failErr := s.host.FailWorkboardTask(corr, task.ID, p.Slug, "dispatch failed: "+err.Error())
		if failErr != nil {
			_, _ = s.host.BlockWorkboardTask(corr, task.ID, p.Slug, "dispatch failed: "+err.Error())
			s.publish(corr, task, "failed", p.Slug, reason, "", err.Error())
			return ExecutionOutput{Task: Project(task), Phase: "failed"}, err
		}
		task = failed
		s.publish(corr, task, decision.Action, p.Slug, reason, "", err.Error())
		if decision.Retry {
			claimed, claimErr := s.host.ClaimWorkboardTask(corr, task.ID, p.Slug, corr)
			if claimErr != nil {
				_, _ = s.host.BlockWorkboardTask(corr, task.ID, p.Slug, "retry claim failed: "+claimErr.Error())
				s.publish(corr, task, "failed", p.Slug, reason, "", claimErr.Error())
				return ExecutionOutput{Task: Project(task), Phase: "failed"}, err
			}
			s.publish(corr, claimed, "retrying", p.Slug, reason, "", "")
			return s.Run(context.Background(), ExecutionInput{CorrelationID: corr, Agent: p, Task: claimed, Intent: intent, Reason: reason})
		}
		return ExecutionOutput{Task: Project(task), Phase: decision.Action}, err
	}
	if current, found := s.tasks.Get(task.ID); found && current.Status == workboard.StatusRunning && current.Claim != nil && current.Claim.RunID == corr {
		if len(current.Criteria) > 0 {
			// Proof loop: judge the answer against the task's acceptance criteria.
			// A satisfying proof drives the task to done; otherwise it parks in
			// review with the gap. Fall back to a plain review if proving errors.
			if proved, perr := s.host.ProveTask(ctx, corr, task.ID, answer); perr == nil {
				task = proved
			} else {
				task, _ = s.host.ReviewWorkboardTask(corr, task.ID, p.Slug, executionSummary(answer, 240))
			}
		} else {
			task, _ = s.host.ReviewWorkboardTask(corr, task.ID, p.Slug, executionSummary(answer, 240))
		}
	} else if found {
		task = current
	}
	s.publish(corr, task, "completed", p.Slug, reason, executionSummary(answer, 300), "")
	return ExecutionOutput{Task: Project(task), Phase: "completed"}, nil
}
