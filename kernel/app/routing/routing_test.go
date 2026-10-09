// SPDX-License-Identifier: MIT

package routing

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
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
	governed  bool
	tasks     map[string][]string
	named     map[string][]string
	def       string
	agents    []Agent
	events    []*event.Event
	store     *fakeStore
	known     map[string]bool
	setTasks  []map[string][]string
	setNamed  []string
	storeOpen int
}

func (f *fake) service() *Service {
	return New(Ports{
		TaskChains: func() (map[string][]string, bool) {
			if !f.governed {
				return nil, false
			}
			return f.tasks, true
		},
		SetTaskChains: func(c map[string][]string) {
			if f.governed {
				f.setTasks = append(f.setTasks, c)
			}
		},
		Named: func() (map[string][]string, string, bool) {
			if !f.governed {
				return nil, "", false
			}
			return f.named, f.def, true
		},
		SetNamed: func(c map[string][]string, def string) {
			if f.governed {
				f.setNamed = append(f.setNamed, encodeChains(c)+"|"+def)
			}
		},
		Agents: func() []Agent { return f.agents },
		Journal: func(visit func(*event.Event)) {
			for _, e := range f.events {
				visit(e)
			}
		},
		Store: func() Store {
			f.storeOpen++
			return f.store
		},
		Models: func() func(string) bool { return func(m string) bool { return f.known[m] } },
	})
}

