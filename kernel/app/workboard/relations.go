// SPDX-License-Identifier: MIT

package workboard

import (
	"context"
	tasks "github.com/agezt/agezt/kernel/workboard"
	"time"
)

type RelationHost interface {
	LinkWorkboardTask(string, string, string, string) (tasks.Task, error)
	SetWorkboardRetryPolicy(string, string, string, *tasks.RetryPolicy) (tasks.Task, error)
	AddWorkboardDependency(string, string, string) (tasks.Task, error)
	ReclaimStaleWorkboardTask(string, string, string, time.Duration) (tasks.Task, error)
	SweepStaleWorkboardClaims(string, string, time.Duration, int) ([]tasks.Task, error)
}
type Relations struct{ host RelationHost }

func NewRelations(host RelationHost) *Relations { return &Relations{host: host} }

type LinkInput struct {
	CorrelationID string `json:"correlation_id,omitempty"`
	ID            string `json:"id"`
	Type          string `json:"type"`
	Target        string `json:"target"`
}
type PolicyInput struct {
	CorrelationID string             `json:"correlation_id,omitempty"`
	ID            string             `json:"id"`
	Actor         string             `json:"actor,omitempty"`
	Policy        *tasks.RetryPolicy `json:"policy"`
}
type DependInput struct {
	CorrelationID string `json:"correlation_id,omitempty"`
	ID            string `json:"id"`
	DependsOn     string `json:"depends_on"`
}
type ReclaimInput struct {
	CorrelationID string `json:"correlation_id,omitempty"`
	ID            string `json:"id"`
	Actor         string `json:"actor,omitempty"`
	StaleAfterMS  int    `json:"stale_after_ms"`
}
type SweepInput struct {
	CorrelationID string `json:"correlation_id,omitempty"`
	Actor         string `json:"actor,omitempty"`
	StaleAfterMS  int    `json:"stale_after_ms"`
	Limit         int    `json:"limit"`
}
type SweepOutput struct {
	Tasks          []Record `json:"tasks"`
	ReclaimedCount int      `json:"reclaimed_count"`
	StaleAfterMS   int      `json:"stale_after_ms"`
}

func (s *Relations) Link(_ context.Context, in LinkInput) (TaskOutput, error) {
	return taskResult(s.host.LinkWorkboardTask(in.CorrelationID, in.ID, in.Type, in.Target))
}
func (s *Relations) Policy(_ context.Context, in PolicyInput) (TaskOutput, error) {
	return taskResult(s.host.SetWorkboardRetryPolicy(in.CorrelationID, in.ID, in.Actor, in.Policy))
}
func (s *Relations) Depend(_ context.Context, in DependInput) (TaskOutput, error) {
	return taskResult(s.host.AddWorkboardDependency(in.CorrelationID, in.ID, in.DependsOn))
}
func (s *Relations) Reclaim(_ context.Context, in ReclaimInput) (TaskOutput, error) {
	return taskResult(s.host.ReclaimStaleWorkboardTask(in.CorrelationID, in.ID, in.Actor, time.Duration(in.StaleAfterMS)*time.Millisecond))
}
func (s *Relations) Sweep(_ context.Context, in SweepInput) (SweepOutput, error) {
	found, err := s.host.SweepStaleWorkboardClaims(in.CorrelationID, in.Actor, time.Duration(in.StaleAfterMS)*time.Millisecond, in.Limit)
	if err != nil {
		return SweepOutput{}, err
	}
	out := make([]Record, 0, len(found))
	for _, task := range found {
		out = append(out, Project(task))
	}
	return SweepOutput{Tasks: out, ReclaimedCount: len(out), StaleAfterMS: in.StaleAfterMS}, nil
}
