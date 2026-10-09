// SPDX-License-Identifier: MIT

package seats

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/seat"
)

func store(t *testing.T) *seat.Store {
	t.Helper()
	st, err := seat.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func create(t *testing.T, s *Service, raw string) (SeatOutput, error) {
	t.Helper()
	var in CreateRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	return s.Create(context.Background(), in)
}

func encode(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestReaders(t *testing.T) {
	for raw, want := range map[string]string{``: "", `null`: "", `7`: "", `" a "`: "a"} {
		if got := text(json.RawMessage(raw)); got != want {
			t.Fatal(raw, got)
		}
	}
	for raw, want := range map[string]string{
		``: "null", `null`: "null", `7`: "null", `"  "`: "null", `[]`: "[]",
		`[" a ","",7,null,"b"]`: `["a","b"]`, `"x, y,,z"`: `["x"," y","","z"]`,
	} {
		if got := encode(t, list(json.RawMessage(raw))); got != want {
			t.Fatal(raw, got)
		}
	}
}

func TestSeats(t *testing.T) {
	ctx := context.Background()
	s := New(store(t))
	base, err := s.List(ctx, ListRequest{})
	if err != nil || base.Count != len(seat.Builtins()) || base.Count != len(base.Seats) || base.Count == 0 {
		t.Fatal("the built-ins are listed", encode(t, base), err)
	}
	builtin := base.Seats[0].ID
	for raw, want := range map[string]string{
		`{"id":"Bad ID"}`:                        "seat: id must be lowercase letters, digits, and dashes",
		`{"id":7}`:                               "seat: id must be lowercase letters, digits, and dashes",
		`{"id":"` + builtin + `"}`:               "seat: id already in use",
		`{"id":"ops","execution_profile":"ssh"}`: "seat: execution profile must be empty, local, warden, or container",
		`{"id":"ops","restrict_tools":"true"}`:   "args.restrict_tools must be a boolean",
		`{"id":"Bad ID","restrict_tools":null}`:  "args.restrict_tools must be a boolean",
	} {
		if _, err := create(t, s, raw); err == nil || err.Error() != want {
			t.Fatalf("%s: %v, want %q", raw, err, want)
		}
	}
	out, err := create(t, s, `{"id":" OPS ","name":" Ops ","description":" d ","execution_profile":" Warden ","model_chain":" a, b","tools":["shell"," ",7],"tenant":"x"}`)
	if got := encode(t, out); err != nil || got != `{"seat":{"id":"ops","name":"Ops","description":"d","execution_profile":"warden","model_chain":["a","b"],"tools":["shell"],"restrict_tools":true}}` {
		t.Fatal("arguments are trimmed and the store normalises the seat", got, err)
	}
	out, _ = create(t, s, `{"id":"thinker","restrict_tools":true}`)
	if got := encode(t, out); got != `{"seat":{"id":"thinker","name":"thinker","description":"","restrict_tools":true}}` {
		t.Fatal("a pure-reasoning seat", got)
	}
	if _, err := create(t, s, `{"id":"ops"}`); err == nil || err.Error() != "seat: id already in use" {
		t.Fatal(err)
	}
	after, _ := s.List(ctx, ListRequest{})
	if after.Count != base.Count+2 || after.Seats[after.Count-1].ID != "thinker" {
		t.Fatal(encode(t, after))
	}
	for raw, want := range map[string]string{`{}`: "seat_delete requires id", `{"id":"  "}`: "seat_delete requires id", `{"id":7}`: "seat_delete requires id", `{"id":"ghost"}`: "seat: not found", `{"id":"` + builtin + `"}`: "seat: built-in seats cannot be modified"} {
		var in DeleteRequest
		if err := json.Unmarshal([]byte(raw), &in); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Delete(ctx, in); err == nil || err.Error() != want {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	del, err := s.Delete(ctx, DeleteRequest{ID: json.RawMessage(`" OPS "`)})
	if got := encode(t, del); err != nil || got != `{"deleted":"OPS"}` {
		t.Fatal("the trimmed id is echoed as given", got, err)
	}
	if final, _ := s.List(ctx, ListRequest{}); final.Count != base.Count+1 {
		t.Fatal(encode(t, final))
	}
	if empty, _ := New(emptyStore{}).List(ctx, ListRequest{}); encode(t, empty) != `{"seats":[],"count":0}` {
		t.Fatal("no seats is an empty array", encode(t, empty))
	}
}

type emptyStore struct{}

func (emptyStore) List() []seat.Seat                   { return nil }
func (emptyStore) Create(seat.Seat) (seat.Seat, error) { return seat.Seat{}, nil }
func (emptyStore) Delete(string) error                 { return nil }

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("provider is required")
	}
	ops, err := Operations(func(context.Context) *Service { return nil })
	if err != nil || len(ops) != 3 {
		t.Fatal(len(ops), err)
	}
	for i, want := range []struct {
		name          string
		read          bool
		input, output reflect.Type
		props         string
	}{
		{"seat_list", true, reflect.TypeFor[ListRequest](), reflect.TypeFor[ListOutput](), ""},
		{"seat_create", false, reflect.TypeFor[CreateRequest](), reflect.TypeFor[SeatOutput](), `"id":{},"name":{},"description":{},"execution_profile":{},"model_chain":{},"tools":{},"restrict_tools":{}`},
		{"seat_delete", false, reflect.TypeFor[DeleteRequest](), reflect.TypeFor[DeleteOutput](), `"id":{}`},
	} {
		s := ops[i].Spec()
		out, err := schema.FromType(want.output, false)
		if err != nil {
			t.Fatal(err)
		}
		if s.Name != want.name || s.ReadOnly != want.read || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamNone || !s.AllowUnknownInput || s.HTTP.Method != "" || s.HTTP.Path != "" || s.Input != want.input || s.Output != want.output || string(s.OutputSchema) != string(out) {
			t.Fatalf("%s spec: %+v", want.name, s)
		}
		if want.props != "" && !strings.Contains(string(s.InputSchema), `"properties":{`+want.props+`}`) {
			t.Fatal(want.name, string(s.InputSchema))
		}
		for _, in := range []string{`{}`, `{"id":7,"tools":"a,b","restrict_tools":null,"tenant":"t"}`} {
			if err := schema.ValidateJSON(s.InputSchema, json.RawMessage(in)); err != nil {
				t.Fatal(want.name, in, err)
			}
		}
	}
}
