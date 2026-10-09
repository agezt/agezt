// SPDX-License-Identifier: MIT

package tenants

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/tenant"
)

type fakeRegistry struct {
	exists   map[string]bool
	infos    []tenant.Info
	err      error
	calls    []string
	acquired time.Time
}

func (f *fakeRegistry) Exists(id string) bool {
	f.calls = append(f.calls, "exists:"+id)
	return f.exists[id]
}
func (f *fakeRegistry) Acquire(id string, now time.Time) (*tenant.Tenant, error) {
	f.calls, f.acquired = append(f.calls, "acquire:"+id), now
	if f.err != nil {
		return nil, f.err
	}
	return &tenant.Tenant{ID: id, BaseDir: "/t/" + id, Token: "tok-" + id}, nil
}
func (f *fakeRegistry) Token(id string) (string, error) {
	f.calls = append(f.calls, "token:"+id)
	return "tok-" + id, f.err
}
func (f *fakeRegistry) List() ([]tenant.Info, error) {
	f.calls = append(f.calls, "list")
	return f.infos, f.err
}
func (f *fakeRegistry) Release(id string) (bool, error) {
	f.calls = append(f.calls, "release:"+id)
	return true, f.err
}
func (f *fakeRegistry) Remove(id string) (bool, error) {
	f.calls = append(f.calls, "remove:"+id)
	return false, f.err
}

func decode[T any](t *testing.T, raw string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(raw, err)
	}
	return v
}

var clock = time.Unix(1_700_000_000, 0)

func TestDisabled(t *testing.T) {
	s := New(nil, nil, time.Now)
	ctx := context.Background()
	for name, call := range map[string]func() error{
		"create":  func() error { _, err := s.Create(ctx, IDRequest{}); return err },
		"token":   func() error { _, err := s.Token(ctx, IDRequest{}); return err },
		"list":    func() error { _, err := s.List(ctx, ListRequest{}); return err },
		"release": func() error { _, err := s.Release(ctx, IDRequest{}); return err },
		"remove":  func() error { _, err := s.Remove(ctx, IDRequest{}); return err },
		"stats":   func() error { _, err := s.Stats(ctx, ListRequest{}); return err },
	} {
		if err := call(); !errors.Is(err, ErrDisabled) || err.Error() != "multi-tenancy is disabled (no tenant registry configured)" {
			t.Fatal("the disabled check comes before the arguments", name, err)
		}
	}
}

func TestIDCodec(t *testing.T) {
	ctx := context.Background()
	for raw, want := range map[string]string{`{}`: "args.id required", `{"id":" "}`: "args.id required", `{"id":3}`: "args.id must be a string", `{"id":null}`: "args.id must be a string"} {
		r := &fakeRegistry{}
		s := New(r, nil, time.Now)
		for _, call := range []func() error{
			func() error { _, err := s.Create(ctx, decode[IDRequest](t, raw)); return err },
			func() error { _, err := s.Token(ctx, decode[IDRequest](t, raw)); return err },
			func() error { _, err := s.Release(ctx, decode[IDRequest](t, raw)); return err },
			func() error { _, err := s.Remove(ctx, decode[IDRequest](t, raw)); return err },
		} {
			if err := call(); err == nil || err.Error() != want {
				t.Fatal(raw, err)
			}
		}
		if r.calls != nil {
			t.Fatal("a bad id never reaches the registry", r.calls)
		}
	}
}

func TestCreateTokenReleaseRemove(t *testing.T) {
	ctx := context.Background()
	r := &fakeRegistry{exists: map[string]bool{"old": true}}
	s := New(r, nil, func() time.Time { return clock })
	out, err := s.Create(ctx, decode[IDRequest](t, `{"id":" acme"}`))
	if err != nil || out != (CreateOutput{ID: " acme", BaseDir: "/t/ acme", Created: true, Token: "tok- acme"}) || !r.acquired.Equal(clock) {
		t.Fatal("the id passes untrimmed and the clock stamps the acquisition", out, err)
	}
	if out, _ := s.Create(ctx, decode[IDRequest](t, `{"id":"old"}`)); out.Created {
		t.Fatal("an existing tenant is not reported created", out)
	}
	if tok, _ := s.Token(ctx, decode[IDRequest](t, `{"id":"acme"}`)); tok != (TokenOutput{ID: "acme", Token: "tok-acme"}) {
		t.Fatal(tok)
	}
	if rel, _ := s.Release(ctx, decode[IDRequest](t, `{"id":"acme"}`)); rel != (ReleaseOutput{Released: true}) {
		t.Fatal(rel)
	}
	if rem, _ := s.Remove(ctx, decode[IDRequest](t, `{"id":"acme"}`)); rem != (RemoveOutput{}) {
		t.Fatal(rem)
	}
	if !reflect.DeepEqual(r.calls, []string{"exists: acme", "acquire: acme", "exists:old", "acquire:old", "token:acme", "release:acme", "remove:acme"}) {
		t.Fatal(r.calls)
	}
	boom := errors.New("invalid tenant id")
	failing := New(&fakeRegistry{err: boom}, nil, time.Now)
	for _, call := range []func() error{
		func() error { _, err := failing.Create(ctx, decode[IDRequest](t, `{"id":"x"}`)); return err },
		func() error { _, err := failing.Token(ctx, decode[IDRequest](t, `{"id":"x"}`)); return err },
		func() error { _, err := failing.Release(ctx, decode[IDRequest](t, `{"id":"x"}`)); return err },
		func() error { _, err := failing.Remove(ctx, decode[IDRequest](t, `{"id":"x"}`)); return err },
		func() error { _, err := failing.List(ctx, ListRequest{}); return err },
		func() error { _, err := failing.Stats(ctx, ListRequest{}); return err },
	} {
		if err := call(); !errors.Is(err, boom) {
			t.Fatal(err)
		}
	}
}

