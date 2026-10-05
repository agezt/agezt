// SPDX-License-Identifier: MIT

package workboard

import (
	"context"
	tasks "github.com/agezt/agezt/kernel/workboard"
	"time"
)

type LifecycleHost interface {
	CreateWorkboardTask(string, tasks.CreateSpec) (tasks.Task, bool, error)
	ClaimWorkboardTask(string, string, string, string) (tasks.Task, error)
	HeartbeatWorkboardTask(string, string, string, string) (tasks.Task, error)
	CommentWorkboardTask(string, string, string, string) (tasks.Task, error)
	BlockWorkboardTask(string, string, string, string) (tasks.Task, error)
	FailWorkboardTask(string, string, string, string) (tasks.Task, tasks.RetryDecision, error)
	UnblockWorkboardTask(string, string, string) (tasks.Task, error)
	CompleteWorkboardTask(string, string, string) (tasks.Task, error)
	ProveTask(context.Context, string, string, string) (tasks.Task, error)
	ArchiveWorkboardTask(string, string, string) (tasks.Task, error)
}
type SeatSetter interface {
	SetSeat(string, string, time.Time) (tasks.Task, error)
}
type Lifecycle struct {
	host  LifecycleHost
	seats SeatSetter
}

func NewLifecycle(host LifecycleHost, seats SeatSetter) *Lifecycle {
	return &Lifecycle{host: host, seats: seats}
}

type CreateInput struct {
	CorrelationID string           `json:"correlation_id,omitempty"`
	Spec          tasks.CreateSpec `json:"spec"`
}
type ClaimInput struct {
	CorrelationID string `json:"correlation_id,omitempty"`
	ID            string `json:"id"`
	Agent         string `json:"agent"`
	RunID         string `json:"run_id,omitempty"`
}
type CommentInput struct {
	CorrelationID string `json:"correlation_id,omitempty"`
	ID            string `json:"id"`
	Author        string `json:"author,omitempty"`
	Body          string `json:"body"`
}
type ReasonInput struct {
	CorrelationID string `json:"correlation_id,omitempty"`
	ID            string `json:"id"`
	Actor         string `json:"actor,omitempty"`
	Reason        string `json:"reason,omitempty"`
}
type ProveInput struct {
	CorrelationID string `json:"correlation_id,omitempty"`
	ID            string `json:"id"`
	Answer        string `json:"answer,omitempty"`
}
type SeatInput struct {
	ID   string `json:"id"`
	Seat string `json:"seat,omitempty"`
}
type TaskOutput struct {
	Task Record `json:"task"`
}
type CreateOutput struct {
	Task    Record `json:"task"`
	Created bool   `json:"created"`
}
type Decision struct {
	Action       string `json:"action"`
	FailureCount int    `json:"failure_count"`
	Retry        bool   `json:"retry"`
	Exhausted    bool   `json:"exhausted"`
	MaxAttempts  int    `json:"max_attempts,omitempty"`
	NextAttempt  int    `json:"next_attempt,omitempty"`
	EscalateTo   string `json:"escalate_to,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

func ProjectDecision(d tasks.RetryDecision) Decision {
	return Decision{Action: d.Action, FailureCount: d.FailureCount, Retry: d.Retry, Exhausted: d.Exhausted, MaxAttempts: d.MaxAttempts, NextAttempt: d.NextAttempt, EscalateTo: d.EscalateTo, Reason: d.Reason}
}

type FailOutput struct {
	Task     Record   `json:"task"`
	Decision Decision `json:"decision"`
}

func taskResult(task tasks.Task, err error) (TaskOutput, error) {
	if err != nil {
		return TaskOutput{}, err
	}
	return TaskOutput{Task: Project(task)}, nil
}
func (s *Lifecycle) Create(_ context.Context, in CreateInput) (CreateOutput, error) {
	task, created, err := s.host.CreateWorkboardTask(in.CorrelationID, in.Spec)
	if err != nil {
		return CreateOutput{}, err
	}
	return CreateOutput{Task: Project(task), Created: created}, nil
}
func (s *Lifecycle) Claim(_ context.Context, in ClaimInput) (TaskOutput, error) {
	return taskResult(s.host.ClaimWorkboardTask(in.CorrelationID, in.ID, in.Agent, in.RunID))
}
func (s *Lifecycle) Heartbeat(_ context.Context, in ClaimInput) (TaskOutput, error) {
	return taskResult(s.host.HeartbeatWorkboardTask(in.CorrelationID, in.ID, in.Agent, in.RunID))
}
func (s *Lifecycle) Comment(_ context.Context, in CommentInput) (TaskOutput, error) {
	return taskResult(s.host.CommentWorkboardTask(in.CorrelationID, in.ID, in.Author, in.Body))
}
func (s *Lifecycle) Block(_ context.Context, in ReasonInput) (TaskOutput, error) {
	return taskResult(s.host.BlockWorkboardTask(in.CorrelationID, in.ID, in.Actor, in.Reason))
}
func (s *Lifecycle) Fail(_ context.Context, in ReasonInput) (FailOutput, error) {
	task, decision, err := s.host.FailWorkboardTask(in.CorrelationID, in.ID, in.Actor, in.Reason)
	if err != nil {
		return FailOutput{}, err
	}
	return FailOutput{Task: Project(task), Decision: ProjectDecision(decision)}, nil
}
func (s *Lifecycle) Unblock(_ context.Context, in ReasonInput) (TaskOutput, error) {
	return taskResult(s.host.UnblockWorkboardTask(in.CorrelationID, in.ID, in.Actor))
}
func (s *Lifecycle) Complete(_ context.Context, in ReasonInput) (TaskOutput, error) {
	return taskResult(s.host.CompleteWorkboardTask(in.CorrelationID, in.ID, in.Actor))
}
func (s *Lifecycle) Prove(ctx context.Context, in ProveInput) (TaskOutput, error) {
	return taskResult(s.host.ProveTask(ctx, in.CorrelationID, in.ID, in.Answer))
}
func (s *Lifecycle) Seat(_ context.Context, in SeatInput) (TaskOutput, error) {
	return taskResult(s.seats.SetSeat(in.ID, in.Seat, time.Now()))
}
func (s *Lifecycle) Archive(_ context.Context, in ReasonInput) (TaskOutput, error) {
	return taskResult(s.host.ArchiveWorkboardTask(in.CorrelationID, in.ID, in.Actor))
}
