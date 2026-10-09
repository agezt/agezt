// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/roster"
)

type permTool struct{ name, description string }

func (p permTool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{Name: p.name, Description: p.description}
}

func (p permTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	return toolapi.Result{}, nil
}

type permFixture struct {
	profiles   map[string]core.Profile
	tools      map[string]toolapi.Tool
	outcomes   map[edict.Capability]edict.Outcome
	decided    []string
	entries    []*configcenter.ConfigEntry
	noCenter   bool
	gets       int
	updates    int
	updateErr  error
	updateMiss bool
}

func newPermFixture() *permFixture {
	return &permFixture{
		profiles: map[string]core.Profile{"a": {Slug: "a", Enabled: true}, "gone": {Slug: "gone", Retired: true}},
		tools:    map[string]toolapi.Tool{},
		outcomes: map[edict.Capability]edict.Outcome{},
	}
}

func (f *permFixture) service() *PermissionService {
	return NewPermissions(PermissionPorts{
		Get: func(ref string) (core.Profile, bool) {
			f.gets++
			p, ok := f.profiles[ref]
			return p, ok
		},
		Update: func(ref string, mutate func(*core.Profile)) (core.Profile, bool, error) {
			f.updates++
			if f.updateErr != nil {
				return core.Profile{}, false, f.updateErr
			}
			p, ok := f.profiles[ref]
			if !ok || f.updateMiss {
				return core.Profile{}, false, nil
			}
			mutate(&p)
			f.profiles[ref] = p
			return p, true, nil
		},
		Tools: func() map[string]toolapi.Tool { return f.tools },
		Decide: func(c edict.Capability, ceiling edict.TrustLevel) edict.Outcome {
			f.decided = append(f.decided, string(c)+"@"+ceiling.String())
			if out, ok := f.outcomes[c]; ok {
				return out
			}
			return edict.Outcome{Decision: edict.DecisionAllow, Level: edict.LevelAllow, Reason: "allow"}
		},
		Config: func() ([]*configcenter.ConfigEntry, bool) { return f.entries, !f.noCenter },
	})
}

