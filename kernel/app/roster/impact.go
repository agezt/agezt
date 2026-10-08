// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/roster"
)

// ImpactHoldings is what each teardown subsystem currently holds for one agent,
// as sorted labels. Workflow references and mailbox messages are reported but
// never cleaned by a removal: a workflow node or a message thread belongs to the
// workflow or the conversation, not to the agent it names.
type ImpactHoldings struct {
	StandingOrders         []string
	Schedules              []string
	Memories               []string
	AuthoredSharedMemories []string
	Skills                 []string
	Configs                []string
	Workspaces             []string
	WorkflowRefs           []string
	MailboxMessages        []string
}

func (h ImpactHoldings) lists() [][]string {
	return [][]string{h.StandingOrders, h.Schedules, h.Memories, h.AuthoredSharedMemories, h.Skills, h.Configs, h.Workspaces, h.WorkflowRefs, h.MailboxMessages}
}

// ImpactSource reads the agent's sub-agent tree and each subsystem's holdings.
type ImpactSource interface {
	Get(ref string) (core.Profile, bool)
	Subagents(slug string) []core.Profile
	Holdings(p core.Profile) ImpactHoldings
}

// ImpactOutput is the teardown preview: for every subsystem, what the agent holds
// and what its sub-agents hold. The wire names are irregular and read verbatim
// by the console.
type ImpactOutput struct {
	Slug                              string   `json:"slug"`
	Subagents                         []string `json:"subagents"`
	SubagentCount                     int      `json:"subagent_count"`
	StandingOrders                    []string `json:"standing_orders"`
	StandingCount                     int      `json:"standing_count"`
	SubagentStandingOrders            []string `json:"subagent_standing_orders"`
	SubagentStandingCount             int      `json:"subagent_standing_count"`
	Schedules                         []string `json:"schedules"`
	ScheduleCount                     int      `json:"schedule_count"`
	SubagentSchedules                 []string `json:"subagent_schedules"`
	SubagentScheduleCount             int      `json:"subagent_schedule_count"`
	Memories                          []string `json:"memories"`
	MemoryCount                       int      `json:"memory_count"`
	SubagentMemories                  []string `json:"subagent_memories"`
	SubagentMemoryCount               int      `json:"subagent_memory_count"`
	AuthoredSharedMemories            []string `json:"authored_shared_memories"`
	AuthoredSharedMemoryCount         int      `json:"authored_shared_memory_count"`
	SubagentAuthoredSharedMemories    []string `json:"subagent_authored_shared_memories"`
	SubagentAuthoredSharedMemoryCount int      `json:"subagent_authored_shared_memory_count"`
	Skills                            []string `json:"skills"`
	SkillCount                        int      `json:"skill_count"`
	SubagentSkills                    []string `json:"subagent_skills"`
	SubagentSkillCount                int      `json:"subagent_skill_count"`
	Configs                           []string `json:"configs"`
	ConfigCount                       int      `json:"config_count"`
	SubagentConfigs                   []string `json:"subagent_configs"`
	SubagentConfigCount               int      `json:"subagent_config_count"`
	Workspaces                        []string `json:"workspaces"`
	WorkspaceCount                    int      `json:"workspace_count"`
	SubagentWorkspaces                []string `json:"subagent_workspaces"`
	SubagentWorkspaceCount            int      `json:"subagent_workspace_count"`
	WorkflowRefs                      []string `json:"workflow_refs"`
	WorkflowRefCount                  int      `json:"workflow_ref_count"`
	SubagentWorkflowRefs              []string `json:"subagent_workflow_refs"`
	SubagentWorkflowRefCount          int      `json:"subagent_workflow_ref_count"`
	MailboxMessages                   []string `json:"mailbox_messages"`
	MailboxMessageCount               int      `json:"mailbox_message_count"`
	SubagentMailboxMessages           []string `json:"subagent_mailbox_messages"`
	SubagentMailboxMessageCount       int      `json:"subagent_mailbox_message_count"`
}

