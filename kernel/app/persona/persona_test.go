// SPDX-License-Identifier: MIT

package persona

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
	system    string
	store     *fakeStore
	file      []byte
	readErr   error
	saveErr   error
	saved     [][]byte
	setSystem []string
}

func (f *fake) service() *Service {
	return New(Ports{
		System:      func() string { return f.system },
		SetSystem:   func(s string) { f.setSystem = append(f.setSystem, s); f.system = s },
		Store:       func() Store { return f.store },
		ReadPrompts: func() ([]byte, error) { return f.file, f.readErr },
		SavePrompts: func(b []byte) error {
			if f.saveErr != nil {
				return f.saveErr
			}
			f.saved = append(f.saved, b)
			f.file = b
			return nil
		},
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

func TestPersona(t *testing.T) {
	ctx := context.Background()
	f := &fake{store: &fakeStore{}}
	if got, _ := f.service().Get(ctx, GetRequest{}); encode(t, got) != `{"system":"","set":false}` {
		t.Fatal(encode(t, got))
	}
	for raw, want := range map[string]string{``: "args.system required (string; empty to clear)", `null`: "args.system must be a string", `7`: "args.system must be a string", `["x"]`: "args.system must be a string"} {
		if _, err := f.service().Set(ctx, SetRequest{System: json.RawMessage(raw)}); err == nil || err.Error() != want {
			t.Fatal(raw, err)
		}
	}
	if len(f.store.ops) != 0 {
		t.Fatal("a refused argument never opens the store", f.store.ops)
	}
	f.store.loadErr = errors.New("bad json")
	if _, err := f.service().Set(ctx, SetRequest{System: json.RawMessage(`"x"`)}); err == nil || err.Error() != "load config: bad json" {
		t.Fatal(err)
	}
	f.store = &fakeStore{saveErr: errors.New("disk full")}
	if _, err := f.service().Set(ctx, SetRequest{System: json.RawMessage(`"x"`)}); err == nil || err.Error() != "save config: disk full" || len(f.setSystem) != 0 {
		t.Fatal("a failed save never applies", err, f.setSystem)
	}
	f.store = &fakeStore{}
	out, err := f.service().Set(ctx, SetRequest{System: json.RawMessage(`" You are <calm> é "`)})
	if got := encode(t, out); err != nil || got != `{"saved":true,"applied":"live","set":true,"length":19}` {
		t.Fatal("the text is kept as given and its byte length reported", got, err)
	}
	if strings.Join(f.store.ops, "|") != "load|set AGEZT_SYSTEM_PROMPT= You are <calm> é |save" || f.system != " You are <calm> é " {
		t.Fatal(f.store.ops, f.system)
	}
	if got, _ := f.service().Get(ctx, GetRequest{}); encode(t, got) != `{"system":" You are \u003ccalm\u003e é ","set":true}` {
		t.Fatal(encode(t, got))
	}
	f.store = &fakeStore{}
	out, _ = f.service().Set(ctx, SetRequest{System: json.RawMessage(`""`)})
	if encode(t, out) != `{"saved":true,"applied":"live","set":false,"length":0}` || strings.Join(f.store.ops, "|") != "load|remove AGEZT_SYSTEM_PROMPT|save" || f.system != "" {
		t.Fatal("an empty text clears", encode(t, out), f.store.ops)
	}
	f.store = &fakeStore{}
	out, _ = f.service().Set(ctx, SetRequest{System: json.RawMessage(`"  "`)})
	if !out.Set || strings.Join(f.store.ops, "|") != "load|set AGEZT_SYSTEM_PROMPT=  |save" {
		t.Fatal("only the empty string clears", encode(t, out), f.store.ops)
	}
}

func TestPrompts(t *testing.T) {
	ctx := context.Background()
	f := &fake{readErr: errors.New("missing")}
	for _, setup := range []func(){
		func() {},
		func() { f.readErr, f.file = nil, []byte("not json") },
		func() { f.file = []byte(`[{"title":7}]`) },
		func() { f.file = []byte(`null`) },
	} {
		setup()
		if got, _ := f.service().Prompts(ctx, PromptsRequest{}); encode(t, got) != `{"prompts":[]}` {
			t.Fatal("a missing, corrupt or empty library is empty", encode(t, got))
		}
	}
	f.file = []byte(`[{"title":"a","text":"b","extra":1},{"text":"only"}]`)
	if got, _ := f.service().Prompts(ctx, PromptsRequest{}); encode(t, got) != `{"prompts":[{"title":"a","text":"b"},{"title":"","text":"only"}]}` {
		t.Fatal(encode(t, got))
	}
	for raw, want := range map[string]string{``: "args.prompts required (array of {title, text})", `null`: "args.prompts must be an array", `{}`: "args.prompts must be an array", `"x"`: "args.prompts must be an array"} {
		if _, err := f.service().SetPrompts(ctx, PromptsSetRequest{Prompts: json.RawMessage(raw)}); err == nil || err.Error() != want {
			t.Fatal(raw, err)
		}
	}
	if len(f.saved) != 0 {
		t.Fatal("a refused argument never writes")
	}
	long, longText := strings.Repeat("t", MaxPromptTitle+5), strings.Repeat("x", MaxPromptText+5)
	in, _ := json.Marshal([]any{
		map[string]any{"title": " Standup ", "text": " draft it "},
		"skip", 7,
		map[string]any{"title": "", "text": "no title"},
		map[string]any{"title": "no text", "text": "  "},
		map[string]any{"title": 7, "text": "typed"},
		map[string]any{"title": long, "text": longText},
	})
	out, err := f.service().SetPrompts(ctx, PromptsSetRequest{Prompts: in})
	if encode(t, out) != `{"saved":true,"count":2}` || err != nil {
		t.Fatal(encode(t, out), err)
	}
	var saved []Prompt
	if err := json.Unmarshal(f.saved[0], &saved); err != nil || len(saved) != 2 || saved[0] != (Prompt{"Standup", "draft it"}) || saved[1].Title != long[:MaxPromptTitle] || saved[1].Text != longText[:MaxPromptText] {
		t.Fatal("entries are trimmed, filtered and capped", saved, err)
	}
	if !strings.HasPrefix(string(f.saved[0]), "[\n  {\n    \"title\": \"Standup\",") {
		t.Fatal("the file is indented JSON", string(f.saved[0]))
	}
	many := make([]any, 0, MaxPrompts+3)
	for i := 0; i < MaxPrompts+3; i++ {
		many = append(many, map[string]any{"title": "t", "text": "x"})
	}
	in, _ = json.Marshal(many)
	if out, _ := f.service().SetPrompts(ctx, PromptsSetRequest{Prompts: in}); out.Count != MaxPrompts {
		t.Fatal("at most MaxPrompts are kept", out.Count)
	}
	if out, _ := f.service().SetPrompts(ctx, PromptsSetRequest{Prompts: json.RawMessage(`[]`)}); encode(t, out) != `{"saved":true,"count":0}` || string(f.file) != "[]" {
		t.Fatal("an empty library is saved", encode(t, out), string(f.file))
	}
	f.saveErr = errors.New("read-only")
	if _, err := f.service().SetPrompts(ctx, PromptsSetRequest{Prompts: json.RawMessage(`[]`)}); err == nil || err.Error() != "save prompts: read-only" {
		t.Fatal(err)
	}
}

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("provider is required")
	}
	ops, err := Operations(func(context.Context) *Service { return nil })
	if err != nil || len(ops) != 4 {
		t.Fatal(len(ops), err)
	}
	for i, want := range []struct {
		name, method, path string
		read               bool
		input, output      reflect.Type
		prop               string
	}{
		{"persona_get", "GET", "/api/persona", true, reflect.TypeFor[GetRequest](), reflect.TypeFor[GetOutput](), ""},
		{"persona_set", "POST", "/api/persona/set", false, reflect.TypeFor[SetRequest](), reflect.TypeFor[SetOutput](), "system"},
		{"prompts_get", "GET", "/api/prompts", true, reflect.TypeFor[PromptsRequest](), reflect.TypeFor[PromptsOutput](), ""},
		{"prompts_set", "POST", "/api/prompts/set", false, reflect.TypeFor[PromptsSetRequest](), reflect.TypeFor[PromptsSetOutput](), "prompts"},
	} {
		s := ops[i].Spec()
		out, err := schema.FromType(want.output, false)
		if err != nil {
			t.Fatal(err)
		}
		if s.Name != want.name || s.ReadOnly != want.read || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamNone || !s.AllowUnknownInput || s.HTTP.Method != want.method || s.HTTP.Path != want.path || s.Input != want.input || s.Output != want.output || string(s.OutputSchema) != string(out) {
			t.Fatalf("%s spec: %+v", want.name, s)
		}
		if want.prop != "" && !strings.Contains(string(s.InputSchema), `"properties":{"`+want.prop+`":{}}`) {
			t.Fatal(want.name, string(s.InputSchema))
		}
		for _, in := range []string{`{}`, `{"system":7,"prompts":null,"tenant":"t"}`} {
			if err := schema.ValidateJSON(s.InputSchema, json.RawMessage(in)); err != nil {
				t.Fatal(want.name, in, err)
			}
		}
	}
}
