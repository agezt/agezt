// SPDX-License-Identifier: MIT

package edict

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	core "github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type fakeWriteEngine struct {
	rules     []core.HardDenyRule
	policy    core.AskPolicy
	levels    map[core.Capability]core.TrustLevel
	addErr    error
	removeErr error
	removed   bool
	calls     []string
}

func (f *fakeWriteEngine) HardDenyRules() []core.HardDenyRule { return f.rules }
func (f *fakeWriteEngine) AskPolicy() core.AskPolicy          { return f.policy }
func (f *fakeWriteEngine) Level(c core.Capability) (core.TrustLevel, bool) {
	l, ok := f.levels[c]
	return l, ok
}
func (f *fakeWriteEngine) SetLevel(c core.Capability, l core.TrustLevel) {
	f.calls = append(f.calls, "level:"+string(c)+"="+l.String())
}
func (f *fakeWriteEngine) SetAskPolicy(p core.AskPolicy) {
	f.calls = append(f.calls, "mode:"+p.String())
}
func (f *fakeWriteEngine) AddHardDeny(r core.HardDenyRule) (core.HardDenyRule, error) {
	f.calls = append(f.calls, "add:"+r.Substring)
	if f.addErr != nil {
		return core.HardDenyRule{}, f.addErr
	}
	r.Name = "runtime[1]"
	f.rules = append(f.rules, r)
	return r, nil
}
func (f *fakeWriteEngine) RemoveHardDeny(name string) (bool, error) {
	f.calls = append(f.calls, "rm:"+name)
	return f.removed, f.removeErr
}

type recorder struct{ specs []event.Spec }

func (r *recorder) publish(s event.Spec) { r.specs = append(r.specs, s) }

func (r *recorder) payload(t *testing.T, i int) string {
	t.Helper()
	if r.specs[i].Subject != "kernel.policy" || r.specs[i].Kind != event.KindPolicyChanged || r.specs[i].Actor != "operator" || r.specs[i].CorrelationID != "" {
		t.Fatalf("policy change envelope %+v", r.specs[i])
	}
	raw, _ := json.Marshal(r.specs[i].Payload)
	return string(raw)
}

func TestDenyAdd(t *testing.T) {
	ctx := context.Background()
	shellCap := string(core.AllCapabilities()[0])
	for raw, want := range map[string]string{
		`{"rule":3}`:                   "args.rule must be a string",
		`{"rule":null}`:                "args.rule must be a string",
		`{"rule":"a;b"}`:               "args.rule must specify exactly one deny rule (no ';' separators)",
		`{"rule":"a;b","tenant":3}`:    "args.rule must specify exactly one deny rule (no ';' separators)",
		`{"rule":"rm -rf","tenant":3}`: "args.tenant must be a string",
	} {
		e, rec := &fakeWriteEngine{}, &recorder{}
		if _, err := NewWrites(e, rec.publish).DenyAdd(ctx, decode[DenyAddRequest](t, raw)); err == nil || err.Error() != want || e.calls != nil || rec.specs != nil {
			t.Fatal(raw, err, e.calls)
		}
	}
	for _, raw := range []string{`{}`, `{"rule":"  "}`} {
		_, parseErr := core.ParseDenyRules(strings.TrimSpace(""))
		e := &fakeWriteEngine{}
		_, err := NewWrites(e, (&recorder{}).publish).DenyAdd(ctx, decode[DenyAddRequest](t, raw))
		if parseErr == nil && err == nil || e.calls != nil {
			t.Fatal("a blank rule never reaches the engine", raw, err, e.calls)
		}
	}
	e, rec := &fakeWriteEngine{rules: []core.HardDenyRule{{Name: "builtin"}}}, &recorder{}
	out, err := NewWrites(e, rec.publish).DenyAdd(ctx, decode[DenyAddRequest](t, `{"rule":"`+shellCap+`:curl evil"}`))
	if err != nil || out.Name != "runtime[1]" || out.Substring != "curl evil" || !reflect.DeepEqual(out.AppliesTo, []string{shellCap}) || out.Count != 2 {
		t.Fatalf("%+v %v", out, err)
	}
	if got := rec.payload(t, 0); got != `{"action":"deny.add","applies_to":["`+shellCap+`"],"count":2,"name":"runtime[1]","substring":"curl evil"}` {
		t.Fatal(got)
	}
	global, _ := NewWrites(&fakeWriteEngine{}, (&recorder{}).publish).DenyAdd(ctx, decode[DenyAddRequest](t, `{"rule":"https://exfil.example"}`))
	if raw, _ := json.Marshal(global); string(raw) != `{"name":"runtime[1]","substring":"https://exfil.example","applies_to":[],"count":1}` {
		t.Fatal("a global rule reports an empty applies_to list", string(raw))
	}
	boom := errors.New("empty substring")
	rec = &recorder{}
	if _, err := NewWrites(&fakeWriteEngine{addErr: boom}, rec.publish).DenyAdd(ctx, decode[DenyAddRequest](t, `{"rule":"x"}`)); !errors.Is(err, boom) || rec.specs != nil {
		t.Fatal("a refused rule journals nothing", err)
	}
}

