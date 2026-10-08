// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/roster"
)

var impactSubsystemNames = []string{"standing", "schedule", "memory", "authored", "skill", "config", "workspace", "workflow", "mailbox"}

type fakeImpactSource struct {
	profiles map[string]core.Profile
	children map[string][]core.Profile
	held     map[string]map[string][]string // slug -> subsystem -> labels
	calls    []string
}

func (f *fakeImpactSource) Get(ref string) (core.Profile, bool) {
	f.calls = append(f.calls, "get:"+ref)
	p, ok := f.profiles[ref]
	return p, ok
}

func (f *fakeImpactSource) Subagents(slug string) []core.Profile {
	f.calls = append(f.calls, "subagents:"+slug)
	return f.children[slug]
}

func (f *fakeImpactSource) Holdings(p core.Profile) ImpactHoldings {
	f.calls = append(f.calls, "holdings:"+p.Slug)
	h := f.held[p.Slug]
	return ImpactHoldings{StandingOrders: h["standing"], Schedules: h["schedule"], Memories: h["memory"], AuthoredSharedMemories: h["authored"], Skills: h["skill"], Configs: h["config"], Workspaces: h["workspace"], WorkflowRefs: h["workflow"], MailboxMessages: h["mailbox"]}
}

func impactFixture() *fakeImpactSource {
	lead := core.Profile{Slug: "lead", Name: "Lead", Enabled: true, ParentAgent: " boss ", OwnerAgent: "owner", Description: "d", Model: " m ", MemoryScope: " scope ", RetiredMS: 7, RetiredReason: "why", Retired: true, Lifecycle: core.AgentLifecycle{Mode: " cycle ", MaxCycles: 3, CompletedCycles: 2}}
	kids := []core.Profile{
		{Slug: "a", OwnerAgent: "LEAD"},
		{Slug: "b", Name: " Bee ", ParentAgent: "lead", OwnerAgent: "lead", Retired: true},
		{Slug: "c", Name: "c"},
	}
	held := map[string]map[string][]string{"lead": {}, "a": {}, "b": {}, "c": {}}
	for _, name := range impactSubsystemNames {
		held["lead"][name] = []string{name + "-own-2", name + "-own-1"}
		held["b"][name] = []string{name + "-b"}
		held["a"][name] = []string{name + "-a2", name + "-a1"}
	}
	held["lead"]["workspace"] = nil // nothing held stays null on the wire
	return &fakeImpactSource{
		profiles: map[string]core.Profile{"lead": lead, "solo": {Slug: "solo", OwnerAgent: " owner "}},
		children: map[string][]core.Profile{"lead": kids},
		held:     held,
	}
}

func TestRosterSubagentImpactLabels(t *testing.T) {
	got := SubagentImpactLabels("lead", []core.Profile{{Slug: "z", OwnerAgent: " Lead "}, {Slug: "b", Name: " Bee ", ParentAgent: "lead", OwnerAgent: "lead", Retired: true}, {Slug: "c", Name: "c"}, {Slug: "d", ParentAgent: "x"}})
	want := []string{"Bee (b) [owner, parent] [retired]", "c [descendant]", "d [descendant]", "z [owner]"}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	if got := SubagentImpactLabels("lead", nil); got == nil || len(got) != 0 {
		t.Fatal("no sub-agents is an empty list", got)
	}
	if got := AggregateSubagentLabels([]core.Profile{{Slug: "b"}, {Slug: "a"}}, [][]string{{"y", "x"}, nil}); !reflect.DeepEqual(got, []string{"b: x", "b: y"}) {
		t.Fatal(got)
	}
	if got := AggregateSubagentLabels([]core.Profile{{Slug: "a"}}, [][]string{nil}); got != nil {
		t.Fatal("nothing held aggregates to nil", got)
	}
}

