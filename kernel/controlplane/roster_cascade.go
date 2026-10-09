// SPDX-License-Identifier: MIT

// Agent teardown impact analysis: what removing or retiring an agent would
// touch, across every subsystem that holds something on its behalf. Split from
// roster.go (refactor Phase 3.5).

package controlplane

import (
	"sort"
	"strings"

	approster "github.com/agezt/agezt/kernel/app/roster"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/skill"
)

// nativeImpactSource reads the teardown impact from the live subsystems. The
// preview's shape and the sub-agent aggregation live in approster.ImpactService.
type nativeImpactSource struct{ s *Server }

func (n nativeImpactSource) Get(ref string) (roster.Profile, bool) { return n.s.k.Roster().Get(ref) }

func (n nativeImpactSource) Subagents(slug string) []roster.Profile { return n.s.agentSubagents(slug) }

func (n nativeImpactSource) Holdings(p roster.Profile) approster.ImpactHoldings {
	s := n.s
	return approster.ImpactHoldings{
		StandingOrders:         s.k.AgentImpact(p.Slug),
		Schedules:              s.agentScheduleImpact(p),
		Memories:               s.agentMemoryImpact(p),
		AuthoredSharedMemories: s.agentAuthoredSharedMemoryImpact(p),
		Skills:                 s.agentSkillImpact(p),
		Configs:                s.agentConfigImpact(p),
		Workspaces:             s.agentWorkspaceImpact(p),
		WorkflowRefs:           s.agentWorkflowImpact(p),
		MailboxMessages:        s.agentMailboxImpact(p),
	}
}

func (s *Server) impactService() *approster.ImpactService {
	return approster.NewImpact(nativeImpactSource{s})
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
		if approster.ConfigEntryBelongsToAgent(e, p.Slug) {
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
// memoryRecordAuthoredSharedByAgent — stay in roster.go alongside the mutators
// that share them; configEntryBelongsToAgent is approster.ConfigEntryBelongsToAgent,
// shared with the agent permission picture.
