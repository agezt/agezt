// SPDX-License-Identifier: MIT

package controlplane

// The impact analysis' host bindings: the stores the application's impact
// derivation reads. The DERIVATION — which labels belong to the agent, how a
// mailbox message reads, whether a workflow node references the agent, which
// agents are sub-agents — lives in the application layer
// (`kernel/app/roster/impact_cascade.go`); what stays here is the host's
// data access.

import (
	approster "github.com/agezt/agezt/kernel/app/roster"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/modules/board/board"
	"github.com/agezt/agezt/kernel/modules/knowledge/memory"
	"github.com/agezt/agezt/kernel/modules/skills/skill"
	"github.com/agezt/agezt/kernel/modules/workflows/workflow"
	core "github.com/agezt/agezt/kernel/roster"
)

// impactPorts binds the host's stores to the application's impact
// derivation.
func (s *Server) impactPorts() approster.ImpactPorts {
	return approster.ImpactPorts{
		Schedules:     func() []cadence.Entry { return s.k.Schedules().List() },
		MemoryRecords: func() ([]memory.Record, error) { return s.k.Memory().Active() },
		Skills:        func() ([]skill.Skill, error) { return s.k.Forge().List() },
		ConfigEntries: func() []*configcenter.ConfigEntry {
			if s.k.ConfigCenter() == nil {
				return nil
			}
			return s.k.ConfigCenter().ListEntries()
		},
		WorkspaceInfo: s.agentWorkspaceInfo,
		Workflows: func() []workflow.Workflow {
			if s.k.Workflows() == nil {
				return nil
			}
			return s.k.Workflows().List()
		},
		Mailbox: func() []board.Message {
			st, err := s.boardReader()
			if err != nil || st == nil {
				return nil
			}
			return st.Read("", boardReadMaxLimit)
		},
	}
}

// nativeImpactSource reads the teardown impact from the live subsystems. The
// preview's shape and the sub-agent aggregation live in approster.ImpactService.
type nativeImpactSource struct{ s *Server }

func (n nativeImpactSource) Get(ref string) (core.Profile, bool) { return n.s.k.Roster().Get(ref) }

func (n nativeImpactSource) Subagents(slug string) []core.Profile {
	return approster.Subagents(n.s.k.Roster().List(), slug)
}

func (n nativeImpactSource) Holdings(p core.Profile) approster.ImpactHoldings {
	ports := n.s.impactPorts()
	return approster.ImpactHoldings{
		StandingOrders:         n.s.k.AgentImpact(p.Slug),
		Schedules:              approster.ScheduleImpact(ports, p),
		Memories:               approster.MemoryImpact(ports, p),
		AuthoredSharedMemories: approster.AuthoredSharedMemoryImpact(ports, p),
		Skills:                 approster.SkillImpact(ports, p),
		Configs:                approster.ConfigImpact(ports, p),
		Workspaces:             approster.WorkspaceImpact(ports, p),
		WorkflowRefs:           approster.WorkflowImpact(ports, p),
		MailboxMessages:        approster.MailboxImpact(ports, p),
	}
}

// impactService binds the live subsystems to the application's impact
// service.
func (s *Server) impactService() *approster.ImpactService {
	return approster.NewImpact(nativeImpactSource{s})
}

// agentSubagents returns the agent's sub-agent tree.
func (s *Server) agentSubagents(slug string) []core.Profile {
	return approster.Subagents(s.k.Roster().List(), slug)
}

// agentRemovalMailboxImpact is the removal payload's retained-message list:
// the agent's own threads plus, when sub-agents are cascaded, theirs.
func (s *Server) agentRemovalMailboxImpact(slug string, subagents []core.Profile, includeSubagents bool) []string {
	return approster.RemovalMailboxImpact(s.impactPorts(), slug, subagents, includeSubagents)
}

// agentWorkflowImpact lists the workflow nodes that reference the agent.
func (s *Server) agentWorkflowImpact(p core.Profile) []string {
	return approster.WorkflowImpact(s.impactPorts(), p)
}