func TestDenyRemove(t *testing.T) {
	ctx := context.Background()
	for raw, want := range map[string]string{`{}`: "args.name required", `{"name":" "}`: "args.name required", `{"name":3}`: "args.name must be a string", `{"name":"x","tenant":3}`: "args.tenant must be a string"} {
		e := &fakeWriteEngine{}
		if _, err := NewWrites(e, (&recorder{}).publish).DenyRemove(ctx, decode[DenyRemoveRequest](t, raw)); err == nil || err.Error() != want || e.calls != nil {
			t.Fatal(raw, err)
		}
	}
	e, rec := &fakeWriteEngine{rules: []core.HardDenyRule{{Name: "a"}}, removed: true}, &recorder{}
	out, err := NewWrites(e, rec.publish).DenyRemove(ctx, decode[DenyRemoveRequest](t, `{"name":" runtime[1] "}`))
	if err != nil || out != (DenyRemoveOutput{Removed: true, Count: 1}) || e.calls[0] != "rm: runtime[1] " {
		t.Fatal(out, err, e.calls)
	}
	if got := rec.payload(t, 0); got != `{"action":"deny.rm","count":1,"name":" runtime[1] "}` {
		t.Fatal(got)
	}
	rec = &recorder{}
	if out, _ := NewWrites(&fakeWriteEngine{}, rec.publish).DenyRemove(ctx, decode[DenyRemoveRequest](t, `{"name":"ghost"}`)); out != (DenyRemoveOutput{}) || rec.specs != nil {
		t.Fatal("nothing removed, nothing journaled", out, rec.specs)
	}
	boom := errors.New("cannot remove the boot-time floor")
	if _, err := NewWrites(&fakeWriteEngine{removeErr: boom}, rec.publish).DenyRemove(ctx, decode[DenyRemoveRequest](t, `{"name":"builtin"}`)); !errors.Is(err, boom) || rec.specs != nil {
		t.Fatal(err)
	}
}

func TestSetLevel(t *testing.T) {
	ctx := context.Background()
	known := string(core.AllCapabilities()[0])
	level := core.TrustLevel(2).String()
	for raw, want := range map[string]string{
		`{}`:                              "args.capability required",
		`{"capability":3}`:                "args.capability must be a string",
		`{"capability":"nope","level":3}`: "unknown capability nope (see `edict show` for the governed set)",
		`{"capability":"` + known + `","level":3}`:                          "args.level must be a string",
		`{"capability":"` + known + `","level":"` + level + `","tenant":3}`: "args.tenant must be a string",
	} {
		e := &fakeWriteEngine{}
		if _, err := NewWrites(e, (&recorder{}).publish).SetLevel(ctx, decode[SetLevelRequest](t, raw)); err == nil || err.Error() != want || e.calls != nil {
			t.Fatal(raw, err)
		}
	}
	if _, err := NewWrites(&fakeWriteEngine{}, (&recorder{}).publish).SetLevel(ctx, decode[SetLevelRequest](t, `{"capability":"`+known+`","level":"bogus"}`)); err == nil {
		t.Fatal("an unparseable level is refused")
	}
	e, rec := &fakeWriteEngine{}, &recorder{}
	out, err := NewWrites(e, rec.publish).SetLevel(ctx, decode[SetLevelRequest](t, `{"capability":"`+known+`","level":"`+level+`"}`))
	if err != nil || out != (SetLevelOutput{Capability: known, From: "unset", To: level}) || e.calls[0] != "level:"+known+"="+level {
		t.Fatal(out, err, e.calls)
	}
	if got := rec.payload(t, 0); got != `{"action":"level.set","capability":"`+known+`","from":"unset","to":"`+level+`"}` {
		t.Fatal(got)
	}
	prev := &fakeWriteEngine{levels: map[core.Capability]core.TrustLevel{core.Capability(known): core.TrustLevel(4)}}
	if out, _ := NewWrites(prev, (&recorder{}).publish).SetLevel(ctx, decode[SetLevelRequest](t, `{"capability":"`+known+`","level":"`+level+`"}`)); out.From != core.TrustLevel(4).String() {
		t.Fatal(out)
	}
}

