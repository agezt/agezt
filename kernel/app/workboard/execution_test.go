// SPDX-License-Identifier: MIT

package workboard_test

import (
	"context"
	"errors"
	appwork "github.com/agezt/agezt/kernel/app/workboard"
	"github.com/agezt/agezt/kernel/proof"
	"github.com/agezt/agezt/kernel/seat"
	tasks "github.com/agezt/agezt/kernel/workboard"
	"reflect"
	"strings"
	"testing"
)

type executionFixture struct {
	trace                               []string
	current                             tasks.Task
	found                               bool
	selected                            seat.Seat
	seatFound                           bool
	failErr, claimErr, proveErr, isoErr error
	decision                            tasks.RetryDecision
	calls, proves, reviews, blocks      int
	publications                        []executionPublication
}
type executionPublication struct {
	phase, answer, err string
	task               tasks.Task
}

func (f *executionFixture) Get(id string) (tasks.Task, bool) { return f.current, f.found }

type executionSeats struct{ f *executionFixture }

func (s executionSeats) Get(id string) (seat.Seat, bool) {
	s.f.trace = append(s.f.trace, "seat:"+id)
	return s.f.selected, s.f.seatFound
}
func (f *executionFixture) CommentWorkboardTask(corr, id, actor, body string) (tasks.Task, error) {
	f.trace = append(f.trace, "comment:"+body)
	return f.current, nil
}
func (f *executionFixture) FailWorkboardTask(corr, id, actor, reason string) (tasks.Task, tasks.RetryDecision, error) {
	f.trace = append(f.trace, "fail:"+reason)
	f.current.Status = tasks.StatusReady
	return f.current, f.decision, f.failErr
}
func (f *executionFixture) BlockWorkboardTask(corr, id, actor, reason string) (tasks.Task, error) {
	f.blocks++
	f.trace = append(f.trace, "block:"+reason)
	return f.current, nil
}
func (f *executionFixture) ClaimWorkboardTask(corr, id, actor, run string) (tasks.Task, error) {
	f.trace = append(f.trace, "claim:"+corr+":"+run)
	f.current.Status = tasks.StatusRunning
	return f.current, f.claimErr
}
func (f *executionFixture) ProveTask(ctx context.Context, corr, id, answer string) (tasks.Task, error) {
	f.proves++
	f.trace = append(f.trace, "prove")
	task := f.current
	task.Status = tasks.StatusDone
	return task, f.proveErr
}
func (f *executionFixture) ReviewWorkboardTask(corr, id, actor, summary string) (tasks.Task, error) {
	f.reviews++
	f.trace = append(f.trace, "review:"+summary)
	task := f.current
	task.Status = tasks.StatusReview
	return task, nil
}
func (f *executionFixture) Agent(ctx context.Context, reason, id string) context.Context {
	f.trace = append(f.trace, "agent:"+reason+":"+id)
	return ctx
}
func (f *executionFixture) Models(ctx context.Context, chain []string) context.Context {
	f.trace = append(f.trace, "models:"+strings.Join(chain, ","))
	return ctx
}
func (f *executionFixture) Tools(ctx context.Context, tools []string) context.Context {
	f.trace = append(f.trace, "tools:"+strings.Join(tools, ","))
	return ctx
}
func (f *executionFixture) Isolation(ctx context.Context, id string) (context.Context, string, error) {
	f.trace = append(f.trace, "isolation:"+id)
	return ctx, id, f.isoErr
}
func (f *executionFixture) service(run func(context.Context, string, string) (string, error)) *appwork.Execution {
	return appwork.NewExecution(f, executionSeats{f}, f, f, run, func(corr string, task tasks.Task, phase, agent, reason, answer, errText string) {
		f.trace = append(f.trace, "publish:"+phase)
		f.publications = append(f.publications, executionPublication{phase, answer, errText, task})
	})
}
func executionInput() appwork.ExecutionInput {
	return appwork.ExecutionInput{CorrelationID: "owned-corr", Agent: appwork.ExecutionAgent{Slug: "writer", ExecutionProfile: "agent-iso"}, Task: tasks.Task{ID: "owned", Seat: " selected "}, Intent: "intent", Reason: "reason"}
}
func runningExecution() *executionFixture {
	return &executionFixture{current: tasks.Task{ID: "owned", Status: tasks.StatusRunning, Claim: &tasks.Claim{Agent: "writer", RunID: "owned-corr"}}, found: true, selected: seat.Seat{ID: "selected", ExecutionProfile: "seat-iso", ModelChain: []string{"one", "two"}, RestrictTools: true, Tools: []string{}}, seatFound: true}
}
func TestWorkboardExecutionRetainsSeatContextAndReviewOrder(t *testing.T) {
	f := runningExecution()
	service := f.service(func(ctx context.Context, corr, intent string) (string, error) {
		f.trace = append(f.trace, "run:"+corr+":"+intent)
		return " answer ", nil
	})
	out, err := service.Run(context.Background(), executionInput())
	want := []string{"agent:reason:owned", "seat:selected", "models:one,two", "tools:", "isolation:seat-iso", "run:owned-corr:intent", "review:answer", "publish:completed"}
	if err != nil || out.Task.Status != tasks.StatusReview || !reflect.DeepEqual(f.trace, want) || f.publications[0].answer != "answer" {
		t.Fatalf("out=%+v err=%v trace=%v", out, err, f.trace)
	}
}
func TestWorkboardExecutionRetainsUnknownAndUnavailableSeatFallbacks(t *testing.T) {
	for _, mode := range []string{"missing", "isolation", "default"} {
		t.Run(mode, func(t *testing.T) {
			f := runningExecution()
			in := executionInput()
			if mode == "missing" {
				f.seatFound = false
			}
			if mode == "isolation" {
				f.isoErr = errors.New("unavailable")
			}
			if mode == "default" {
				in.Task.Seat = " DEFAULT "
			}
			_, err := f.service(func(context.Context, string, string) (string, error) { return "answer", nil }).Run(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			trace := strings.Join(f.trace, "\n")
			if mode == "missing" && (!strings.Contains(trace, `seat "selected" is unknown`) || !strings.Contains(trace, "isolation:agent-iso")) {
				t.Fatal(trace)
			}
			if mode == "isolation" && !strings.Contains(trace, `seat selected isolation "seat-iso" unavailable (unavailable)`) {
				t.Fatal(trace)
			}
			if mode == "default" && (strings.Contains(trace, "seat:") || !strings.Contains(trace, "isolation:agent-iso")) {
				t.Fatal(trace)
			}
		})
	}
}
func TestWorkboardExecutionRetainsProofAndClaimOwnership(t *testing.T) {
	for _, mode := range []string{"prove", "prove-error", "other-run", "not-running", "missing"} {
		t.Run(mode, func(t *testing.T) {
			f := runningExecution()
			f.current.Criteria = []proof.Criterion{{Text: "criterion"}}
			if mode == "prove-error" {
				f.proveErr = errors.New("proof cause")
			}
			if mode == "other-run" {
				f.current.Claim.RunID = "other"
			}
			if mode == "not-running" {
				f.current.Status = tasks.StatusDone
			}
			if mode == "missing" {
				f.found = false
			}
			out, err := f.service(func(context.Context, string, string) (string, error) { return "answer", nil }).Run(context.Background(), executionInput())
			if err != nil || out.Phase != "completed" {
				t.Fatal(out, err)
			}
			wantProof := mode == "prove" || mode == "prove-error"
			if (f.proves == 1) != wantProof || (f.reviews == 1) != (mode == "prove-error") {
				t.Fatalf("%s proves=%d reviews=%d", mode, f.proves, f.reviews)
			}
		})
	}
}
func TestWorkboardExecutionRetainsFailureReclaimAndRecursion(t *testing.T) {
	for _, mode := range []string{"fail-cause", "stop", "claim-cause", "retry"} {
		t.Run(mode, func(t *testing.T) {
			f := runningExecution()
			cause := errors.New("owned run cause")
			f.decision = tasks.RetryDecision{Action: "retry_ready", Retry: mode == "retry" || mode == "claim-cause"}
			if mode == "fail-cause" {
				f.failErr = errors.New("fail cause")
			}
			if mode == "claim-cause" {
				f.claimErr = errors.New("claim cause")
			}
			out, err := f.service(func(context.Context, string, string) (string, error) {
				f.calls++
				if f.calls == 1 {
					return "", cause
				}
				return "answer", nil
			}).Run(context.Background(), executionInput())
			if mode == "retry" {
				if err != nil || f.calls != 2 || out.Phase != "completed" || f.reviews != 1 {
					t.Fatalf("retry out=%+v err=%v fixture=%+v", out, err, f)
				}
				phases := []string{}
				for _, p := range f.publications {
					phases = append(phases, p.phase)
				}
				if !reflect.DeepEqual(phases, []string{"retry_ready", "retrying", "completed"}) {
					t.Fatal(phases)
				}
			} else if err != cause || f.calls != 1 {
				t.Fatal(out, err, f.calls)
			}
			if (f.blocks == 1) != (mode == "fail-cause" || mode == "claim-cause") {
				t.Fatal("block count", f.blocks)
			}
			if mode == "claim-cause" && f.publications[len(f.publications)-1].err != "claim cause" {
				t.Fatal(f.publications)
			}
		})
	}
}
func TestWorkboardExecutionRetainsUnicodeSummaryLimits(t *testing.T) {
	f := runningExecution()
	answer := strings.Repeat("ç", 200)
	_, err := f.service(func(context.Context, string, string) (string, error) { return " " + answer + " ", nil }).Run(context.Background(), executionInput())
	if err != nil || !strings.Contains(strings.Join(f.trace, "\n"), "review:"+strings.Repeat("ç", 120)+"…") || f.publications[0].answer != strings.Repeat("ç", 150)+"…" {
		t.Fatal(f.trace, f.publications, err)
	}
}
