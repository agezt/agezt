// SPDX-License-Identifier: MIT
package config

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

type configProbe struct {
	base, model, ask string
	system           bool
	tools, plugins   int
	route            RoutingReader
	calls            []string
}

func (p *configProbe) BaseDir() string       { p.calls = append(p.calls, "base"); return p.base }
func (p *configProbe) Model() string         { p.calls = append(p.calls, "model"); return p.model }
func (p *configProbe) SystemPromptSet() bool { p.calls = append(p.calls, "system"); return p.system }
func (p *configProbe) ToolCount() int        { p.calls = append(p.calls, "tools"); return p.tools }
func (p *configProbe) PluginCount() int      { p.calls = append(p.calls, "plugins"); return p.plugins }
func (p *configProbe) AskPolicy() string     { p.calls = append(p.calls, "ask"); return p.ask }
func (p *configProbe) Routing() (RoutingReader, bool) {
	p.calls = append(p.calls, "routing")
	return p.route, p.route != nil
}

type routingProbe struct {
	routes, requires map[string][]string
	models           map[string]string
	calls            []string
}

func (p *routingProbe) TaskRoutesView() map[string][]string {
	p.calls = append(p.calls, "routes")
	return p.routes
}
func (p *routingProbe) TaskRouteRequiresView() map[string][]string {
	p.calls = append(p.calls, "requires")
	return p.requires
}
func (p *routingProbe) TaskModelOverridesView() map[string]string {
	p.calls = append(p.calls, "models")
	return p.models
}
func TestConfigTypedShowRequiredRootsRawFieldsPathsPresenceAndReadOrder(t *testing.T) {
	p := &configProbe{base: filepath.Join(" raw ", "owned"), model: " raw model ", ask: " raw ask ", system: true, tools: -2, plugins: 0}
	names := []string{"present", "empty", "absent"}
	seen := []string{}
	present := func(name string) bool {
		seen = append(seen, name)
		p.calls = append(p.calls, "env")
		return name != "absent"
	}
	svc := New(p, names, present)
	names[0] = "changed"
	out, err := svc.Show(context.Background(), ShowInput{})
	if err != nil {
		t.Fatal(err)
	}
	want := Paths{Base: p.base, Journal: filepath.Join(p.base, "journal"), State: filepath.Join(p.base, "state"), Runtime: filepath.Join(p.base, "runtime"), Catalog: filepath.Join(p.base, "catalog"), Vault: filepath.Join(p.base, "vault.json")}
	if out.Paths != want || out.Model != p.model || out.AskPolicy != p.ask || !out.SystemPromptSet || out.ToolCount != -2 || out.PluginCount != 0 || !reflect.DeepEqual(out.Env, map[string]bool{"present": true, "empty": true}) || !reflect.DeepEqual(seen, []string{"present", "empty", "absent"}) || !reflect.DeepEqual(p.calls, []string{"base", "env", "env", "env", "model", "system", "tools", "plugins", "ask", "routing"}) {
		t.Fatal(out, seen, p.calls)
	}
	p.base = "next"
	p.model = ""
	p.ask = ""
	p.system = false
	p.tools = 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	second, err := svc.Show(ctx, ShowInput{})
	if err != nil || second.Model != "" || second.AskPolicy != "" || second.SystemPromptSet || second.ToolCount != 0 || out.Model != " raw model " || second.Paths.Base != "next" {
		t.Fatal(out, second, err)
	}
	out.Env["owned"] = true
	if second.Env["owned"] {
		t.Fatal("env snapshots aliased")
	}
	empty, _ := New(p, nil, func(string) bool { t.Fatal("no listed env"); return false }).Show(context.Background(), ShowInput{})
	raw, _ := json.Marshal(empty)
	var root map[string]json.RawMessage
	json.Unmarshal(raw, &root)
	if string(root["env"]) != "{}" || root["system_prompt"] != nil || root["routing"] != nil || len(root) != 7 {
		t.Fatal(string(raw))
	}
	var paths map[string]json.RawMessage
	json.Unmarshal(root["paths"], &paths)
	if len(paths) != 6 || string(root["system_prompt_set"]) != "false" || string(root["tool_count"]) != "0" || string(root["model"]) != `""` {
		t.Fatal(string(raw))
	}
}
func TestConfigTypedShowRoutingPresenceNilEmptyArraysOrderAndOwnedSnapshots(t *testing.T) {
	variants := []*routingProbe{{}, {routes: map[string][]string{}, requires: map[string][]string{}, models: map[string]string{}}, {routes: map[string][]string{"nil": nil, "empty": {}, " raw ": {"second", "first"}}, requires: map[string][]string{" raw ": {" raw cap ", "first"}}, models: map[string]string{" raw ": " raw model ", "empty": ""}}}
	for i, r := range variants {
		p := &configProbe{route: r}
		svc := New(p, nil, func(string) bool { return false })
		out, err := svc.Show(context.Background(), ShowInput{})
		if err != nil || !reflect.DeepEqual(r.calls, []string{"routes", "requires", "models"}) {
			t.Fatal(out, err, r.calls)
		}
		if i < 2 {
			if out.Routing != nil {
				t.Fatal(out)
			}
			continue
		}
		routing := out.Routing
		raw, _ := json.Marshal(out)
		var root map[string]json.RawMessage
		json.Unmarshal(raw, &root)
		var maps map[string]json.RawMessage
		json.Unmarshal(root["routing"], &maps)
		if len(root) != 8 || len(maps) != 3 {
			t.Fatal(string(raw))
		}
		if !reflect.DeepEqual(routing.Routes["nil"], []string{}) || !reflect.DeepEqual(routing.Routes["empty"], []string{}) || !reflect.DeepEqual(routing.Routes[" raw "], []string{"second", "first"}) || routing.ModelOverrides["empty"] != "" || routing.ModelOverrides[" raw "] != " raw model " || !reflect.DeepEqual(routing.Requires[" raw "], []string{" raw cap ", "first"}) {
			t.Fatal(routing)
		}
		routing.Routes[" raw "][0] = "changed"
		routing.Requires[" raw "][0] = "changed"
		routing.ModelOverrides[" raw "] = "changed"
		routing.Routes["extra"] = nil
		if r.routes[" raw "][0] != "second" || r.requires[" raw "][0] != " raw cap " || r.models[" raw "] != " raw model " || r.routes["extra"] != nil {
			t.Fatal("routing alias", r)
		}
		r.routes[" raw "][0] = "fresh"
		second, _ := svc.Show(context.Background(), ShowInput{})
		if second.Routing.Routes[" raw "][0] != "fresh" {
			t.Fatal(second)
		}
	}
	for _, r := range []*routingProbe{{routes: map[string][]string{"only": nil}}, {requires: map[string][]string{"only": nil}}, {models: map[string]string{"only": ""}}} {
		out, _ := New(&configProbe{route: r}, nil, func(string) bool { return false }).Show(context.Background(), ShowInput{})
		raw, _ := json.Marshal(out.Routing)
		var maps map[string]json.RawMessage
		json.Unmarshal(raw, &maps)
		if len(maps) != 1 {
			t.Fatal(string(raw))
		}
	}
}