func TestList(t *testing.T) {
	r := &fakeRegistry{infos: []tenant.Info{{ID: "a", BaseDir: "/t/a", Open: true}, {ID: "b", BaseDir: "/t/b"}}}
	out, err := New(r, nil, time.Now).List(context.Background(), ListRequest{})
	if err != nil || !reflect.DeepEqual(out, ListOutput{Tenants: []Row{{ID: "a", BaseDir: "/t/a", Open: true}, {ID: "b", BaseDir: "/t/b"}}, Count: 2}) {
		t.Fatal(out, err)
	}
	empty, _ := New(&fakeRegistry{}, nil, time.Now).List(context.Background(), ListRequest{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"tenants":[],"count":0}` {
		t.Fatal(string(raw))
	}
}

func TestStats(t *testing.T) {
	r := &fakeRegistry{infos: []tenant.Info{{ID: "busy", Open: true}, {ID: "closed"}, {ID: "broken"}, {ID: "unreadable"}, {ID: "unreadable-open", Open: true}}}
	activity := func(id string) (func() ([]Run, error), error) {
		switch id {
		case "broken":
			return nil, errors.New("cannot open")
		case "unreadable", "unreadable-open":
			return func() ([]Run, error) { return nil, errors.New("journal unreadable") }, nil
		case "busy":
			return func() ([]Run, error) {
				return []Run{
					{SpentMicrocents: 10, StartedUnixMS: 100, CompletedUnixMS: 900, Completed: true},
					{SpentMicrocents: 5, StartedUnixMS: 200, FailedUnixMS: 1_500, Failed: true, Completed: false},
					{SpentMicrocents: 1, StartedUnixMS: 1_200},
					{StartedUnixMS: 50, CompletedUnixMS: 60, Completed: true, Failed: true},
				}, nil
			}, nil
		}
		return func() ([]Run, error) { return nil, nil }, nil
	}
	out, err := New(r, activity, time.Now).Stats(context.Background(), ListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if string(raw) != `{"tenants":[{"id":"busy","runs":4,"completed":2,"failed":1,"active":1,"spent_microcents":16,"last_activity_unix_ms":1500},{"id":"closed","runs":0,"completed":0,"failed":0,"active":0,"spent_microcents":0,"last_activity_unix_ms":0},{"id":"broken","error":"cannot open"},{"id":"unreadable","error":"journal unreadable"},{"id":"unreadable-open","error":"journal unreadable"}],"count":5,"total_runs":4,"total_spent_microcents":16}` {
		t.Fatal(string(raw))
	}
	if !reflect.DeepEqual(r.calls, []string{"list", "release:closed", "release:unreadable"}) {
		t.Fatal("only tenants that were closed are released again, and never one whose kernel did not open", r.calls)
	}
	empty, _ := New(&fakeRegistry{}, activity, time.Now).Stats(context.Background(), ListRequest{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"tenants":[],"count":0,"total_runs":0,"total_spent_microcents":0}` {
		t.Fatal(string(raw))
	}
}

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("nil provider")
	}
	ops, err := Operations(func(context.Context) *Service { return New(&fakeRegistry{}, nil, time.Now) })
	if err != nil || len(ops) != 6 {
		t.Fatal(ops, err)
	}
	for i, w := range []struct {
		name     string
		readOnly bool
		tenancy  opapi.Tenancy
		out      any
	}{
		{"tenant_create", false, opapi.Primary, CreateOutput{}},
		{"tenant_list", true, opapi.Primary, ListOutput{Tenants: []Row{}}},
		{"tenant_release", false, opapi.Primary, ReleaseOutput{}},
		{"tenant_remove", false, opapi.Primary, RemoveOutput{}},
		{"tenant_token", false, opapi.Primary, TokenOutput{}},
		{"tenant_stats", true, opapi.CallerTenant, StatsOutput{Tenants: []StatsRow{{ID: "a", Error: new("x")}, {ID: "b", Runs: new(1), Completed: new(1), Failed: new(0), Active: new(0), SpentMicrocents: new(int64(2)), LastActivityUnixMS: new(int64(3))}}}},
	} {
		spec := ops[i].Spec()
		if spec.Name != w.name || spec.ReadOnly != w.readOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != w.tenancy || !spec.AllowUnknownInput || spec.HTTP != (opapi.HTTP{}) || spec.Output != reflect.TypeOf(w.out) {
			t.Fatal(spec)
		}
		raw, _ := json.Marshal(w.out)
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatal(w.name, err, string(raw))
		}
	}
}