// impactFields pairs each subsystem, in ImpactHoldings.lists order, with its
// own/sub-agent list and count fields.
func (o *ImpactOutput) impactFields() [][4]any {
	return [][4]any{
		{&o.StandingOrders, &o.StandingCount, &o.SubagentStandingOrders, &o.SubagentStandingCount},
		{&o.Schedules, &o.ScheduleCount, &o.SubagentSchedules, &o.SubagentScheduleCount},
		{&o.Memories, &o.MemoryCount, &o.SubagentMemories, &o.SubagentMemoryCount},
		{&o.AuthoredSharedMemories, &o.AuthoredSharedMemoryCount, &o.SubagentAuthoredSharedMemories, &o.SubagentAuthoredSharedMemoryCount},
		{&o.Skills, &o.SkillCount, &o.SubagentSkills, &o.SubagentSkillCount},
		{&o.Configs, &o.ConfigCount, &o.SubagentConfigs, &o.SubagentConfigCount},
		{&o.Workspaces, &o.WorkspaceCount, &o.SubagentWorkspaces, &o.SubagentWorkspaceCount},
		{&o.WorkflowRefs, &o.WorkflowRefCount, &o.SubagentWorkflowRefs, &o.SubagentWorkflowRefCount},
		{&o.MailboxMessages, &o.MailboxMessageCount, &o.SubagentMailboxMessages, &o.SubagentMailboxMessageCount},
	}
}

// SubagentImpactLabels names each sub-agent with its relation to slug and its
// retired state, sorted.
func SubagentImpactLabels(slug string, children []core.Profile) []string {
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
		label += " [" + strings.Join(roles, ", ") + "]"
		if child.Retired {
			label += " [retired]"
		}
		out = append(out, label)
	}
	sort.Strings(out)
	return out
}

// AggregateSubagentLabels prefixes each child's labels (lists[i] for
// children[i]) with its slug and sorts the union; nil when no child holds
// anything.
func AggregateSubagentLabels(children []core.Profile, lists [][]string) []string {
	var out []string
	for i, child := range children {
		for _, label := range lists[i] {
			out = append(out, child.Slug+": "+label)
		}
	}
	sort.Strings(out)
	return out
}

// ImpactService answers the teardown preview and the tombstone, both read-only.
type ImpactService struct{ src ImpactSource }

func NewImpact(src ImpactSource) *ImpactService { return &ImpactService{src: src} }

// Summary is the teardown preview for a resolved agent.
func (s *ImpactService) Summary(p core.Profile) ImpactOutput {
	children := s.src.Subagents(p.Slug)
	labels := SubagentImpactLabels(p.Slug, children)
	out := ImpactOutput{Slug: p.Slug, Subagents: labels, SubagentCount: len(labels)}
	own := s.src.Holdings(p).lists()
	kids := make([][][]string, len(children))
	for i, child := range children {
		kids[i] = s.src.Holdings(child).lists()
	}
	for i, f := range out.impactFields() {
		*f[0].(*[]string), *f[1].(*int) = own[i], len(own[i])
		perChild := make([][]string, len(children))
		for j := range children {
			perChild[j] = kids[j][i]
		}
		agg := AggregateSubagentLabels(children, perChild)
		*f[2].(*[]string), *f[3].(*int) = agg, len(agg)
	}
	return out
}

func (s *ImpactService) resolve(in RefPageRequest) (core.Profile, error) {
	ref, err := in.ref()
	if err != nil {
		return core.Profile{}, err
	}
	p, ok := s.src.Get(ref)
	if !ok {
		return core.Profile{}, errors.New("unknown agent: " + ref)
	}
	return p, nil
}

func (s *ImpactService) Impact(_ context.Context, in RefPageRequest) (ImpactOutput, error) {
	p, err := s.resolve(in)
	if err != nil {
		return ImpactOutput{}, err
	}
	return s.Summary(p), nil
}

// TombstoneFootprint is the durable footprint the removal cascade would act on.
type TombstoneFootprint struct {
	StandingOrders  int `json:"standing_orders"`
	Schedules       int `json:"schedules"`
	Memories        int `json:"memories"`
	AuthoredShared  int `json:"authored_shared"`
	Skills          int `json:"skills"`
	Configs         int `json:"configs"`
	Workspaces      int `json:"workspaces"`
	WorkflowRefs    int `json:"workflow_refs"`
	MailboxMessages int `json:"mailbox_messages"`
	Subagents       int `json:"subagents"`
}