func TestRosterImpactSummaryPairsEverySubsystem(t *testing.T) {
	src := impactFixture()
	out, err := NewImpact(src).Impact(context.Background(), RefPageRequest{Ref: json.RawMessage(`"lead"`)})
	if err != nil {
		t.Fatal(err)
	}
	if out.Slug != "lead" || out.SubagentCount != 3 || !reflect.DeepEqual(out.Subagents, []string{"Bee (b) [owner, parent] [retired]", "a [owner]", "c [descendant]"}) {
		t.Fatal(out.Slug, out.Subagents)
	}
	raw, _ := json.Marshal(out)
	var wire map[string]json.RawMessage
	json.Unmarshal(raw, &wire)
	keys := map[string][4]string{
		"standing":  {"standing_orders", "standing_count", "subagent_standing_orders", "subagent_standing_count"},
		"schedule":  {"schedules", "schedule_count", "subagent_schedules", "subagent_schedule_count"},
		"memory":    {"memories", "memory_count", "subagent_memories", "subagent_memory_count"},
		"authored":  {"authored_shared_memories", "authored_shared_memory_count", "subagent_authored_shared_memories", "subagent_authored_shared_memory_count"},
		"skill":     {"skills", "skill_count", "subagent_skills", "subagent_skill_count"},
		"config":    {"configs", "config_count", "subagent_configs", "subagent_config_count"},
		"workspace": {"workspaces", "workspace_count", "subagent_workspaces", "subagent_workspace_count"},
		"workflow":  {"workflow_refs", "workflow_ref_count", "subagent_workflow_refs", "subagent_workflow_ref_count"},
		"mailbox":   {"mailbox_messages", "mailbox_message_count", "subagent_mailbox_messages", "subagent_mailbox_message_count"},
	}
	if len(wire) != 3+4*len(keys) {
		t.Fatal("wire keys", len(wire))
	}
	for name, k := range keys {
		own := `["` + name + `-own-2","` + name + `-own-1"]`
		count := "2"
		if name == "workspace" {
			own, count = "null", "0"
		}
		sub := `["a: ` + name + `-a1","a: ` + name + `-a2","b: ` + name + `-b"]`
		if string(wire[k[0]]) != own || string(wire[k[1]]) != count || string(wire[k[2]]) != sub || string(wire[k[3]]) != "3" {
			t.Fatal(name, string(wire[k[0]]), string(wire[k[1]]), string(wire[k[2]]), string(wire[k[3]]))
		}
	}
	// One holdings read per agent in the tree.
	if strings.Join(src.calls, ",") != "get:lead,subagents:lead,holdings:lead,holdings:a,holdings:b,holdings:c" {
		t.Fatal(src.calls)
	}
	solo, _ := NewImpact(impactFixture()).Impact(context.Background(), RefPageRequest{Ref: json.RawMessage(`"solo"`)})
	raw, _ = json.Marshal(solo)
	if !strings.Contains(string(raw), `"subagents":[]`) || !strings.Contains(string(raw), `"subagent_skills":null`) || !strings.Contains(string(raw), `"skills":null`) || solo.SubagentCount != 0 {
		t.Fatal("no tree", string(raw))
	}
}

func TestRosterImpactAndTombstoneErrors(t *testing.T) {
	s := NewImpact(impactFixture())
	for raw, want := range map[string]string{`{}`: "args.ref required", `{"ref":" "}`: "args.ref required", `{"ref":3}`: "args.ref must be a string", `{"ref":"ghost"}`: "unknown agent: ghost"} {
		var in RefPageRequest
		json.Unmarshal([]byte(raw), &in)
		if _, err := s.Impact(context.Background(), in); err == nil || err.Error() != want {
			t.Fatal("impact", raw, err)
		}
		if _, err := s.Tombstone(context.Background(), in); err == nil || err.Error() != want {
			t.Fatal("tombstone", raw, err)
		}
	}
}

