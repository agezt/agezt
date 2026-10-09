// SPDX-License-Identifier: MIT

package council

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type fakeStore struct {
	loadErr, saveErr error
	ops              []string
}

func (f *fakeStore) Load() error {
	f.ops = append(f.ops, "load")
	return f.loadErr
}
func (f *fakeStore) Set(name, value string) { f.ops = append(f.ops, "set "+name+"="+value) }
func (f *fakeStore) Remove(name string) bool {
	f.ops = append(f.ops, "remove "+name)
	return true
}
func (f *fakeStore) Save() error {
	f.ops = append(f.ops, "save")
	return f.saveErr
}

type fake struct {
	members []Member
	store   *fakeStore
	known   map[string]bool
	applied [][]Member
}

func (f *fake) service() *Service {
	return New(Ports{
		Members:    func() []Member { return f.members },
		SetMembers: func(m []Member) { f.applied = append(f.applied, m); f.members = m },
		Store:      func() Store { return f.store },
		Models:     func() func(string) bool { return func(m string) bool { return f.known[m] } },
	})
}

func encode(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestMembers(t *testing.T) {
	f := &fake{}
	if out, _ := f.service().Members(context.Background(), MembersRequest{}); encode(t, out) != `{"members":[],"count":0}` {
		t.Fatal("no panel is an empty array", encode(t, out))
	}
	f.members = []Member{{"Chair", "m1"}, {"Elder 2", "m2"}}
	if out, _ := f.service().Members(context.Background(), MembersRequest{}); encode(t, out) != `{"members":[{"seat":"Chair","model":"m1"},{"seat":"Elder 2","model":"m2"}],"count":2}` {
		t.Fatal(encode(t, out))
	}
}

func TestSet(t *testing.T) {
	ctx := context.Background()
	f := &fake{store: &fakeStore{}, known: map[string]bool{"known": true}}
	for raw, want := range map[string]string{
		``:                             "args.members required (array of {seat, model})",
		`null`:                         "args.members must be an array",
		`{}`:                           "args.members must be an array",
		`[{"model":"a"},7]`:            "args.members[1] must be an object {seat, model}",
		`[{"model":"a"},{"seat":"s"}]`: "args.members[1].model is required",
		`[{"model":7}]`:                "args.members[0].model is required",
		`[{"model":""},{"model":7}]`:   "args.members[0].model is required",
	} {
		if _, err := f.service().Set(ctx, SetRequest{Members: json.RawMessage(raw)}); err == nil || err.Error() != want {
			t.Fatalf("%s: %v, want %q", raw, err, want)
		}
	}
	if len(f.store.ops) != 0 {
		t.Fatal("a refused argument never opens the store")
	}
	f.store.loadErr = errors.New("bad json")
	if _, err := f.service().Set(ctx, SetRequest{Members: json.RawMessage(`[]`)}); err == nil || err.Error() != "load config: bad json" {
		t.Fatal(err)
	}
	f.store = &fakeStore{saveErr: errors.New("disk full")}
	if _, err := f.service().Set(ctx, SetRequest{Members: json.RawMessage(`[]`)}); err == nil || err.Error() != "save config: disk full" || len(f.applied) != 0 {
		t.Fatal("a failed save never applies", err, f.applied)
	}
	f.store = &fakeStore{}
	out, err := f.service().Set(ctx, SetRequest{Members: json.RawMessage(`[{"seat":" Zeta ","model":" zz "},{"model":"known"},{"seat":"Alpha","model":"aa"},{"seat":7,"model":"zz"},{"model":" "}]`)})
	if got := encode(t, out); err != nil || got != `{"saved":true,"applied":"live","member_count":5,"unknown_models":["","aa","zz"]}` {
		t.Fatal(got, err)
	}
	if got := strings.Join(f.store.ops, "|"); got != "load|set AGEZT_COUNCIL_MEMBERS=known,zz,,aa,zz|save" {
		t.Fatal("models persist in seat order", got)
	}
	if got := encode(t, f.applied[0]); got != `[{"seat":"Elder 1","model":"known"},{"seat":"Elder 2","model":"zz"},{"seat":"Elder 3","model":""},{"seat":"Alpha","model":"aa"},{"seat":"Zeta","model":"zz"}]` {
		t.Fatal("blank seats are named by their position after sorting", got)
	}
	f.store = &fakeStore{}
	out, _ = f.service().Set(ctx, SetRequest{Members: json.RawMessage(`[]`)})
	if encode(t, out) != `{"saved":true,"applied":"live","member_count":0}` || strings.Join(f.store.ops, "|") != "load|remove AGEZT_COUNCIL_MEMBERS|save" || encode(t, f.applied[1]) != `[]` {
		t.Fatal("an empty panel clears", encode(t, out), f.store.ops, f.applied)
	}
	f.store = &fakeStore{}
	if out, _ := f.service().Set(ctx, SetRequest{Members: json.RawMessage(`[{"model":"known"},{"model":"known"}]`)}); encode(t, out) != `{"saved":true,"applied":"live","member_count":2}` {
		t.Fatal("known models are not reported", encode(t, out))
	}
}

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("provider is required")
	}
	ops, err := Operations(func(context.Context) *Service { return nil })
	if err != nil || len(ops) != 3 {
		t.Fatal(len(ops), err)
	}
	for i, want := range []struct {
		name, method, path string
		read               bool
		input, output      reflect.Type
	}{
		{"council_members", "GET", "/api/council/members", true, reflect.TypeFor[MembersRequest](), reflect.TypeFor[MembersOutput]()},
		{"council_set", "POST", "/api/council/set", false, reflect.TypeFor[SetRequest](), reflect.TypeFor[SetOutput]()},
		{"conductor_roles", "", "", true, reflect.TypeFor[RolesRequest](), reflect.TypeFor[RolesOutput]()},
	} {
		s := ops[i].Spec()
		out, err := schema.FromType(want.output, false)
		if err != nil {
			t.Fatal(err)
		}
		if s.Name != want.name || s.ReadOnly != want.read || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamNone || !s.AllowUnknownInput || s.HTTP.Method != want.method || s.HTTP.Path != want.path || s.Input != want.input || s.Output != want.output || string(s.OutputSchema) != string(out) {
			t.Fatalf("%s spec: %+v", want.name, s)
		}
		for _, in := range []string{`{}`, `{"members":null,"tenant":"t"}`} {
			if err := schema.ValidateJSON(s.InputSchema, json.RawMessage(in)); err != nil {
				t.Fatal(want.name, in, err)
			}
		}
	}
	if !strings.Contains(string(ops[1].Spec().InputSchema), `"properties":{"members":{}}`) {
		t.Fatal(string(ops[1].Spec().InputSchema))
	}
}

func TestRoles(t *testing.T) {
	f := &fake{}
	for _, c := range []struct {
		members []Member
		want    string
	}{
		{nil, `{"thinker":"","worker":"","verifier":"","available_models":[],"auto_filled":true}`},
		{[]Member{{"Chair", "a"}}, `{"thinker":"a","worker":"a","verifier":"a","available_models":["a"],"auto_filled":true}`},
		{[]Member{{"Chair", "a"}, {"Scribe", "b"}}, `{"thinker":"a","worker":"b","verifier":"a","available_models":["a","b"],"auto_filled":true}`},
		{[]Member{{"x", "a"}, {"y", "b"}, {"z", "c"}, {"w", "d"}}, `{"thinker":"a","worker":"b","verifier":"c","available_models":["a","b","c","d"],"auto_filled":true}`},
	} {
		f.members = c.members
		if out, _ := f.service().Roles(context.Background(), RolesRequest{}); encode(t, out) != c.want {
			t.Fatal("roles cycle through the panel's models", encode(t, out), c.want)
		}
	}
}
