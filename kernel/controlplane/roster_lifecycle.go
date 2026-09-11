// SPDX-License-Identifier: MIT

package controlplane

// Agent lifecycle handlers (M846): retire/revive/set-retired/remove plus
// the cascade view + reference-detection helpers. Carved out of roster.go
// during the Day 24 god file split #8 so the main file can focus on
// escalation/workspace.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/roster"
)

func (s *Server) handleAgentRetire(conn net.Conn, req Request) {
	s.handleAgentSetRetired(conn, req, true)
}

func (s *Server) handleAgentRevive(conn net.Conn, req Request) {
	s.handleAgentSetRetired(conn, req, false)
}

func (s *Server) handleAgentSetRetired(conn net.Conn, req Request, retired bool) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	reason := stringArg(req.Args, "reason")
	// Compute impact BEFORE the state change so a retire reports what it affected.
	var impact []string
	var impactSummary map[string]any
	if retired {
		if p, ok := s.k.Roster().Get(ref); ok {
			impact = s.k.AgentImpact(p.Slug)
			impactSummary = s.agentImpactResult(p)
		}
	} else if p, ok := s.k.Roster().Get(ref); ok {
		if err := s.validateAgentHierarchyRefs(p); err != nil {
			s.fail(conn, req, err)
			return
		}
	}
	p, err := s.k.SetProfileRetired(ref, retired, reason)
	if err != nil {
		if errors.Is(err, roster.ErrNotFound) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
			return
		}
		s.fail(conn, req, err)
		return
	}
	res := map[string]any{"profile": profileView(p)}
	if retired {
		pausedStanding, err := s.pauseAgentStanding(p.Slug)
		if err != nil {
			s.fail(conn, req, err)
			return
		}
		pausedSchedules, err := s.pauseAgentSchedules(p.Slug)
		if err != nil {
			s.fail(conn, req, err)
			return
		}
		res["impact"] = impact
		res["impact_summary"] = impactSummary
		res["standing_paused"] = pausedStanding
		res["schedules_paused"] = pausedSchedules
		if impactSummary != nil {
			impactSummary["standing_paused"] = pausedStanding
			impactSummary["schedules_paused"] = pausedSchedules
		}
		publishOperatorAction(s.k, "agent.retire", s.k.NewCorrelation(), map[string]any{
			"agent":            p.Slug,
			"reason":           p.RetiredReason,
			"retired_ms":       p.RetiredMS,
			"standing_paused":  pausedStanding,
			"schedules_paused": pausedSchedules,
			"impact_summary":   impactSummary,
		})
	} else {
		pausedStanding := s.countAgentPausedStanding(p.Slug)
		pausedSchedules := s.countAgentPausedSchedules(p.Slug)
		res["standing_paused"] = pausedStanding
		res["schedules_paused"] = pausedSchedules
		publishOperatorAction(s.k, "agent.revive", s.k.NewCorrelation(), map[string]any{
			"agent":            p.Slug,
			"standing_paused":  pausedStanding,
			"schedules_paused": pausedSchedules,
		})
	}
	s.invalidateAgentListCache()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: res})
}