func TestRosterTombstone(t *testing.T) {
	out, err := NewImpact(impactFixture()).Tombstone(context.Background(), RefPageRequest{Ref: json.RawMessage(`"lead"`)})
	if err != nil {
		t.Fatal(err)
	}
	want := Tombstone{
		Slug: "lead", Name: "Lead", Kind: (core.Profile{Slug: "lead", ParentAgent: " boss ", OwnerAgent: "owner"}).Kind(), Description: "d", Manager: "boss",
		Retired: true, RetiredMS: 7, RetiredReason: "why", LifecycleMode: "cycle", CompletedCycles: 2, MaxCycles: 3, MemoryScope: "scope", Model: "m",
		Footprint:        TombstoneFootprint{StandingOrders: 2, Schedules: 2, Memories: 2, AuthoredShared: 2, Skills: 2, Configs: 2, Workspaces: 0, WorkflowRefs: 2, MailboxMessages: 2, Subagents: 3},
		RetainedByDesign: TombstoneRetained{MailboxMessages: 2, WorkflowRefs: 2},
	}
	if out.Tombstone != want {
		t.Fatalf("%+v\nwant %+v", out.Tombstone, want)
	}
	solo, _ := NewImpact(impactFixture()).Tombstone(context.Background(), RefPageRequest{Ref: json.RawMessage(`"solo"`)})
	if solo.Tombstone.Manager != "owner" {
		t.Fatal("manager falls back to the owner", solo.Tombstone.Manager)
	}
	// Distinct counts per subsystem reach the right footprint fields.
	src := impactFixture()
	for i, name := range impactSubsystemNames {
		src.held["lead"][name] = make([]string, i+1)
	}
	got, _ := NewImpact(src).Tombstone(context.Background(), RefPageRequest{Ref: json.RawMessage(`"lead"`)})
	if f := got.Tombstone.Footprint; f != (TombstoneFootprint{StandingOrders: 1, Schedules: 2, Memories: 3, AuthoredShared: 4, Skills: 5, Configs: 6, Workspaces: 7, WorkflowRefs: 8, MailboxMessages: 9, Subagents: 3}) || got.Tombstone.RetainedByDesign != (TombstoneRetained{MailboxMessages: 9, WorkflowRefs: 8}) {
		t.Fatalf("%+v %+v", f, got.Tombstone.RetainedByDesign)
	}
}

func TestRosterImpactOperations(t *testing.T) {
	if _, err := ImpactOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	src := impactFixture()
	ops, err := ImpactOperations(func(ctx context.Context) *ImpactService {
		if ctx.Value(listRouteKey{}) != "selected" {
			t.Fatal("route lost")
		}
		return NewImpact(src)
	})
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	impact, tombstone := ops[0].Spec(), ops[1].Spec()
	if impact.Name != "agent_impact" || !impact.ReadOnly || impact.Authz != opapi.PrimaryOnly || impact.Tenancy != opapi.Primary || !impact.AllowUnknownInput || impact.Input != reflect.TypeFor[RefPageRequest]() || impact.Output != reflect.TypeFor[ImpactOutput]() || impact.HTTP != (opapi.HTTP{Method: "GET", Path: "/api/agents/impact"}) {
		t.Fatal(impact)
	}
	if tombstone.Name != "agent_tombstone" || !tombstone.ReadOnly || tombstone.Authz != opapi.PrimaryOnly || tombstone.Tenancy != opapi.Primary || !tombstone.AllowUnknownInput || tombstone.Output != reflect.TypeFor[TombstoneOutput]() || tombstone.HTTP != (opapi.HTTP{}) {
		t.Fatal(tombstone)
	}
	raw, _ := json.Marshal(ImpactOutput{Subagents: []string{}})
	if schema.ValidateJSON(impact.OutputSchema, raw) != nil || schema.ValidateJSON(impact.OutputSchema, json.RawMessage(strings.Replace(string(raw), `"skill_count":0`, `"skill_count":"0"`, 1))) == nil {
		t.Fatal("impact schema", string(raw))
	}
	d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}})
	for name, want := range map[string]reflect.Type{"agent_impact": reflect.TypeFor[ImpactOutput](), "agent_tombstone": reflect.TypeFor[TombstoneOutput]()} {
		out, err := d.Dispatch(context.Background(), opapi.Caller{}, name, json.RawMessage(`{"ref":"lead"}`), nil)
		if err != nil || reflect.TypeOf(out) != want {
			t.Fatal(name, out, err)
		}
	}
	for _, principal := range []opapi.Principal{{Kind: opapi.Tenant, Tenant: "acme"}, {Kind: opapi.Agent}} {
		denied, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{principal}, Router: listRoute{}})
		for _, name := range []string{"agent_impact", "agent_tombstone"} {
			if _, err := denied.Dispatch(context.Background(), opapi.Caller{}, name, json.RawMessage(`{"ref":"lead"}`), nil); err == nil {
				t.Fatal("non-primary read", name)
			}
		}
	}
	src.calls = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{}, "agent_impact", json.RawMessage(`{"ref":"lead"}`), nil); !errors.Is(err, context.Canceled) || src.calls != nil {
		t.Fatal("canceled read", err, src.calls)
	}
}
