// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/roster"
)

// RemoveRequest keeps every argument raw: ref is a strict string and the
// cascade is an optional object of lenient flags.
type RemoveRequest struct {
	Ref     json.RawMessage `json:"ref,omitempty"`
	Cascade json.RawMessage `json:"cascade,omitempty"`
}

// RemoveCascade selects what a removal cleans up besides the profile.
type RemoveCascade struct {
	Standing       bool
	Schedules      bool
	Memory         bool
	AuthoredMemory bool
	Skills         bool
	Config         bool
	Workspace      bool
	Subagents      bool
}

func boolish(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		x = strings.TrimSpace(strings.ToLower(x))
		return x == "1" || x == "true" || x == "yes" || x == "on"
	default:
		return false
	}
}

// ParseRemoveCascade reads the cascade flags; anything but an object selects
// nothing. authored_shared_memory and workdir are accepted aliases.
func ParseRemoveCascade(raw json.RawMessage) RemoveCascade {
	var c RemoveCascade
	v, _ := rawValue(raw)
	m, ok := v.(map[string]any)
	if !ok {
		return c
	}
	c.Standing = boolish(m["standing"])
	c.Schedules = boolish(m["schedules"])
	c.Memory = boolish(m["memory"])
	c.AuthoredMemory = boolish(m["authored_memory"]) || boolish(m["authored_shared_memory"])
	c.Skills = boolish(m["skills"])
	c.Config = boolish(m["config"])
	c.Workspace = boolish(m["workspace"]) || boolish(m["workdir"])
	c.Subagents = boolish(m["subagents"])
	return c
}

func (c RemoveCascade) view() map[string]any {
	return map[string]any{
		"standing":        c.Standing,
		"schedules":       c.Schedules,
		"memory":          c.Memory,
		"authored_memory": c.AuthoredMemory,
		"skills":          c.Skills,
		"config":          c.Config,
		"workspace":       c.Workspace,
		"subagents":       c.Subagents,
	}
}

// RemoveReport is what a removal did and what it deliberately kept: mailbox
// messages and workflow references belong to the conversation or the workflow,
// so they are reported as retained, never cleaned.
type RemoveReport struct {
	StandingRemoved                    int      `json:"standing_removed"`
	SchedulesRemoved                   int      `json:"schedules_removed"`
	MemoriesForgotten                  int      `json:"memories_forgotten"`
	AuthoredMemoriesForgotten          int      `json:"authored_memories_forgotten"`
	SkillsArchived                     int      `json:"skills_archived"`
	ConfigsDeleted                     int      `json:"configs_deleted"`
	ConfigsAccessPruned                int      `json:"configs_access_pruned"`
	WorkspacesDeleted                  int      `json:"workspaces_deleted"`
	SubagentsRetired                   int      `json:"subagents_retired"`
	SubagentsRetiredSlugs              []string `json:"subagents_retired_slugs"`
	MailboxMessagesRetained            int      `json:"mailbox_messages_retained"`
	MailboxMessagesRetainedRefs        []string `json:"mailbox_messages_retained_refs"`
	WorkflowRefsRetained               int      `json:"workflow_refs_retained"`
	WorkflowRefsRetainedLabels         []string `json:"workflow_refs_retained_labels"`
	SubagentWorkflowRefsRetained       int      `json:"subagent_workflow_refs_retained"`
	SubagentWorkflowRefsRetainedLabels []string `json:"subagent_workflow_refs_retained_labels"`
}

// RemoveOutput reports only `removed:false` for an unknown agent, and the full
// report once a removal ran.
type RemoveOutput struct {
	Removed bool `json:"removed"`
	*RemoveReport
}

// RemovePorts are the native effects of an agent removal. Each cascade port
// does nothing and reports zero when its flag is off.
type RemovePorts struct {
	Get        func(string) (core.Profile, bool)
	Subagents  func(slug string) []core.Profile
	Retained   func(slug string, subagents []core.Profile, includeSubagents bool) []string
	Workflows  func(core.Profile) []string
	Retire     func(parent string, children []core.Profile, on bool) (int, []string, error)
	Standing   func(slug string, on bool) (int, error)
	Schedules  func(slug string, on bool) (int, error)
	Memory     func(p core.Profile, on bool) (int, error)
	Authored   func(slug string, on bool) (int, error)
	Skills     func(slug string, on bool) (int, error)
	Config     func(slug string, on bool) (int, int, error)
	Workspace  func(p core.Profile, on bool) (int, error)
	Remove     func(ref string) (bool, error)
	NewCorr    func() string
	Publish    func(subject, corr string, payload map[string]any)
	Invalidate func()
}

// RemoveService deletes an agent profile with the cleanup its cascade selects.
type RemoveService struct{ ports RemovePorts }

func NewRemove(ports RemovePorts) *RemoveService { return &RemoveService{ports: ports} }