func permRequest(t *testing.T, raw string) PermissionsRequest {
	t.Helper()
	var in PermissionsRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func capRequest(t *testing.T, raw string) CapabilitiesRequest {
	t.Helper()
	var in CapabilitiesRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func TestPermissionsRef(t *testing.T) {
	ctx := context.Background()
	for raw, want := range map[string]string{`{}`: "args.ref required", `{"ref":""}`: "args.ref required", `{"ref":"  "}`: "args.ref required", `{"ref":3}`: "args.ref must be a string", `{"ref":null}`: "args.ref must be a string"} {
		f := newPermFixture()
		if _, err := f.service().Permissions(ctx, permRequest(t, raw)); err == nil || err.Error() != want || f.gets != 0 {
			t.Fatal(raw, err, f.gets)
		}
		withField := strings.Replace(raw, "}", `,"workdir":3}`, 1)
		if raw == `{}` {
			withField = `{"workdir":3}`
		}
		if _, err := f.service().Capabilities(ctx, capRequest(t, withField)); err == nil || err.Error() != want || f.gets != 0 {
			t.Fatal("capabilities checks the ref first", withField, err)
		}
	}
	f := newPermFixture()
	if _, err := f.service().Permissions(ctx, permRequest(t, `{"ref":" a"}`)); err == nil || err.Error() != "unknown agent:  a" {
		t.Fatal("the ref passes untrimmed", err)
	}
}

func TestPermissionRows(t *testing.T) {
	f := newPermFixture()
	for _, name := range []string{"shell", "beta", "alpha", "memory"} {
		f.tools[name] = permTool{name: name, description: "<" + name + ">"}
	}
	f.tools["zz"] = permTool{name: "aa", description: "alias"}
	f.outcomes["shell"] = edict.Outcome{Decision: edict.DecisionDeny, Level: edict.LevelDeny, Reason: "hard", HardDenied: true}
	f.outcomes["memory"] = edict.Outcome{Decision: edict.DecisionAllow, Level: edict.LevelAsk, Reason: "ask", RequiresApproval: true}
	f.outcomes["zz"] = edict.Outcome{Decision: edict.DecisionAllow, Level: edict.LevelAskFirst, Reason: "would", WouldAsk: true}
	f.profiles["a"] = core.Profile{Slug: "a", Enabled: true, ToolAllow: []string{" Shell ", "memory", "zz", "BETA"}, ToolDeny: []string{"BETA"}, TrustCeiling: " L2 "}
	out, err := f.service().Permissions(context.Background(), permRequest(t, `{"ref":"a"}`))
	if err != nil {
		t.Fatal(err)
	}
	f2 := false
	t2 := true
	want := []PermissionRow{
		{Name: "alpha", Description: "<alpha>", Capability: "alpha", Status: "hidden", Source: "agent_allow", Reason: "not in agent tool allowlist"},
		{Name: "beta", Description: "<beta>", Capability: "beta", Status: "denied", Source: "agent_deny", Reason: "agent tool denylist"},
		{Name: "memory", Description: "<memory>", Capability: "memory", Allowed: true, Ask: true, Status: "L1", Source: "edict", Reason: "ask", Level: "L1", HardDenied: &f2, RequiresApproval: true},
		{Name: "shell", Description: "<shell>", Capability: "shell", Status: "denied", Source: "edict", Reason: "hard", Level: "L0", HardDenied: &t2},
		{Name: "aa", Description: "alias", Capability: "zz", Allowed: true, Ask: true, Status: "L2", Source: "edict", Reason: "would", Level: "L2", HardDenied: &f2},
	}
	if !reflect.DeepEqual(out.Permissions, want) || out.Count != 5 || out.AllowedCount != 2 {
		t.Fatalf("rows sort by registration name; deny wins over allow; the allowlist hides the rest: %+v", out.Permissions)
	}
	if !reflect.DeepEqual(f.decided, []string{"memory@L2", "shell@L2", "zz@L2"}) {
		t.Fatal("the trust ceiling bounds every policy decision", f.decided)
	}
	raw, _ := json.Marshal(out.Permissions[0])
	if strings.Contains(string(raw), "hard_denied") || strings.Contains(string(raw), "requires_approval") || !strings.Contains(string(raw), `"level":""`) {
		t.Fatal("rows the agent lists decide carry no policy fields", string(raw))
	}
	for ceiling, want := range map[string]string{"": "L4", "bogus": "L4", "L0": "L0"} {
		f.decided = nil
		f.profiles["a"] = core.Profile{Slug: "a", TrustCeiling: ceiling}
		if _, err := f.service().Permissions(context.Background(), permRequest(t, `{"ref":"a"}`)); err != nil || f.decided[0] != "alpha@"+want {
			t.Fatal("an empty or unparsable ceiling decides at allow", ceiling, f.decided)
		}
	}
}

func TestPermissionConfigRows(t *testing.T) {
	f := newPermFixture()
	f.noCenter = true
	out, _ := f.service().Permissions(context.Background(), permRequest(t, `{"ref":"a"}`))
	if raw, _ := json.Marshal(out); out.ConfigEntries != nil || !strings.Contains(string(raw), `"config_entries":null`) || !strings.Contains(string(raw), `"permissions":[]`) {
		t.Fatal("no config center reports null entries", string(raw))
	}
	f.noCenter = false
	f.entries = []*configcenter.ConfigEntry{}
	if out, _ := f.service().Permissions(context.Background(), permRequest(t, `{"ref":"a"}`)); out.ConfigEntries == nil || len(out.ConfigEntries) != 0 {
		t.Fatal("an empty center reports an empty list", out.ConfigEntries)
	}
	mk := func(key string, edit func(*configcenter.ConfigEntry)) *configcenter.ConfigEntry {
		e := configcenter.NewConfigEntry(key, "v")
		edit(e)
		return e
	}
	f.entries = []*configcenter.ConfigEntry{
		mk("z-excluded", func(e *configcenter.ConfigEntry) { e.ExcludedAgents = []string{" A "}; e.AllowedAgents = []string{"a"} }),
		mk("agent/a/mode", func(e *configcenter.ConfigEntry) { e.Description = "<mine>" }),
		mk("m-allowed", func(e *configcenter.ConfigEntry) { e.AllowedAgents = []string{"x", " A "} }),
		mk("n-other", func(e *configcenter.ConfigEntry) { e.AllowedAgents = []string{"x"} }),
	}
	out, _ = f.service().Permissions(context.Background(), permRequest(t, `{"ref":"a"}`))
	want := []ConfigPermissionRow{
		{Key: "agent/a/mode", Rating: "internal", Visible: true, Source: "config_global", Reason: "visible to all eligible agents", Owned: true, Description: "<mine>"},
		{Key: "m-allowed", Rating: "internal", Visible: true, Source: "config_allowed", Reason: "agent is in config allowed_agents", AllowedAgents: []string{"x", " A "}},
		{Key: "n-other", Rating: "internal", Source: "config_allowed", Reason: "not in config allowed_agents", AllowedAgents: []string{"x"}},
		{Key: "z-excluded", Rating: "internal", Source: "config_excluded", Reason: "agent is in config excluded_agents", AllowedAgents: []string{"a"}, ExcludedAgents: []string{" A "}},
	}
	if !reflect.DeepEqual(out.ConfigEntries, want) {
		t.Fatalf("entries sort by key; exclusion wins; agents match trimmed and case-insensitively: %+v", out.ConfigEntries)
	}
	g := out.Governance
	if g.ConfigCount != 4 || g.ConfigVisibleCount != 2 || g.ConfigHiddenCount != 2 || g.ConfigOwnedCount != 1 || !reflect.DeepEqual(g.VisibleConfigs, []string{"agent/a/mode", "m-allowed"}) || !reflect.DeepEqual(g.HiddenConfigs, []string{"n-other", "z-excluded"}) {
		t.Fatalf("governance counts config visibility: %+v", g)
	}
}

func TestConfigEntryBelongsToAgent(t *testing.T) {
	if ConfigEntryBelongsToAgent(nil, "a") || ConfigEntryBelongsToAgent(configcenter.NewConfigEntry("agent/a/x", "v"), " ") {
		t.Fatal("no entry or no slug owns nothing")
	}
	for _, key := range []string{"agent/a/x", " AGENTS/A/x", "agent.a.x", "agents.a.x"} {
		if !ConfigEntryBelongsToAgent(configcenter.NewConfigEntry(key, "v"), " A ") {
			t.Fatal(key)
		}
	}
	for _, key := range []string{"agent/ab/x", "agent/a", "x/agent/a/"} {
		if ConfigEntryBelongsToAgent(configcenter.NewConfigEntry(key, "v"), "a") {
			t.Fatal(key)
		}
	}
	byCreator := configcenter.NewConfigEntry("k", "v")
	byCreator.CreatedBy = " A "
	if !ConfigEntryBelongsToAgent(byCreator, "a") {
		t.Fatal("the creator owns the entry")
	}
	for _, tag := range []string{"agent:a", " Agent/A ", "owner:a", "owner/a"} {
		e := configcenter.NewConfigEntry("k", "v")
		e.Tags = []string{"other", tag}
		if !ConfigEntryBelongsToAgent(e, "a") {
			t.Fatal(tag)
		}
	}
	for _, key := range []string{"agent", "agent_slug", "owner_agent", "parent_agent"} {
		e := configcenter.NewConfigEntry("k", "v")
		e.Metadata = map[string]string{key: " A "}
		if !ConfigEntryBelongsToAgent(e, "a") {
			t.Fatal(key)
		}
	}
	e := configcenter.NewConfigEntry("k", "v")
	e.Metadata = map[string]string{"other": "a"}
	e.Tags = []string{"a"}
	if ConfigEntryBelongsToAgent(e, "a") {
		t.Fatal("other metadata and bare tags do not own")
	}
}

func TestWakeAccess(t *testing.T) {
	managed := false
	cases := []struct {
		p                                  core.Profile
		status, reason, scope, manager     string
		direct, delegation, directCallable bool
		sources                            []string
	}{
		{core.Profile{Slug: "a", Enabled: true}, "direct", "directly callable", "any", "", true, true, true, []string{}},
		{core.Profile{Slug: "a", Enabled: false, Retired: true}, "retired", "agent is retired", "any", "", false, false, true, []string{}},
		{core.Profile{Slug: "a"}, "paused", "agent is paused", "any", "", false, false, true, []string{}},
		{core.Profile{Slug: "a", Enabled: true, DirectCallable: &managed, OwnerAgent: " boss ", ParentAgent: "lead"}, "managed", "managed by lead", "manager", "lead", false, true, false, []string{"boss", "lead"}},
		{core.Profile{Slug: "a", Enabled: true, DirectCallable: &managed, OwnerAgent: "boss"}, "managed", "managed by boss", "manager", "boss", false, true, false, []string{"boss"}},
		{core.Profile{Slug: "a", Enabled: true, DirectCallable: &managed, OwnerAgent: "Boss", ParentAgent: " boss "}, "managed", "managed by boss", "manager", "boss", false, true, false, []string{"Boss"}},
		{core.Profile{Slug: "a", Enabled: true, DirectCallable: &managed}, "managed", "managed sub-agent requires parent/owner delegation", "manager", "", false, false, false, []string{}},
	}
	for i, c := range cases {
		w := wakeAccess(c.p)
		if w.Status != c.status || w.Reason != c.reason || w.DelegationScope != c.scope || w.Manager != c.manager || w.DirectAllowed != c.direct || w.ScheduleAllowed != c.direct || w.ChannelAllowed != c.direct || w.OperatorAllowed != c.direct || w.DelegationAllowed != c.delegation || w.DirectCallable != c.directCallable || !reflect.DeepEqual(w.DelegationSources, c.sources) || w.Kind != c.p.Kind() || w.Enabled != c.p.Enabled || w.Retired != c.p.Retired {
			t.Fatalf("case %d: %+v", i, w)
		}
	}
	if w := wakeAccess(core.Profile{OwnerAgent: " o ", ParentAgent: " p ", System: true}); w.OwnerAgent != "o" || w.ParentAgent != "p" || !w.System {
		t.Fatal(w)
	}
}

func TestGovernance(t *testing.T) {
	allow := PermissionRow{Name: "a", Allowed: true}
	ask := PermissionRow{Name: "b", Allowed: true, Ask: true}
	blocked := PermissionRow{Name: "c"}
	unnamed := PermissionRow{Allowed: true}
	g := governance(core.Profile{Slug: " s "}, []PermissionRow{allow, unnamed}, nil)
	if g.Risk != "open" || g.TrustCeiling != "L4" || g.ToolPolicy != "default" || g.MemoryPolicy != "default:s" || g.MemoryWrites != "enabled" || !reflect.DeepEqual(g.DirectTools, []string{"a"}) || g.AllowedCount != 2 {
		t.Fatalf("an unrestricted agent with only allowed tools is open: %+v", g)
	}
	if g.Summary != "tools 2/2 allowed, 0 ask, 0 blocked, config 0/0 visible, trust L4" || g.PermissionPassport != "trust L4, tools default, 2 direct, 0 ask, 0 blocked, memory default:s, memory_writes enabled" || g.AuthorityBoundary != "direct agent · owns soul, memory scope, tool policy, trust ceiling, and config overrides" {
		t.Fatalf("%q / %q / %q", g.Summary, g.PermissionPassport, g.AuthorityBoundary)
	}
	if g.ExecutionBoundary != "agent identity owns tools, memory, model route, retry, and repair; schedules/workflows invoke through this policy" || g.DirectTools == nil || g.AskTools == nil || g.BlockedTools == nil || g.VisibleConfigs == nil || g.HiddenConfigs == nil {
		t.Fatal("lists are never null", g)
	}
	if g := governance(core.Profile{}, nil, nil); g.Risk != "governed" {
		t.Fatal("no tools is governed", g.Risk)
	}
	for _, p := range []core.Profile{{TrustCeiling: "L3"}, {ToolAllow: []string{"x"}}, {ToolDeny: []string{"x"}}} {
		if g := governance(p, []PermissionRow{allow}, nil); g.Risk != "restricted" {
			t.Fatal("any restriction is restricted", p, g.Risk)
		}
	}
	g = governance(core.Profile{Slug: "s"}, []PermissionRow{allow, ask, blocked}, nil)
	if g.Risk != "restricted" || g.AskCount != 1 || g.BlockedCount != 1 || !reflect.DeepEqual(g.AskTools, []string{"b"}) || !reflect.DeepEqual(g.BlockedTools, []string{"c"}) {
		t.Fatal(g)
	}
	managed := false
	if g := governance(core.Profile{DirectCallable: &managed, OwnerAgent: "o"}, nil, nil); g.AuthorityBoundary != "managed sub-agent · manager controls wake access; delegated work still runs under this agent policy" {
		t.Fatal(g.AuthorityBoundary)
	}
	for allowList, denyList := range map[string]string{"allowlist+denylist": "both", "allowlist": "allow", "denylist": "deny"} {
		p := core.Profile{}
		if denyList != "deny" {
			p.ToolAllow = []string{"x"}
		}
		if denyList != "allow" {
			p.ToolDeny = []string{"y"}
		}
		if g := governance(p, nil, nil); g.ToolPolicy != allowList || g.ToolAllowCount != len(p.ToolAllow) || g.ToolDenyCount != len(p.ToolDeny) {
			t.Fatal(allowList, g.ToolPolicy)
		}
	}
	g = governance(core.Profile{Slug: "s", MemoryScope: " team ", TrustCeiling: " L2 ", MaxCostMc: 9007199254740993, MaxDailyMc: 4, NoisePolicy: &core.NoisePolicy{DisableMemoryWrites: true, MinNotifySeverity: " info ", MinNotifyIntervalSec: 5}}, nil, nil)
	if g.MemoryPolicy != "scoped:team" || g.MemoryScope != "team" || g.MemoryWrites != "disabled" || g.TrustCeiling != "L2" || g.MaxCostMc != 9007199254740993 || g.MaxDailyMc != 4 || g.NoiseMinNotifySeverity != "info" || g.NoiseMinNotifyIntervalSec != 5 || !g.NoiseDisableMemoryWrites || g.NoiseSilentOnSuccess {
		t.Fatalf("%+v", g)
	}
	g = governance(core.Profile{System: true, NoisePolicy: &core.NoisePolicy{MinNotifySeverity: "info", MinNotifyIntervalSec: 60}}, []PermissionRow{allow}, nil)
	if g.Risk != "system_guardian" || !g.SystemEnforced || !strings.HasSuffix(g.Summary, ", system enforced") || g.AuthorityBoundary != "system guardian · kernel-owned defaults enforce quiet, capped permissions" || !g.NoiseSilentOnSuccess || !g.NoiseDisableMemoryWrites || g.NoiseMinNotifySeverity != "warning" || g.NoiseMinNotifyIntervalSec != 8*3600 || g.MemoryWrites != "disabled" {
		t.Fatalf("a system guardian is held quiet: %+v", g)
	}
	g = governance(core.Profile{System: true, NoisePolicy: &core.NoisePolicy{MinNotifySeverity: "critical", MinNotifyIntervalSec: 99999}}, nil, nil)
	if g.NoiseMinNotifySeverity != "critical" || g.NoiseMinNotifyIntervalSec != 99999 {
		t.Fatal("stricter guardian settings stand", g)
	}
	for severity, rank := range map[string]int{" CRITICAL ": 3, "warning": 2, "warn": 2, "info": 1, "": 1, "loud": 0} {
		if noiseSeverityRank(severity) != rank {
			t.Fatal(severity)
		}
	}
	if g := governance(core.Profile{System: true, NoisePolicy: &core.NoisePolicy{MinNotifySeverity: "loud"}}, nil, nil); g.NoiseMinNotifySeverity != "warning" {
		t.Fatal("an unknown severity is raised to warning", g.NoiseMinNotifySeverity)
	}
}

func TestCapabilities(t *testing.T) {
	ctx := context.Background()
	f := newPermFixture()
	if _, err := f.service().Capabilities(ctx, capRequest(t, `{"ref":"ghost","workdir":"x"}`)); err == nil || err.Error() != "unknown agent: ghost" || f.updates != 0 {
		t.Fatal(err)
	}
	if _, err := f.service().Capabilities(ctx, capRequest(t, `{"ref":"ghost","workdir":3}`)); err == nil || err.Error() != "unknown agent: ghost" {
		t.Fatal("the agent is looked up before the patch decodes", err)
	}
	for _, raw := range []string{`{"ref":"a"}`, `{"ref":"a","unrelated":1}`} {
		if _, err := f.service().Capabilities(ctx, capRequest(t, raw)); err == nil || err.Error() != "args capability field required" || f.updates != 0 {
			t.Fatal(raw, err)
		}
	}
	for _, field := range []string{`"trust_ceiling":3`, `"tool_allow":"x"`, `"tool_deny":[1]`, `"noise_policy":"loud"`, `"config_overrides":{"A":1}`, `"memory_scope":false`, `"workdir":{}`, `"max_cost_mc":"many"`, `"max_daily_mc":1.5`} {
		if _, err := f.service().Capabilities(ctx, capRequest(t, `{"ref":"a",`+field+`}`)); err == nil || !strings.HasPrefix(err.Error(), "json: cannot unmarshal") || f.updates != 0 {
			t.Fatal(field, err)
		}
	}
	if _, err := f.service().Capabilities(ctx, capRequest(t, `{"ref":"a","trust_ceiling":"L1","max_cost_mc":"many"}`)); err == nil || f.updates != 0 {
		t.Fatal("one bad field rejects the whole patch", err)
	}
	f.profiles["orphan"] = core.Profile{Slug: "orphan", OwnerAgent: "gone"}
	if _, err := f.service().Capabilities(ctx, capRequest(t, `{"ref":"orphan","workdir":"x"}`)); err == nil || !strings.Contains(err.Error(), "retired") || f.updates != 0 {
		t.Fatal("the hierarchy is validated before the update", err)
	}
	f.updateErr = errors.New("roster: workdir must be a relative path")
	if _, err := f.service().Capabilities(ctx, capRequest(t, `{"ref":"a","workdir":"../x"}`)); !errors.Is(err, f.updateErr) || f.updates != 1 {
		t.Fatal(err)
	}
	f.updateErr, f.updateMiss = nil, true
	if _, err := f.service().Capabilities(ctx, capRequest(t, `{"ref":"a","workdir":"x"}`)); err == nil || err.Error() != "unknown agent: a" {
		t.Fatal("an agent removed meanwhile is unknown", err)
	}
	f.updateMiss = false
	f.tools["alpha"] = permTool{name: "alpha"}
	f.profiles["a"] = core.Profile{Slug: "a", Enabled: true, ToolDeny: []string{"old"}}
	out, err := f.service().Capabilities(ctx, capRequest(t, `{"ref":"a","trust_ceiling":" L1 ","tool_allow":["alpha"],"tool_deny":null,"noise_policy":{"silent_on_success":true},"config_overrides":{"AGEZT_X":"1"},"memory_scope":"m","workdir":"w","max_cost_mc":9007199254740993,"max_daily_mc":2}`))
	if err != nil {
		t.Fatal(err)
	}
	got := f.profiles["a"]
	if got.TrustCeiling != " L1 " || !reflect.DeepEqual(got.ToolAllow, []string{"alpha"}) || got.ToolDeny != nil || got.NoisePolicy == nil || !got.NoisePolicy.SilentOnSuccess || got.ConfigOverrides["AGEZT_X"] != "1" || got.MemoryScope != "m" || got.Workdir != "w" || got.MaxCostMc != 9007199254740993 || got.MaxDailyMc != 2 {
		t.Fatalf("every sent field is patched as sent: %+v", got)
	}
	if out.Profile.Slug != "a" || out.Slug != "a" || out.TrustCeiling != "L1" || out.Count != 1 || out.Permissions[0].Status != "allowed" || f.decided[len(f.decided)-1] != "alpha@L1" {
		t.Fatalf("the output pictures the patched agent: %+v", out)
	}
	raw, _ := json.Marshal(out)
	if !strings.HasPrefix(string(raw), `{"profile":{`) || !strings.Contains(string(raw), `"wake_access":{`) || !strings.Contains(string(raw), `"governance":{`) {
		t.Fatal(string(raw))
	}
	if _, err := f.service().Capabilities(ctx, capRequest(t, `{"ref":"a","config_overrides":{},"trust_ceiling":null,"max_cost_mc":null}`)); err != nil {
		t.Fatal(err)
	}
	if got := f.profiles["a"]; got.ConfigOverrides != nil || got.TrustCeiling != "" || got.MaxCostMc != 0 || got.Workdir != "w" {
		t.Fatalf("an empty override map clears; null sends the zero value; unsent fields stay: %+v", got)
	}
	if _, err := f.service().Capabilities(ctx, capRequest(t, `{"ref":"a","workdir":null}`)); err != nil || f.profiles["a"].Workdir != "" {
		t.Fatal("a field sent as null is still a patch", err, f.profiles["a"])
	}
	shared := []string{"x"}
	patch := capabilityPatch{toolAllow: &shared}
	var dst core.Profile
	patch.apply(&dst)
	shared[0] = "changed"
	if dst.ToolAllow[0] != "x" {
		t.Fatal("the patch copies its lists")
	}
}

func TestPermissionOperations(t *testing.T) {
	if _, err := PermissionOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	f := newPermFixture()
	f.tools["alpha"] = permTool{name: "alpha"}
	f.entries = []*configcenter.ConfigEntry{configcenter.NewConfigEntry("agent/a/x", "v")}
	ops, err := PermissionOperations(func(context.Context) *PermissionService { return f.service() })
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	read, write := ops[0].Spec(), ops[1].Spec()
	if read.Name != "agent_permissions" || !read.ReadOnly || read.Authz != opapi.PrimaryOnly || read.Tenancy != opapi.Primary || !read.AllowUnknownInput || read.HTTP != (opapi.HTTP{Method: "GET", Path: "/api/agents/permissions"}) || read.Output != reflect.TypeFor[PermissionsOutput]() {
		t.Fatal(read)
	}
	if write.Name != "agent_capabilities" || write.ReadOnly || write.Authz != opapi.PrimaryOnly || write.Tenancy != opapi.Primary || !write.AllowUnknownInput || write.HTTP != (opapi.HTTP{Method: "POST", Path: "/api/agents/capabilities"}) || write.Output != reflect.TypeFor[CapabilitiesOutput]() {
		t.Fatal(write)
	}
	picture, _ := f.service().Permissions(context.Background(), permRequest(t, `{"ref":"a"}`))
	raw, _ := json.Marshal(picture)
	if err := schema.ValidateJSON(read.OutputSchema, raw); err != nil {
		t.Fatal(err, string(raw))
	}
	patched, err := f.service().Capabilities(context.Background(), capRequest(t, `{"ref":"a","workdir":"w"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(patched)
	if err := schema.ValidateJSON(write.OutputSchema, raw); err != nil {
		t.Fatal(err, string(raw))
	}
}
