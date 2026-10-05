// SPDX-License-Identifier: MIT

// Package workboard owns transport-independent task-board use cases.
package workboard

import (
	"context"
	"errors"
	"fmt"
	tasks "github.com/agezt/agezt/kernel/workboard"
	"sort"
	"strings"
)

type Reader interface {
	List(tasks.Filter) []tasks.Task
	Get(string) (tasks.Task, bool)
}
type Service struct{ reader Reader }

func New(reader Reader) *Service { return &Service{reader: reader} }

// Record preserves actual task JSON fields plus the native computed projection.
type Record struct {
	tasks.Task
	CommentCount       int   `json:"comment_count"`
	LinkCount          int   `json:"link_count"`
	AttemptCount       int   `json:"attempt_count"`
	FailedAttemptCount int   `json:"failed_attempt_count"`
	CriteriaCount      *int  `json:"criteria_count,omitempty"`
	CriteriaMet        *int  `json:"criteria_met,omitempty"`
	Gated              *bool `json:"gated,omitempty"`
	Proven             *bool `json:"proven,omitempty"`
	MaxAttempts        int   `json:"max_attempts,omitempty"`
	NextAttempt        int   `json:"next_attempt,omitempty"`
}

func Project(t tasks.Task) Record {
	out := Record{Task: t, CommentCount: len(t.Comments), LinkCount: len(t.Links), AttemptCount: len(t.Attempts), FailedAttemptCount: tasks.FailedAttemptCount(t)}
	if len(t.Criteria) > 0 {
		count, met := len(t.Criteria), 0
		for _, criterion := range t.Criteria {
			if criterion.Met {
				met++
			}
		}
		gated, proven := true, t.Proof != nil && t.Proof.Satisfied()
		out.CriteriaCount = &count
		out.CriteriaMet = &met
		out.Gated = &gated
		out.Proven = &proven
	}
	if decision := tasks.RetryDecisionFor(t, ""); decision.MaxAttempts > 0 {
		out.MaxAttempts = decision.MaxAttempts
		if decision.NextAttempt > 0 {
			out.NextAttempt = decision.NextAttempt
		}
	}
	return out
}

type ListInput struct {
	Status          tasks.Status `json:"status,omitempty"`
	Tenant          string       `json:"tenant,omitempty"`
	Assignee        string       `json:"assignee,omitempty"`
	IncludeArchived bool         `json:"include_archived,omitempty"`
	Limit           int          `json:"limit,omitempty"`
}
type ListOutput struct {
	Tasks []Record `json:"tasks"`
	Count int      `json:"count"`
}

func (s *Service) List(_ context.Context, in ListInput) (ListOutput, error) {
	found := s.reader.List(tasks.Filter{Status: in.Status, Tenant: in.Tenant, Assignee: in.Assignee, IncludeArchived: in.IncludeArchived, Limit: in.Limit})
	out := make([]Record, 0, len(found))
	for _, task := range found {
		out = append(out, Project(task))
	}
	return ListOutput{Tasks: out, Count: len(out)}, nil
}

type Lane struct {
	Assignee string         `json:"assignee"`
	Label    string         `json:"label"`
	Counts   map[string]int `json:"counts"`
	Tasks    []Record       `json:"tasks"`
	Count    int            `json:"count"`
}
type LanesOutput struct {
	Lanes     []Lane `json:"lanes"`
	Count     int    `json:"count"`
	TaskCount int    `json:"task_count"`
}

func (s *Service) Lanes(_ context.Context, in ListInput) (LanesOutput, error) {
	found := s.reader.List(tasks.Filter{Status: in.Status, Tenant: in.Tenant, IncludeArchived: in.IncludeArchived, Limit: in.Limit})
	byAssignee := map[string]*Lane{}
	for _, task := range found {
		key := strings.TrimSpace(task.Assignee)
		lane := byAssignee[key]
		if lane == nil {
			label := key
			if label == "" {
				label = "unassigned"
			}
			lane = &Lane{Assignee: key, Label: label, Counts: map[string]int{}}
			byAssignee[key] = lane
		}
		lane.Counts[string(task.Status)]++
		lane.Tasks = append(lane.Tasks, Project(task))
		lane.Count = len(lane.Tasks)
	}
	keys := make([]string, 0, len(byAssignee))
	for key := range byAssignee {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if keys[i] == "" {
			return false
		}
		if keys[j] == "" {
			return true
		}
		return strings.ToLower(keys[i]) < strings.ToLower(keys[j])
	})
	out := make([]Lane, 0, len(keys))
	for _, key := range keys {
		out = append(out, *byAssignee[key])
	}
	return LanesOutput{Lanes: out, Count: len(out), TaskCount: len(found)}, nil
}

type ShowInput struct {
	ID string `json:"id"`
}
type ShowOutput struct {
	Task Record `json:"task"`
}

func (s *Service) Show(_ context.Context, in ShowInput) (ShowOutput, error) {
	if in.ID == "" {
		return ShowOutput{}, errors.New("workboard_show requires id")
	}
	task, found := s.reader.Get(in.ID)
	if !found {
		return ShowOutput{}, fmt.Errorf("unknown workboard task: %s", in.ID)
	}
	return ShowOutput{Task: Project(task)}, nil
}
