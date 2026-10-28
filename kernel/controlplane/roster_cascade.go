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
	appschedule "github.com/agezt/agezt/kernel/app/schedule"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/boardapi"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
)

// impactPorts binds the host's stores to the application's impact
// derivation.
func (s *Server) impactPorts() approster.ImpactPorts {
	return approster.ImpactPorts{
		Schedules:     appschedule.PortsFromKernel(s.k).Entries,
		MemoryRecords: func() ([]memoryapi.Record, error) { return s.k.MemoryActive() },
		// A daemon with NO skill library holds a nil port (W3.55): the cascade
		// sees no skills rather than calling into nothing.
		Skills: func() ([]skillapi.Skill, error) {
			return s.skillPorts().List()
		},
		ConfigEntries: func() []*configcenter.ConfigEntry {
			entries, _ := s.configCenterPorts().Entries()
			return entries
		},
		WorkspaceInfo: s.agentWorkspaceInfo,
		Workflows:     s.workflowPorts().Entries,
		Mailbox: func() []boardapi.Message {
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

func (n nativeImpactSource) Get(ref string) (agentapi.Profile, bool) {
	return n.s.rosterPorts().Get(ref)
}

func (n nativeImpactSource) Subagents(slug string) []agentapi.Profile {
	return approster.Subagents(n.s.rosterPorts().List(), slug)
}

func (n nativeImpactSource) Holdings(p agentapi.Profile) approster.ImpactHoldings {
	ports := n.s.impactPorts()
	return approster.ImpactHoldings{
		StandingOrders:         n.s.rosterPorts().Impact(p.Slug),
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
func (s *Server) agentSubagents(slug string) []agentapi.Profile {
	return approster.Subagents(s.rosterPorts().List(), slug)
}

// agentRemovalMailboxImpact is the removal payload's retained-message list:
// the agent's own threads plus, when sub-agents are cascaded, theirs.
func (s *Server) agentRemovalMailboxImpact(slug string, subagents []agentapi.Profile, includeSubagents bool) []string {
	return approster.RemovalMailboxImpact(s.impactPorts(), slug, subagents, includeSubagents)
}

// agentWorkflowImpact lists the workflow nodes that reference the agent.
func (s *Server) agentWorkflowImpact(p agentapi.Profile) []string {
	return approster.WorkflowImpact(s.impactPorts(), p)
}