func raw(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func fallback(t *testing.T, ts int64, payload any) *event.Event {
	t.Helper()
	return &event.Event{Kind: event.KindProviderFallback, TSUnixMS: ts, Payload: json.RawMessage(raw(t, payload))}
}

func TestRouting(t *testing.T) {
	ctx := context.Background()
	f := &fake{}
	out, err := f.service().Routing(ctx, RoutingRequest{})
	if got := raw(t, out); err != nil || got != `{"task_types":["chat","plan","code","verify","summarize","salience","distill","forge","shadow-eval","delegate"],"chains":{},"activity":{}}` {
		t.Fatal("no governor has no chains", got, err)
	}
	long := strings.Repeat("x", 200)
	f.governed = true
	f.tasks = map[string][]string{"chat": {"a", "@fast"}, "odd": nil}
	f.events = []*event.Event{
		fallback(t, 1, map[string]any{"failed_model": "a", "next_model": "b", "reason": "503", "scope": "model-chain", "task_type": "chat"}),
		{Kind: event.KindBudgetCeilingSet, TSUnixMS: 2, Payload: json.RawMessage(`{"scope":"model-chain","task_type":"chat"}`)},
		fallback(t, 3, map[string]any{"failed": "openai", "next": "mock", "reason": "429"}),
		fallback(t, 4, map[string]any{"failed_model": "x", "scope": "model-chain", "task_type": 7}),
		fallback(t, 5, map[string]any{"failed_model": "b", "next_model": "c", "scope": "model-chain", "task_type": "chat"}),
		fallback(t, 6, map[string]any{"failed_model": "p", "next_model": "q", "reason": long, "scope": "model-chain"}),
		{Kind: event.KindProviderFallback, TSUnixMS: 7, Payload: json.RawMessage(`not json`)},
	}
	out, err = f.service().Routing(ctx, RoutingRequest{})
	want := `{"task_types":["chat","plan","code","verify","summarize","salience","distill","forge","shadow-eval","delegate"],"chains":{"chat":["a","@fast"],"odd":[]},"activity":{"(unknown)":{"fallbacks":1,"last_failed":"p","last_next":"q","last_reason":"` + strings.Repeat("x", 160) + `…","last_ms":6},"chat":{"fallbacks":2,"last_failed":"b","last_next":"c","last_reason":"503","last_ms":5}}}`
	if got := raw(t, out); err != nil || got != want {
		t.Fatalf("activity folds model-chain fallbacks by task:\n%s\n%s", got, want)
	}
}

func TestDecodeAndEncodeChains(t *testing.T) {
	for _, in := range []string{`null`, `[]`, `"a=b"`, `7`, `true`} {
		if _, err := decodeChains(json.RawMessage(in)); err == nil || err.Error() != "chains must be an object {task: [models]}" {
			t.Fatal(in, err)
		}
	}
	for in, want := range map[string]string{
		`{" code ":"m"}`: `chains["code"] must be an array of model ids`,
		`{"a":{}}`:       `chains["a"] must be an array of model ids`,
		`{"a":null}`:     `chains["a"] must be an array of model ids`,
	} {
		if _, err := decodeChains(json.RawMessage(in)); err == nil || err.Error() != want {
			t.Fatal(in, err)
		}
	}
	got, err := decodeChains(json.RawMessage(`{" code ":[" m1 ",7,"",null,"m2"," "],"  ":["x"],"empty":[],"blank":[" ",3],"z":["@fast"]}`))
	if err != nil || !reflect.DeepEqual(got, map[string][]string{"code": {"m1", "m2"}, "z": {"@fast"}}) {
		t.Fatal(got, err)
	}
	if spec := encodeChains(map[string][]string{"z": {"a"}, "b": {"c", "d"}, "empty": {}}); spec != "b=c,d;z=a" {
		t.Fatal(spec)
	}
	if encodeChains(nil) != "" {
		t.Fatal("no chains encode to nothing")
	}
}

func TestSetRouting(t *testing.T) {
	ctx := context.Background()
	f := &fake{store: &fakeStore{}, known: map[string]bool{"m1": true}}
	if _, err := f.service().SetRouting(ctx, RoutingSetRequest{}); err == nil || err.Error() != "args.chains required (object {task: [models]})" {
		t.Fatal(err)
	}
	if _, err := f.service().SetRouting(ctx, RoutingSetRequest{Chains: json.RawMessage(`{"a":"m"}`)}); err == nil || err.Error() != `chains["a"] must be an array of model ids` {
		t.Fatal(err)
	}
	if f.storeOpen != 0 {
		t.Fatal("a refused argument never opens the store")
	}
	f.store.loadErr = errors.New("bad json")
	if _, err := f.service().SetRouting(ctx, RoutingSetRequest{Chains: json.RawMessage(`{}`)}); err == nil || err.Error() != "load config: bad json" {
		t.Fatal(err)
	}
	f.store = &fakeStore{saveErr: errors.New("disk full")}
	if _, err := f.service().SetRouting(ctx, RoutingSetRequest{Chains: json.RawMessage(`{"code":["m1"]}`)}); err == nil || err.Error() != "save config: disk full" {
		t.Fatal(err)
	}
	if len(f.setTasks) != 0 {
		t.Fatal("a failed save never applies")
	}
	f.governed, f.store = true, &fakeStore{}
	out, err := f.service().SetRouting(ctx, RoutingSetRequest{Chains: json.RawMessage(`{"plan":["zz","m1","zz"],"code":["m1","aa"]}`)})
	if got := raw(t, out); err != nil || got != `{"saved":true,"applied":"live","task_count":2,"unknown_models":["aa","zz"]}` {
		t.Fatal(got, err)
	}
	if got := strings.Join(f.store.ops, "|"); got != "load|set AGEZT_TASK_MODEL_CHAINS=code=m1,aa;plan=zz,m1,zz|save" {
		t.Fatal(got)
	}
	if len(f.setTasks) != 1 || !reflect.DeepEqual(f.setTasks[0], map[string][]string{"plan": {"zz", "m1", "zz"}, "code": {"m1", "aa"}}) {
		t.Fatal("applied live", f.setTasks)
	}
	f.store = &fakeStore{}
	out, err = f.service().SetRouting(ctx, RoutingSetRequest{Chains: json.RawMessage(`{"code":[" "]}`)})
	if got := raw(t, out); err != nil || got != `{"saved":true,"applied":"live","task_count":0}` {
		t.Fatal("clearing omits unknown models", got, err)
	}
	if got := strings.Join(f.store.ops, "|"); got != "load|remove AGEZT_TASK_MODEL_CHAINS|save" {
		t.Fatal(got)
	}
	f.governed, f.store, f.setTasks = false, &fakeStore{}, nil
	out, err = f.service().SetRouting(ctx, RoutingSetRequest{Chains: json.RawMessage(`{"code":["m1"]}`)})
	if got := raw(t, out); err != nil || got != `{"saved":true,"applied":"live","task_count":1}` || len(f.store.ops) != 3 {
		t.Fatal("without a governor the chains still persist", got, err, f.store.ops)
	}
}

func TestChains(t *testing.T) {
	ctx := context.Background()
	f := &fake{agents: []Agent{{Slug: "a", Model: "@fast"}}}
	out, err := f.service().Chains(ctx, ChainsRequest{})
	if got := raw(t, out); err != nil || got != `{"chains":{},"default":"","usage":{"__dangling__":["fast"]}}` {
		t.Fatal("without a governor every reference dangles", got, err)
	}
	f.governed = true
	f.named = map[string][]string{"fast": {"m1", "m2"}, "big": {"m3"}, "idle": nil, "spare": {"m4"}}
	f.def = "idle"
	f.tasks = map[string][]string{"code": {"@fast", "@gone", "real"}, "plan": {"@fast"}, "chat": {"@"}}
	f.agents = []Agent{
		{Slug: "zed", Model: "@fast", Fallbacks: []string{"@big", "@fast", "@missing", "@"}},
		{Slug: "amy", Model: "m1", Fallbacks: []string{"@fast"}},
		{Slug: "bob", Model: "@gone"},
	}
	out, err = f.service().Chains(ctx, ChainsRequest{})
	want := `{"chains":{"big":["m3"],"fast":["m1","m2"],"idle":[],"spare":["m4"]},"default":"idle","usage":{"__dangling__":["gone","missing"],"big":{"agents":["zed"]},"fast":{"agents":["amy","zed"],"tasks":["code","plan"]},"idle":{"default":true}}}`
	if got := raw(t, out); err != nil || got != want {
		t.Fatalf("usage:\n%s\n%s", got, want)
	}
	f.def = "fast"
	out, _ = f.service().Chains(ctx, ChainsRequest{})
	if got := raw(t, out.Usage["fast"]); got != `{"agents":["amy","zed"],"tasks":["code","plan"],"default":true}` {
		t.Fatal("a referenced default is marked", got)
	}
	if _, listed := out.Usage["idle"]; listed {
		t.Fatal("an unreferenced chain that is not the default is not listed")
	}
	f.def = "gone"
	out, _ = f.service().Chains(ctx, ChainsRequest{})
	if _, listed := out.Usage["gone"]; listed || raw(t, out.Usage["__dangling__"]) != `["gone","missing"]` {
		t.Fatal("an undefined default is never listed as a chain", out.Usage)
	}
}

func TestSetChains(t *testing.T) {
	ctx := context.Background()
	f := &fake{store: &fakeStore{}, known: map[string]bool{"m1": true}}
	set := func(in string) (ChainsSetOutput, error) {
		t.Helper()
		var req ChainsSetRequest
		if err := json.Unmarshal([]byte(in), &req); err != nil {
			t.Fatal(err)
		}
		return f.service().SetChains(ctx, req)
	}
	for in, want := range map[string]string{
		`{}`:                                      "args.chains required (object {name: [models]})",
		`{"chains":null}`:                         "chains must be an object {task: [models]}",
		`{"chains":{"a":"m"}}`:                    `chains["a"] must be an array of model ids`,
		`{"chains":{"Fast":["m"]}}`:               `invalid chain name "Fast" (use lower-case letters, digits, dashes)`,
		`{"chains":{"-x":["m"]}}`:                 `invalid chain name "-x" (use lower-case letters, digits, dashes)`,
		`{"chains":{"a_b":["m"]}}`:                `invalid chain name "a_b" (use lower-case letters, digits, dashes)`,
		`{"chains":{"fast":["m","@big"]}}`:        `chain "fast" model "@big": chains may not reference other chains`,
		`{"chains":{"fast":["m"]},"default":"x"}`: `default chain "x" is not one of the defined chains`,
		`{"chains":{},"default":" fast "}`:        `default chain "fast" is not one of the defined chains`,
	} {
		if _, err := set(in); err == nil || err.Error() != want {
			t.Fatalf("%s: %v, want %q", in, err, want)
		}
	}
	if f.storeOpen != 0 {
		t.Fatal("a refused argument never opens the store")
	}
	f.store.loadErr = errors.New("bad json")
	if _, err := set(`{"chains":{}}`); err == nil || err.Error() != "load config: bad json" {
		t.Fatal(err)
	}
	f.store = &fakeStore{saveErr: errors.New("disk full")}
	if _, err := set(`{"chains":{}}`); err == nil || err.Error() != "save config: disk full" {
		t.Fatal(err)
	}
	f.governed, f.store = true, &fakeStore{}
	out, err := set(`{"chains":{" fast ":["m1"," zz "],"big":["aa","m1"],"0-x":["m1"]},"default":"  big ","tenant":"x"}`)
	if got := raw(t, out); err != nil || got != `{"saved":true,"applied":"live","chain_count":3,"default":"big","unknown_models":["aa","zz"]}` {
		t.Fatal(got, err)
	}
	if got := strings.Join(f.store.ops, "|"); got != "load|set AGEZT_FALLBACK_CHAINS=0-x=m1;big=aa,m1;fast=m1,zz|set AGEZT_DEFAULT_CHAIN=big|save" {
		t.Fatal(got)
	}
	if len(f.setNamed) != 1 || f.setNamed[0] != "0-x=m1;big=aa,m1;fast=m1,zz|big" {
		t.Fatal("applied live", f.setNamed)
	}
	f.store = &fakeStore{}
	out, err = set(`{"chains":{"fast":["m1"]},"default":7}`)
	if got := raw(t, out); err != nil || got != `{"saved":true,"applied":"live","chain_count":1,"default":""}` {
		t.Fatal("a non-string default reads as none", got, err)
	}
	if got := strings.Join(f.store.ops, "|"); got != "load|set AGEZT_FALLBACK_CHAINS=fast=m1|remove AGEZT_DEFAULT_CHAIN|save" {
		t.Fatal(got)
	}
	f.store = &fakeStore{}
	if _, err := set(`{"chains":{"fast":[]}}`); err != nil || strings.Join(f.store.ops, "|") != "load|remove AGEZT_FALLBACK_CHAINS|remove AGEZT_DEFAULT_CHAIN|save" {
		t.Fatal("clearing removes both", f.store.ops, err)
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
		properties         string
	}{
		{"routing_get", "GET", "/api/routing", true, reflect.TypeFor[RoutingRequest](), reflect.TypeFor[RoutingOutput](), ""},
		{"routing_set", "POST", "/api/routing/set", false, reflect.TypeFor[RoutingSetRequest](), reflect.TypeFor[RoutingSetOutput](), `"chains":{}`},
		{"chains_get", "GET", "/api/chains", true, reflect.TypeFor[ChainsRequest](), reflect.TypeFor[ChainsOutput](), ""},
		{"chains_set", "POST", "/api/chains/set", false, reflect.TypeFor[ChainsSetRequest](), reflect.TypeFor[ChainsSetOutput](), `"chains":{},"default":{}`},
	} {
		s := ops[i].Spec()
		out, err := schema.FromType(want.output, false)
		if err != nil {
			t.Fatal(err)
		}
		if s.Name != want.name || s.ReadOnly != want.read || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamNone || !s.AllowUnknownInput || s.HTTP.Method != want.method || s.HTTP.Path != want.path || s.Input != want.input || s.Output != want.output || string(s.OutputSchema) != string(out) {
			t.Fatalf("%s spec: %+v", want.name, s)
		}
		if want.properties != "" && !strings.Contains(string(s.InputSchema), `"properties":{`+want.properties+`}`) {
			t.Fatalf("%s input schema: %s", want.name, s.InputSchema)
		}
		for _, in := range []string{`{}`, `{"chains":null,"default":7,"tenant":"x"}`} {
			if err := schema.ValidateJSON(s.InputSchema, json.RawMessage(in)); err != nil {
				t.Fatal(want.name, in, err)
			}
		}
	}
}
