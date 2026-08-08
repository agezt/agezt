// SPDX-License-Identifier: MIT

// Agent teardown impact analysis: what removing or retiring an agent would
// touch, across every subsystem that holds something on its behalf. Split from
// roster.go (refactor Phase 3.5).

package controlplane

import (
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/skill"
)

// cascadeSubsystem is one subsystem an agent's teardown touches, and the single
// place that subsystem's identity lives.
//
// Each one used to be four hand-written things: a lister for the agent, a
// near-identical lister for its sub-agents that differed only in which lister it
// called, and two payload keys wired by hand into a forty-entry map literal.
// Adding a subsystem meant four edits with nothing checking them against each
// other — so a subsystem could be listed in the preview under one name and
// counted under another, or aggregated for sub-agents and then never surfaced.
type cascadeSubsystem struct {
	// Key and CountKey name the agent's own list and its length in the impact
	// payload. Both are spelled out rather than derived because the wire names
	// are irregular — `workflow_refs`/`workflow_ref_count`,
	// `standing_orders`/`standing_count` — and the console reads them verbatim.
	Key, CountKey string
	// SubKey and SubCountKey are the same for the aggregate across the agent's
	// sub-agents, where each label carries its child's slug.
	SubKey, SubCountKey string
	// Impact lists what this subsystem currently holds for one agent, sorted.
	Impact func(s *Server, p roster.Profile) []string
}

// cascadeSubsystems is the ordered set of subsystems the impact preview reports.
//
// Two of them are deliberately preview-only: workflow references and mailbox
// messages are REPORTED but never cleaned, because a workflow node or a message
// thread belongs to the workflow or the conversation, not to the agent it
// happens to name. The removal payload says so explicitly (`*_retained`), and
// that asymmetry is why this table describes impact only and does not also drive
// the removal — see handleAgentRemove.
var cascadeSubsystems = []cascadeSubsystem{
	{
		Key: "standing_orders", CountKey: "standing_count",
		SubKey: "subagent_standing_orders", SubCountKey: "subagent_standing_count",
		Impact: func(s *Server, p roster.Profile) []string { return s.k.AgentImpact(p.Slug) },
	},
	{
		Key: "schedules", CountKey: "schedule_count",
		SubKey: "subagent_schedules", SubCountKey: "subagent_schedule_count",
		Impact: (*Server).agentScheduleImpact,
	},
	{
		Key: "memories", CountKey: "memory_count",
		SubKey: "subagent_memories", SubCountKey: "subagent_memory_count",
		Impact: (*Server).agentMemoryImpact,
	},
	{
		Key: "authored_shared_memories", CountKey: "authored_shared_memory_count",
		SubKey: "subagent_authored_shared_memories", SubCountKey: "subagent_authored_shared_memory_count",
		Impact: (*Server).agentAuthoredSharedMemoryImpact,
	},
	{
		Key: "skills", CountKey: "skill_count",
		SubKey: "subagent_skills", SubCountKey: "subagent_skill_count",
		Impact: (*Server).agentSkillImpact,
	},
	{
		Key: "configs", CountKey: "config_count",
		SubKey: "subagent_configs", SubCountKey: "subagent_config_count",
		Impact: (*Server).agentConfigImpact,
	},
	{
		Key: "workspaces", CountKey: "workspace_count",
		SubKey: "subagent_workspaces", SubCountKey: "subagent_workspace_count",
		Impact: (*Server).agentWorkspaceImpact,
	},
	{
		Key: "workflow_refs", CountKey: "workflow_ref_count",
		SubKey: "subagent_workflow_refs", SubCountKey: "subagent_workflow_ref_count",
		Impact: (*Server).agentWorkflowImpact,
	},
	{
		Key: "mailbox_messages", CountKey: "mailbox_message_count",
		SubKey: "subagent_mailbox_messages", SubCountKey: "subagent_mailbox_message_count",
		Impact: (*Server).agentMailboxImpact,
	},
}

// subagentImpact aggregates one subsystem's impact across an agent's
// sub-agents, prefixing each label with the child it belongs to. This replaced
// eight character-identical functions whose only difference was the lister they
// called.
func (s *Server) subagentImpact(children []roster.Profile, impact func(*Server, roster.Profile) []string) []string {
	var out []string
	for _, child := range children {
		for _, label := range impact(s, child) {
			out = append(out, child.Slug+": "+label)
		}
	}
	sort.Strings(out)
	return out
}

// agentImpactResult is the teardown preview: for every subsystem, what this
// agent holds and what its sub-agents hold. Read-only.
func (s *Server) agentImpactResult(p roster.Profile) map[string]any {
	children := s.agentSubagents(p.Slug)
	subagents := agentSubagentImpact(p.Slug, children)

	out := map[string]any{
		"slug":           p.Slug,
		"subagents":      subagents,
		"subagent_count": len(subagents),
	}
	for _, sub := range cascadeSubsystems {
		own := sub.Impact(s, p)
		kids := s.subagentImpact(children, sub.Impact)
		out[sub.Key], out[sub.CountKey] = own, len(own)
		out[sub.SubKey], out[sub.SubCountKey] = kids, len(kids)
	}
	return out
}