func TestSetMode(t *testing.T) {
	ctx := context.Background()
	mode := core.AskPolicy(1).String()
	for raw, want := range map[string]string{`{"mode":3}`: "args.mode must be a string", `{"mode":"` + mode + `","tenant":3}`: "args.tenant must be a string"} {
		e := &fakeWriteEngine{}
		if _, err := NewWrites(e, (&recorder{}).publish).SetMode(ctx, decode[SetModeRequest](t, raw)); err == nil || err.Error() != want || e.calls != nil {
			t.Fatal(raw, err)
		}
	}
	if _, err := NewWrites(&fakeWriteEngine{}, (&recorder{}).publish).SetMode(ctx, decode[SetModeRequest](t, `{"mode":"bogus"}`)); err == nil {
		t.Fatal("an unknown mode is refused")
	}
	e, rec := &fakeWriteEngine{policy: core.AskPolicy(0)}, &recorder{}
	out, err := NewWrites(e, rec.publish).SetMode(ctx, decode[SetModeRequest](t, `{"mode":"`+mode+`"}`))
	if err != nil || out != (SetModeOutput{From: core.AskPolicy(0).String(), To: mode}) || e.calls[0] != "mode:"+mode {
		t.Fatal(out, err, e.calls)
	}
	if got := rec.payload(t, 0); got != `{"action":"mode.set","from":"`+core.AskPolicy(0).String()+`","to":"`+mode+`"}` {
		t.Fatal(got)
	}
}

type editAuth struct{}

func (editAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return opapi.Principal{Kind: opapi.Tenant, Tenant: "acme"}, nil
}

type editRoute struct{}

func (editRoute) Route(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) {
	return ctx, nil
}

type editAudit struct {
	fail   error
	begins []string
}

func (a *editAudit) Begin(_ context.Context, r opapi.AuditRecord) (opapi.AuditSpan, error) {
	if a.fail != nil {
		return nil, a.fail
	}
	a.begins = append(a.begins, r.Operation)
	return editSpan{}, nil
}

type editSpan struct{}

func (editSpan) End(context.Context, error) error { return nil }

func TestWriteOperations(t *testing.T) {
	if _, err := WriteOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	e, rec := &fakeWriteEngine{}, &recorder{}
	ops, err := WriteOperations(func(context.Context) *Writes { return NewWrites(e, rec.publish) })
	if err != nil || len(ops) != 4 {
		t.Fatal(ops, err)
	}
	for i, w := range []struct {
		name, path string
		out        any
	}{
		{"edict_deny_add", "/api/edict/deny_add", DenyAddOutput{AppliesTo: []string{}}},
		{"edict_deny_rm", "/api/edict/deny_rm", DenyRemoveOutput{}},
		{"edict_set_level", "/api/edict/set_level", SetLevelOutput{}},
		{"edict_set_mode", "/api/edict/set_mode", SetModeOutput{}},
	} {
		spec := ops[i].Spec()
		if spec.Name != w.name || spec.ReadOnly || spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant || !spec.AllowUnknownInput || spec.HTTP != (opapi.HTTP{Method: "POST", Path: w.path}) || spec.Output != reflect.TypeOf(w.out) {
			t.Fatal(spec)
		}
		raw, _ := json.Marshal(w.out)
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatal(w.name, err)
		}
	}
	mode := core.AskPolicy(1).String()
	failing, _ := app.NewDispatcher(ops, app.Dependencies{Auth: editAuth{}, Router: editRoute{}, Audit: &editAudit{fail: errors.New("audit unavailable")}})
	if _, err := failing.Dispatch(context.Background(), opapi.Caller{Tenant: "acme"}, "edict_set_mode", json.RawMessage(`{"mode":"`+mode+`"}`), nil); err == nil || err.Error() != "audit unavailable" || e.calls != nil || rec.specs != nil {
		t.Fatal("failed audit admission must block the policy change", err, e.calls)
	}
	audit := &editAudit{}
	d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: editAuth{}, Router: editRoute{}, Audit: audit})
	if _, err := d.Dispatch(context.Background(), opapi.Caller{Tenant: "acme"}, "edict_set_mode", json.RawMessage(`{"mode":"`+mode+`"}`), nil); err != nil || len(audit.begins) != 1 || len(rec.specs) != 1 {
		t.Fatal(err, audit.begins, rec.specs)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{Tenant: "acme"}, "edict_set_mode", json.RawMessage(`{"mode":"`+mode+`"}`), nil); !errors.Is(err, context.Canceled) || len(audit.begins) != 1 || len(e.calls) != 1 {
		t.Fatal("a canceled write neither audits nor changes policy", err)
	}
}
