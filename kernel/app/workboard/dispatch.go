// SPDX-License-Identifier: MIT

package workboard

import (
	"context"
	"errors"
	"fmt"
	tasks "github.com/agezt/agezt/kernel/workboard"
	"strings"
)

type DispatchHost interface {
	NewCorrelation() string
	ClaimWorkboardTask(string, string, string, string) (tasks.Task, error)
	LinkWorkboardTask(string, string, string, string) (tasks.Task, error)
}

// Agent admission and execution are supplied by the selected host; the app layer
// neither imports roster/runtime nor changes the existing background runner.
type DispatchAgent struct {
	Slug          string
	Retired       bool
	Enabled       bool
	DirectAllowed bool
	DirectError   string
	Run           func(string, tasks.Task, string, string)
}
type Dispatch struct {
	store   WatchStore
	host    DispatchHost
	agent   func(string) (DispatchAgent, bool)
	publish func(string, tasks.Task, string, string, string, string, string)
}

func NewDispatch(store WatchStore, host DispatchHost, agent func(string) (DispatchAgent, bool), publish func(string, tasks.Task, string, string, string, string, string)) *Dispatch {
	return &Dispatch{store: store, host: host, agent: agent, publish: publish}
}

type DispatchInput struct {
	ID     string `json:"id"`
	Agent  string `json:"agent,omitempty"`
	Reason string `json:"reason,omitempty"`
	Intent string `json:"intent,omitempty"`
}
type DispatchOutput struct {
	Accepted      bool   `json:"accepted"`
	Task          Record `json:"task"`
	Agent         string `json:"agent"`
	CorrelationID string `json:"correlation_id"`
}

func (s *Dispatch) Dispatch(_ context.Context, in DispatchInput) (DispatchOutput, error) {
	if in.ID == "" {
		return DispatchOutput{}, errors.New("workboard_dispatch requires id")
	}
	task, found := s.store.Get(in.ID)
	if !found {
		return DispatchOutput{}, fmt.Errorf("unknown workboard task: %s", in.ID)
	}
	blocked, err := s.store.BlockingDependencies(task.ID)
	if err != nil {
		return DispatchOutput{}, err
	}
	if len(blocked) > 0 {
		return DispatchOutput{}, errors.New("workboard task blocked by dependencies: " + dependencySummary(blocked))
	}
	agentRef := strings.TrimSpace(in.Agent)
	if agentRef == "" {
		agentRef = strings.TrimSpace(task.Assignee)
	}
	if agentRef == "" {
		return DispatchOutput{}, errors.New("workboard_dispatch requires --agent or a task assignee")
	}
	agent, ok := s.agent(agentRef)
	if !ok {
		return DispatchOutput{}, fmt.Errorf("unknown agent: %s", agentRef)
	}
	if agent.Retired {
		return DispatchOutput{}, fmt.Errorf("agent %s is retired — revive it first", agent.Slug)
	}
	if !agent.Enabled {
		return DispatchOutput{}, fmt.Errorf("agent %s is paused", agent.Slug)
	}
	if !agent.DirectAllowed {
		return DispatchOutput{}, errors.New(agent.DirectError)
	}
	corr := s.host.NewCorrelation()
	_, err = s.host.ClaimWorkboardTask(corr, task.ID, agent.Slug, corr)
	if err != nil {
		return DispatchOutput{}, err
	}
	claimed, err := s.host.LinkWorkboardTask(corr, task.ID, "run", corr)
	if err != nil {
		return DispatchOutput{}, err
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		reason = "workboard dispatch"
	}
	intent := DispatchIntent(in.Intent, claimed)
	s.publish(corr, claimed, "requested", agent.Slug, reason, "", "")
	go agent.Run(corr, claimed, intent, reason)
	return DispatchOutput{Accepted: true, Task: Project(claimed), Agent: agent.Slug, CorrelationID: corr}, nil
}
func DispatchIntent(explicit string, task tasks.Task) string {
	if explicit = strings.TrimSpace(explicit); explicit != "" {
		return explicit
	}
	var b strings.Builder
	b.WriteString("Workboard task dispatch.\n")
	b.WriteString("You are assigned a durable AGEZT workboard task. Use the workboard tool to heartbeat, comment, block, link artifacts/runs, and complete the task when it is actually done.\n")
	b.WriteString("Task ID: ")
	b.WriteString(task.ID)
	b.WriteString("\nTitle: ")
	b.WriteString(task.Title)
	b.WriteString("\nStatus: ")
	b.WriteString(string(task.Status))
	if task.Priority != 0 {
		b.WriteString("\nPriority: ")
		b.WriteString(fmt.Sprintf("%d", task.Priority))
	}
	if task.Tenant != "" {
		b.WriteString("\nTenant: ")
		b.WriteString(task.Tenant)
	}
	if task.Description != "" {
		b.WriteString("\nDescription:\n")
		b.WriteString(task.Description)
	}
	if len(task.Tags) > 0 {
		b.WriteString("\nTags: ")
		b.WriteString(strings.Join(task.Tags, ", "))
	}
	b.WriteString("\nExpected finish: call workboard {\"op\":\"complete\",\"id\":\"")
	b.WriteString(task.ID)
	b.WriteString("\"} only when complete; otherwise call workboard block/comment with the concrete reason or next step.")
	return b.String()
}
func dependencySummary(states []tasks.DependencyState) string {
	parts := make([]string, 0, len(states))
	for _, st := range states {
		status := string(st.Status)
		if st.Missing {
			status = "missing"
		}
		if st.Title != "" {
			parts = append(parts, fmt.Sprintf("%s(%s:%s)", st.ID, st.Title, status))
		} else {
			parts = append(parts, fmt.Sprintf("%s(%s)", st.ID, status))
		}
	}
	return strings.Join(parts, ", ")
}