func (s *Server) agentScheduleImpact(p roster.Profile) []string {
	var out []string
	for _, e := range s.k.Schedules().List() {
		if strings.EqualFold(strings.TrimSpace(e.Agent), p.Slug) {
			label := e.Intent
			if strings.TrimSpace(label) == "" {
				label = e.ID
			}
			out = append(out, label+" ("+e.ID+")")
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentMemoryImpact(p roster.Profile) []string {
	scope := strings.TrimSpace(p.MemoryScope)
	if scope == "" {
		scope = p.Slug
	}
	records, err := s.k.Memory().Active()
	if err != nil {
		return nil
	}
	var out []string
	for _, r := range records {
		if memoryRecordBelongsToAgent(r, scope) {
			out = append(out, memoryRecordLabel(r))
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentAuthoredSharedMemoryImpact(p roster.Profile) []string {
	records, err := s.k.Memory().Active()
	if err != nil {
		return nil
	}
	var out []string
	for _, r := range records {
		if memoryRecordAuthoredSharedByAgent(r, p.Slug) {
			out = append(out, memoryRecordLabel(r))
		}
	}
	sort.Strings(out)
	return out
}

// memoryRecordLabel names a record by its subject, falling back to its id when
// a record was stored without one.
func memoryRecordLabel(r memory.Record) string {
	subj := strings.TrimSpace(r.Subject)
	if subj == "" {
		subj = r.ID
	}
	return subj + " (" + r.ID + ")"
}

func (s *Server) agentSkillImpact(p roster.Profile) []string {
	all, err := s.k.Forge().List()
	if err != nil {
		return nil
	}
	var out []string
	for _, sk := range all {
		if strings.EqualFold(strings.TrimSpace(sk.Agent), p.Slug) && sk.Status != skill.StatusArchived {
			name := strings.TrimSpace(sk.Name)
			if name == "" {
				name = sk.ID
			}
			out = append(out, name+" ("+sk.ID+")")
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentConfigImpact(p roster.Profile) []string {
	if s.k.ConfigCenter() == nil {
		return nil
	}
	var out []string
	for _, e := range s.k.ConfigCenter().ListEntries() {
		if configEntryBelongsToAgent(e, p.Slug) {
			label := strings.TrimSpace(e.Key)
			if e.Rating != "" {
				label += " [" + string(e.Rating) + "]"
			}
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentWorkspaceImpact(p roster.Profile) []string {
	info, ok := s.agentWorkspaceInfo(p)
	if !ok {
		return nil
	}
	return []string{info}
}

func (s *Server) agentWorkflowImpact(p roster.Profile) []string {
	slug := strings.TrimSpace(p.Slug)
	if slug == "" || s.k.Workflows() == nil {
		return nil
	}
	var out []string
	for _, w := range s.k.Workflows().List() {
		for _, n := range w.Nodes {
			if !workflowNodeConfigReferencesAgent(n.Config, slug) {
				continue
			}
			label := w.Name + "/" + n.ID
			if strings.TrimSpace(n.Label) != "" {
				label += " " + strings.TrimSpace(n.Label)
			}
			label += " [" + n.Type + "]"
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) agentMailboxImpact(p roster.Profile) []string {
	st, err := s.boardReader()
	if err != nil {
		return nil
	}
	slug := strings.ToLower(strings.TrimSpace(p.Slug))
	if slug == "" {
		return nil
	}
	var out []string
	for _, msg := range st.Read("", boardReadMaxLimit) {
		if label, ok := agentMailboxImpactLabel(msg, slug); ok {
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

// agentRemovalMailboxImpact is the removal payload's retained-message list: the
// agent's own threads plus, when sub-agents are cascaded, theirs — DEDUPED,
// because a message between a parent and its child appears in both.
func (s *Server) agentRemovalMailboxImpact(slug string, subagents []roster.Profile, includeSubagents bool) []string {
	seen := map[string]bool{}
	add := func(labels []string) {
		for _, label := range labels {
			if strings.TrimSpace(label) != "" {
				seen[label] = true
			}
		}
	}
	add(s.agentMailboxImpact(roster.Profile{Slug: slug}))
	if includeSubagents {
		for _, child := range subagents {
			add(s.agentMailboxImpact(child))
		}
	}
	out := make([]string, 0, len(seen))
	for label := range seen {
		out = append(out, label)
	}
	sort.Strings(out)
	return out
}

// The predicates these listers apply — agentMailboxImpactLabel,
// workflowNodeConfigReferencesAgent, memoryRecordBelongsToAgent,
// memoryRecordAuthoredSharedByAgent, configEntryBelongsToAgent — stay in
// roster.go alongside the mutators that share them.