func (s *Server) handleAgentRemove(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, found := s.k.Roster().Get(ref)
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"removed": false}})
		return
	}
	if p.System {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "system agent " + p.Slug + " cannot be removed; retire or pause it instead"})
		return
	}
	cascade := parseAgentRemoveCascade(req.Args["cascade"])
	subagents := s.agentSubagents(p.Slug)
	if len(subagents) > 0 && !cascade.Subagents {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: fmt.Sprintf("agent %s has %d dependent sub-agent(s); set cascade.subagents=true to retire them before removal", p.Slug, len(subagents))})
		return
	}
	retainedMailboxMessageLabels := s.agentRemovalMailboxImpact(p.Slug, subagents, cascade.Subagents)
	retainedWorkflowRefLabels := s.agentWorkflowImpact(p)
	retainedSubagentWorkflowRefLabels := []string(nil)
	if cascade.Subagents {
		retainedSubagentWorkflowRefLabels = s.subagentImpact(subagents, (*Server).agentWorkflowImpact)
	}
	retainedMailboxMessages := len(retainedMailboxMessageLabels)
	retainedWorkflowRefs := len(retainedWorkflowRefLabels)
	retainedSubagentWorkflowRefs := len(retainedSubagentWorkflowRefLabels)
	retiredSubagents, retiredSubagentSlugs, err := s.retireAgentSubagents(p.Slug, subagents, cascade.Subagents)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	removedStanding, err := s.removeAgentStanding(p.Slug, cascade.Standing)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	removedSchedules, err := s.removeAgentSchedules(p.Slug, cascade.Schedules)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if cascade.Subagents {
		for _, child := range subagents {
			n, err := s.removeAgentStanding(child.Slug, cascade.Standing)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			removedStanding += n
			n, err = s.removeAgentSchedules(child.Slug, cascade.Schedules)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			removedSchedules += n
		}
	}
	forgotMemory, err := s.forgetAgentMemory(p, cascade.Memory)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	forgotAuthoredMemory, err := s.forgetAgentAuthoredSharedMemory(p.Slug, cascade.AuthoredMemory)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	archivedSkills, err := s.archiveAgentSkills(p.Slug, cascade.Skills)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	deletedConfig, prunedConfigAccess, err := s.deleteAgentConfigEntries(p.Slug, cascade.Config)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	deletedWorkspaces, err := s.deleteAgentWorkspace(p, cascade.Workspace)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if cascade.Subagents {
		for _, child := range subagents {
			n, err := s.forgetAgentMemory(child, cascade.Memory)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			forgotMemory += n
			n, err = s.forgetAgentAuthoredSharedMemory(child.Slug, cascade.AuthoredMemory)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			forgotAuthoredMemory += n
			n, err = s.archiveAgentSkills(child.Slug, cascade.Skills)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			archivedSkills += n
			var pruned int
			n, pruned, err = s.deleteAgentConfigEntries(child.Slug, cascade.Config)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			deletedConfig += n
			prunedConfigAccess += pruned
			n, err = s.deleteAgentWorkspace(child, cascade.Workspace)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			deletedWorkspaces += n
		}
	}
	ok, err := s.k.RemoveProfile(ref)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if ok {
		publishOperatorAction(s.k, "agent.remove", s.k.NewCorrelation(), map[string]any{
			"agent":                                  p.Slug,
			"removed":                                true,
			"cascade":                                agentRemoveCascadeView(cascade),
			"standing_removed":                       removedStanding,
			"schedules_removed":                      removedSchedules,
			"memories_forgotten":                     forgotMemory,
			"authored_memories_forgotten":            forgotAuthoredMemory,
			"skills_archived":                        archivedSkills,
			"configs_deleted":                        deletedConfig,
			"configs_access_pruned":                  prunedConfigAccess,
			"workspaces_deleted":                     deletedWorkspaces,
			"subagents_retired":                      retiredSubagents,
			"subagents_retired_slugs":                retiredSubagentSlugs,
			"mailbox_messages_retained":              retainedMailboxMessages,
			"mailbox_messages_retained_refs":         retainedMailboxMessageLabels,
			"workflow_refs_retained":                 retainedWorkflowRefs,
			"workflow_refs_retained_labels":          retainedWorkflowRefLabels,
			"subagent_workflow_refs_retained":        retainedSubagentWorkflowRefs,
			"subagent_workflow_refs_retained_labels": retainedSubagentWorkflowRefLabels,
		})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"removed":                                ok,
		"standing_removed":                       removedStanding,
		"schedules_removed":                      removedSchedules,
		"memories_forgotten":                     forgotMemory,
		"authored_memories_forgotten":            forgotAuthoredMemory,
		"skills_archived":                        archivedSkills,
		"configs_deleted":                        deletedConfig,
		"configs_access_pruned":                  prunedConfigAccess,
		"workspaces_deleted":                     deletedWorkspaces,
		"subagents_retired":                      retiredSubagents,
		"subagents_retired_slugs":                retiredSubagentSlugs,
		"mailbox_messages_retained":              retainedMailboxMessages,
		"mailbox_messages_retained_refs":         retainedMailboxMessageLabels,
		"workflow_refs_retained":                 retainedWorkflowRefs,
		"workflow_refs_retained_labels":          retainedWorkflowRefLabels,
		"subagent_workflow_refs_retained":        retainedSubagentWorkflowRefs,
		"subagent_workflow_refs_retained_labels": retainedSubagentWorkflowRefLabels,
	}})
	s.invalidateAgentListCache()
}

type agentRemoveCascade struct {
	Standing       bool
	Schedules      bool
	Memory         bool
	AuthoredMemory bool
	Skills         bool
	Config         bool
	Workspace      bool
	Subagents      bool
}