func (s *RemoveService) Remove(_ context.Context, in RemoveRequest) (RemoveOutput, error) {
	ref, err := RefPageRequest{Ref: in.Ref}.ref()
	if err != nil {
		return RemoveOutput{}, err
	}
	p, found := s.ports.Get(ref)
	if !found {
		return RemoveOutput{Removed: false}, nil
	}
	if p.System {
		return RemoveOutput{}, errors.New("system agent " + p.Slug + " cannot be removed; retire or pause it instead")
	}
	cascade := ParseRemoveCascade(in.Cascade)
	subagents := s.ports.Subagents(p.Slug)
	if len(subagents) > 0 && !cascade.Subagents {
		return RemoveOutput{}, fmt.Errorf("agent %s has %d dependent sub-agent(s); set cascade.subagents=true to retire them before removal", p.Slug, len(subagents))
	}
	r := &RemoveReport{}
	r.MailboxMessagesRetainedRefs = s.ports.Retained(p.Slug, subagents, cascade.Subagents)
	r.WorkflowRefsRetainedLabels = s.ports.Workflows(p)
	if cascade.Subagents {
		lists := make([][]string, len(subagents))
		for i, child := range subagents {
			lists[i] = s.ports.Workflows(child)
		}
		r.SubagentWorkflowRefsRetainedLabels = AggregateSubagentLabels(subagents, lists)
	}
	r.MailboxMessagesRetained = len(r.MailboxMessagesRetainedRefs)
	r.WorkflowRefsRetained = len(r.WorkflowRefsRetainedLabels)
	r.SubagentWorkflowRefsRetained = len(r.SubagentWorkflowRefsRetainedLabels)

	if r.SubagentsRetired, r.SubagentsRetiredSlugs, err = s.ports.Retire(p.Slug, subagents, cascade.Subagents); err != nil {
		return RemoveOutput{}, err
	}
	if r.StandingRemoved, err = s.ports.Standing(p.Slug, cascade.Standing); err != nil {
		return RemoveOutput{}, err
	}
	if r.SchedulesRemoved, err = s.ports.Schedules(p.Slug, cascade.Schedules); err != nil {
		return RemoveOutput{}, err
	}
	if cascade.Subagents {
		for _, child := range subagents {
			n, err := s.ports.Standing(child.Slug, cascade.Standing)
			if err != nil {
				return RemoveOutput{}, err
			}
			r.StandingRemoved += n
			if n, err = s.ports.Schedules(child.Slug, cascade.Schedules); err != nil {
				return RemoveOutput{}, err
			}
			r.SchedulesRemoved += n
		}
	}
	if r.MemoriesForgotten, err = s.ports.Memory(p, cascade.Memory); err != nil {
		return RemoveOutput{}, err
	}
	if r.AuthoredMemoriesForgotten, err = s.ports.Authored(p.Slug, cascade.AuthoredMemory); err != nil {
		return RemoveOutput{}, err
	}
	if r.SkillsArchived, err = s.ports.Skills(p.Slug, cascade.Skills); err != nil {
		return RemoveOutput{}, err
	}
	if r.ConfigsDeleted, r.ConfigsAccessPruned, err = s.ports.Config(p.Slug, cascade.Config); err != nil {
		return RemoveOutput{}, err
	}
	if r.WorkspacesDeleted, err = s.ports.Workspace(p, cascade.Workspace); err != nil {
		return RemoveOutput{}, err
	}
	if cascade.Subagents {
		for _, child := range subagents {
			n, err := s.ports.Memory(child, cascade.Memory)
			if err != nil {
				return RemoveOutput{}, err
			}
			r.MemoriesForgotten += n
			if n, err = s.ports.Authored(child.Slug, cascade.AuthoredMemory); err != nil {
				return RemoveOutput{}, err
			}
			r.AuthoredMemoriesForgotten += n
			if n, err = s.ports.Skills(child.Slug, cascade.Skills); err != nil {
				return RemoveOutput{}, err
			}
			r.SkillsArchived += n
			var pruned int
			if n, pruned, err = s.ports.Config(child.Slug, cascade.Config); err != nil {
				return RemoveOutput{}, err
			}
			r.ConfigsDeleted += n
			r.ConfigsAccessPruned += pruned
			if n, err = s.ports.Workspace(child, cascade.Workspace); err != nil {
				return RemoveOutput{}, err
			}
			r.WorkspacesDeleted += n
		}
	}
	ok, err := s.ports.Remove(ref)
	if err != nil {
		return RemoveOutput{}, err
	}
	if ok {
		payload := objectPayload(r)
		payload["agent"] = p.Slug
		payload["removed"] = true
		payload["cascade"] = cascade.view()
		s.ports.Publish("agent.remove", s.ports.NewCorr(), payload)
	}
	s.ports.Invalidate()
	return RemoveOutput{Removed: ok, RemoveReport: r}, nil
}

func RemoveOperations(provider func(context.Context) *RemoveService) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("roster remove provider required")
	}
	output, err := schema.FromType(reflect.TypeFor[RemoveOutput](), false)
	if err != nil {
		return nil, err
	}
	op, err := app.NewOperation(opapi.Spec{Name: "agent_remove", OutputSchema: output, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{},"cascade":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/agents/remove"}}, func(ctx context.Context, in RemoveRequest) (RemoveOutput, error) {
		return provider(ctx).Remove(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
