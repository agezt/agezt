// SPDX-License-Identifier: MIT

// Control-plane roster lifecycle handlers (Retire/Revive/SetRetired/Remove) + agentRemoveCascade.
// Code extracted from roster_lifecycle.go during the Day-93 god-file split.
// Public API unchanged.
package controlplane


import (
	"errors"
	"fmt"
	"net"

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

