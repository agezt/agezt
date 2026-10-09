// SPDX-License-Identifier: MIT

package edict

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	core "github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type fakeEngine struct {
	levels  map[core.Capability]core.TrustLevel
	rules   []core.HardDenyRule
	policy  core.AskPolicy
	outcome core.Outcome
	decided []string
}

func (f *fakeEngine) Levels() map[core.Capability]core.TrustLevel { return f.levels }
func (f *fakeEngine) HardDenyRules() []core.HardDenyRule {
	return append([]core.HardDenyRule(nil), f.rules...)
}
func (f *fakeEngine) AskPolicy() core.AskPolicy { return f.policy }
func (f *fakeEngine) Decide(c core.Capability, input string) core.Outcome {
	f.decided = append(f.decided, string(c)+"|"+input)
	return f.outcome
}

func decode[T any](t *testing.T, raw string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(raw, err)
	}
	return v
}

func engine() *fakeEngine {
	return &fakeEngine{
		levels: map[core.Capability]core.TrustLevel{"shell.exec": core.TrustLevel(0), "net.fetch": core.TrustLevel(4)},
		rules: []core.HardDenyRule{
			{Name: "zz-runtime", Substring: "rm -rf"},
			{Name: "aa-builtin", Substring: "mkfs", AppliesTo: []core.Capability{"shell.exec", "fs.write"}},
		},
		policy: core.AskPolicy(0),
	}
}

func TestShow(t *testing.T) {
	e := engine()
	out, err := New(e).Show(context.Background(), TenantRequest{})
	if err != nil {
		t.Fatal(err)
	}
	want := ShowOutput{
		AskPolicy: e.policy.String(),
		Levels:    map[string]string{"shell.exec": core.TrustLevel(0).String(), "net.fetch": core.TrustLevel(4).String()},
		HardDeny:  []HardDenyRow{{Name: "aa-builtin", Substring: "mkfs", AppliesTo: []string{"shell.exec", "fs.write"}}, {Name: "zz-runtime", Substring: "rm -rf"}},
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("%+v", out)
	}
	raw, _ := json.Marshal(out.HardDeny[1])
	if string(raw) != `{"name":"zz-runtime","substring":"rm -rf","applies_to":null}` {
		t.Fatal("no applies_to means every capability (null)", string(raw))
	}
	empty, _ := New(&fakeEngine{}).Show(context.Background(), TenantRequest{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"ask_policy":"`+core.AskPolicy(0).String()+`","levels":{},"hard_deny":[]}` {
		t.Fatal(string(raw))
	}
}

func TestDenyList(t *testing.T) {
	out, err := New(engine()).DenyList(context.Background(), TenantRequest{})
	if err != nil || len(out.Rules) != 2 || out.Rules[0].Name != "aa-builtin" {
		t.Fatalf("%+v %v", out, err)
	}
	for _, r := range out.Rules {
		if r.Removable != core.IsRuntimeRule(r.Name) {
			t.Fatal("removable follows the runtime-rule naming", r)
		}
	}
	raw, _ := json.Marshal(out.Rules[1])
	if string(raw) != `{"name":"zz-runtime","substring":"rm -rf","applies_to":null,"removable":`+map[bool]string{true: "true", false: "false"}[core.IsRuntimeRule("zz-runtime")]+`}` {
		t.Fatal(string(raw))
	}
	empty, _ := New(&fakeEngine{}).DenyList(context.Background(), TenantRequest{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"rules":[]}` {
		t.Fatal(string(raw))
	}
}

func TestTenantStrict(t *testing.T) {
	ctx := context.Background()
	for _, raw := range []string{`{"tenant":3}`, `{"tenant":null}`, `{"tenant":["a"]}`} {
		if _, err := New(engine()).Show(ctx, decode[TenantRequest](t, raw)); err == nil || err.Error() != "args.tenant must be a string" {
			t.Fatal(raw, err)
		}
		if _, err := New(engine()).DenyList(ctx, decode[TenantRequest](t, raw)); err == nil || err.Error() != "args.tenant must be a string" {
			t.Fatal(raw, err)
		}
	}
	if _, err := New(engine()).Show(ctx, decode[TenantRequest](t, `{"tenant":" acme "}`)); err != nil {
		t.Fatal(err)
	}
}

func TestTest(t *testing.T) {
	ctx := context.Background()
	for raw, want := range map[string]string{
		`{}`:                                     "args.capability required",
		`{"capability":""}`:                      "args.capability required",
		`{"capability":3}`:                       "args.capability required",
		`{"capability":null,"tenant":3}`:         "args.capability required",
		`{"capability":"shell.exec","tenant":3}`: "args.tenant must be a string",
	} {
		e := engine()
		if _, err := New(e).Test(ctx, decode[TestRequest](t, raw)); err == nil || err.Error() != want || e.decided != nil {
			t.Fatal(raw, err, e.decided)
		}
	}
	e := engine()
	e.outcome = core.Outcome{Decision: "deny", Capability: "shell.exec", Level: core.TrustLevel(0), Reason: "hard deny", HardDenied: true, HardDenyRule: "aa-builtin", WouldAsk: true, RequiresApproval: true}
	out, err := New(e).Test(ctx, decode[TestRequest](t, `{"capability":" shell.exec","input":42}`))
	if err != nil || out != (TestOutput{Decision: "deny", Capability: "shell.exec", Level: core.TrustLevel(0).String(), Reason: "hard deny", HardDenied: true, HardDenyRule: "aa-builtin", WouldAsk: true, RequiresApproval: true}) {
		t.Fatalf("%+v %v", out, err)
	}
	if !reflect.DeepEqual(e.decided, []string{" shell.exec|"}) {
		t.Fatal("the capability passes untrimmed and a non-string input is an empty probe", e.decided)
	}
	e.outcome = core.Outcome{Decision: "allow", Capability: "net.fetch", WouldAsk: true}
	out, err = New(e).Test(ctx, decode[TestRequest](t, `{"capability":"net.fetch","input":"https://x"}`))
	if err != nil || e.decided[1] != "net.fetch|https://x" || !out.WouldAsk || out.RequiresApproval || out.HardDenied || out.HardDenyRule != "" {
		t.Fatal("would-ask and requires-approval are reported separately", out, e.decided, err)
	}
}

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("nil provider")
	}
	ops, err := Operations(func(context.Context) *Service { return New(engine()) })
	if err != nil || len(ops) != 3 {
		t.Fatal(ops, err)
	}
	s := New(engine())
	show, _ := s.Show(context.Background(), TenantRequest{})
	list, _ := s.DenyList(context.Background(), TenantRequest{})
	probe, _ := s.Test(context.Background(), decode[TestRequest](t, `{"capability":"x"}`))
	for i, w := range []struct {
		name string
		http opapi.HTTP
		out  any
	}{
		{"edict_show", opapi.HTTP{Method: "GET", Path: "/api/edict_show"}, show},
		{"edict_deny_list", opapi.HTTP{}, list},
		{"edict_test", opapi.HTTP{Method: "GET", Path: "/api/edict/test"}, probe},
	} {
		spec := ops[i].Spec()
		if spec.Name != w.name || !spec.ReadOnly || spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant || !spec.AllowUnknownInput || spec.HTTP != w.http || spec.Output != reflect.TypeOf(w.out) {
			t.Fatal(spec)
		}
		raw, _ := json.Marshal(w.out)
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatal(w.name, err, string(raw))
		}
	}
}