func agentRemoveCascadeView(c agentRemoveCascade) map[string]any {
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

func parseAgentRemoveCascade(raw any) agentRemoveCascade {
	var c agentRemoveCascade
	m, ok := raw.(map[string]any)
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

func agentMailboxImpactLabel(msg board.Message, slug string) (string, bool) {
	from := strings.ToLower(strings.TrimSpace(msg.From))
	to := strings.ToLower(strings.TrimSpace(msg.To))
	acked := boardMessageAckedBy(msg, slug)
	var direction string
	switch {
	case from == slug:
		direction = "sent"
	case to == slug:
		direction = "received"
	case msg.To == board.Everyone && from != slug:
		direction = "broadcast"
	case acked:
		direction = "acked"
	default:
		return "", false
	}
	topic := strings.TrimSpace(msg.Topic)
	if topic == "" {
		topic = "board"
	}
	id := strings.TrimSpace(msg.ID)
	if id == "" {
		id = strconv.FormatInt(msg.TSMS, 10)
	}
	return topic + " " + direction + " (" + id + ")", true
}

func agentSubagentImpact(slug string, children []roster.Profile) []string {
	out := make([]string, 0, len(children))
	for _, child := range children {
		roles := make([]string, 0, 2)
		if strings.EqualFold(strings.TrimSpace(child.OwnerAgent), slug) {
			roles = append(roles, "owner")
		}
		if strings.EqualFold(strings.TrimSpace(child.ParentAgent), slug) {
			roles = append(roles, "parent")
		}
		if len(roles) == 0 {
			roles = append(roles, "descendant")
		}
		label := child.Slug
		if strings.TrimSpace(child.Name) != "" && strings.TrimSpace(child.Name) != child.Slug {
			label = strings.TrimSpace(child.Name) + " (" + child.Slug + ")"
		}
		if len(roles) > 0 {
			label += " [" + strings.Join(roles, ", ") + "]"
		}
		if child.Retired {
			label += " [retired]"
		}
		out = append(out, label)
	}
	sort.Strings(out)
	return out
}

func workflowNodeConfigReferencesAgent(raw json.RawMessage, slug string) bool {
	if len(raw) == 0 {
		return false
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	return jsonValueReferencesAgent(v, strings.ToLower(strings.TrimSpace(slug)), "")
}

func jsonValueReferencesAgent(v any, slug, key string) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			if jsonValueReferencesAgent(value, slug, strings.ToLower(strings.TrimSpace(k))) {
				return true
			}
		}
	case []any:
		for _, value := range x {
			if jsonValueReferencesAgent(value, slug, key) {
				return true
			}
		}
	case string:
		if !agentReferenceConfigKey(key) {
			return false
		}
		return strings.EqualFold(strings.TrimSpace(x), slug)
	}
	return false
}

func agentReferenceConfigKey(key string) bool {
	switch key {
	case "agent", "agent_slug", "target_agent", "owner_agent", "parent_agent", "delegate_to", "source_agent", "root_agent":
		return true
	default:
		return false
	}
}

func (s *Server) agentSubagents(slug string) []roster.Profile {
	root := strings.TrimSpace(slug)
	if root == "" {
		return nil
	}
	byManager := map[string][]roster.Profile{}
	for _, p := range s.k.Roster().List() {
		childSlug := strings.TrimSpace(p.Slug)
		if childSlug == "" || strings.EqualFold(childSlug, root) {
			continue
		}
		seenManager := map[string]bool{}
		for _, manager := range []string{strings.TrimSpace(p.OwnerAgent), strings.TrimSpace(p.ParentAgent)} {
			if manager == "" || strings.EqualFold(manager, childSlug) || seenManager[strings.ToLower(manager)] {
				continue
			}
			seenManager[strings.ToLower(manager)] = true
			byManager[strings.ToLower(manager)] = append(byManager[strings.ToLower(manager)], p)
		}
	}
	var out []roster.Profile
	seen := map[string]bool{strings.ToLower(root): true}
	var walk func(string)
	walk = func(parent string) {
		for _, child := range byManager[strings.ToLower(strings.TrimSpace(parent))] {
			key := strings.ToLower(strings.TrimSpace(child.Slug))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, child)
			walk(child.Slug)
		}
	}
	walk(root)
	sort.Slice(out, func(i, j int) bool {
		return strings.Compare(out[i].Slug, out[j].Slug) < 0
	})
	return out
}