// TombstoneRetained is what a removal keeps by design: mailbox messages and
// workflow references are the agent's lasting trace.
type TombstoneRetained struct {
	MailboxMessages int `json:"mailbox_messages"`
	WorkflowRefs    int `json:"workflow_refs"`
}

// Tombstone is an agent's portable death certificate: identity, lifecycle and
// retirement record, and durable footprint.
type Tombstone struct {
	Slug             string             `json:"slug"`
	Name             string             `json:"name"`
	Kind             string             `json:"kind"`
	System           bool               `json:"system"`
	Description      string             `json:"description"`
	Manager          string             `json:"manager"`
	Retired          bool               `json:"retired"`
	RetiredMS        int64              `json:"retired_ms"`
	RetiredReason    string             `json:"retired_reason"`
	LifecycleMode    string             `json:"lifecycle_mode"`
	CompletedCycles  int                `json:"completed_cycles"`
	MaxCycles        int                `json:"max_cycles"`
	MemoryScope      string             `json:"memory_scope"`
	Model            string             `json:"model"`
	Footprint        TombstoneFootprint `json:"footprint"`
	RetainedByDesign TombstoneRetained  `json:"retained_by_design"`
}

type TombstoneOutput struct {
	Tombstone Tombstone `json:"tombstone"`
}

func (s *ImpactService) Tombstone(_ context.Context, in RefPageRequest) (TombstoneOutput, error) {
	p, err := s.resolve(in)
	if err != nil {
		return TombstoneOutput{}, err
	}
	impact := s.Summary(p)
	manager := strings.TrimSpace(p.ParentAgent)
	if manager == "" {
		manager = strings.TrimSpace(p.OwnerAgent)
	}
	return TombstoneOutput{Tombstone: Tombstone{
		Slug:            p.Slug,
		Name:            p.Name,
		Kind:            p.Kind(),
		System:          p.System,
		Description:     p.Description,
		Manager:         manager,
		Retired:         p.Retired,
		RetiredMS:       p.RetiredMS,
		RetiredReason:   p.RetiredReason,
		LifecycleMode:   strings.TrimSpace(p.Lifecycle.Mode),
		CompletedCycles: p.Lifecycle.CompletedCycles,
		MaxCycles:       p.Lifecycle.MaxCycles,
		MemoryScope:     strings.TrimSpace(p.MemoryScope),
		Model:           strings.TrimSpace(p.Model),
		Footprint: TombstoneFootprint{
			StandingOrders:  impact.StandingCount,
			Schedules:       impact.ScheduleCount,
			Memories:        impact.MemoryCount,
			AuthoredShared:  impact.AuthoredSharedMemoryCount,
			Skills:          impact.SkillCount,
			Configs:         impact.ConfigCount,
			Workspaces:      impact.WorkspaceCount,
			WorkflowRefs:    impact.WorkflowRefCount,
			MailboxMessages: impact.MailboxMessageCount,
			Subagents:       impact.SubagentCount,
		},
		RetainedByDesign: TombstoneRetained{MailboxMessages: impact.MailboxMessageCount, WorkflowRefs: impact.WorkflowRefCount},
	}}, nil
}

func ImpactOperations(provider func(context.Context) *ImpactService) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("roster impact provider required")
	}
	impactSchema, err := schema.FromType(reflect.TypeFor[ImpactOutput](), false)
	if err != nil {
		return nil, err
	}
	tombstoneSchema, err := schema.FromType(reflect.TypeFor[TombstoneOutput](), false)
	if err != nil {
		return nil, err
	}
	input := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{}}}`)
	impact, err := app.NewOperation(opapi.Spec{Name: "agent_impact", ReadOnly: true, OutputSchema: impactSchema, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: input, HTTP: opapi.HTTP{Method: "GET", Path: "/api/agents/impact"}}, func(ctx context.Context, in RefPageRequest) (ImpactOutput, error) {
		return provider(ctx).Impact(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	tombstone, err := app.NewOperation(opapi.Spec{Name: "agent_tombstone", ReadOnly: true, OutputSchema: tombstoneSchema, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: input}, func(ctx context.Context, in RefPageRequest) (TombstoneOutput, error) {
		return provider(ctx).Tombstone(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{impact, tombstone}, nil
}
