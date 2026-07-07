// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestConfigCoverageDefinition(t *testing.T) {
	tl := New(t.TempDir())
	def := tl.Definition()
	if def.Name != "config" {
		t.Fatalf("Name = %q", def.Name)
	}
	if !strings.Contains(def.Description, "Config Center") {
		t.Fatalf("description should mention Config Center, got %q", def.Description)
	}
	if def.Effect.Class != agent.EffectReversible {
		t.Fatalf("Effect.Class = %v, want %v", def.Effect.Class, agent.EffectReversible)
	}
	schema := string(def.InputSchema)
	for _, want := range []string{`"schema"`, `"get"`, `"set"`, `"register"`, `"unregister"`, `"scope"`} {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema should mention %q, got %s", want, schema)
		}
	}
}

func TestConfigCoverageScopeAndErrf(t *testing.T) {
	cases := []struct {
		raw, def string
		want     string
		err      bool
	}{
		{raw: "", def: "effective", want: "effective"},
		{raw: "  ", def: "effective", want: "effective"},
		{raw: "effective", def: "effective", want: "effective"},
		{raw: "GLOBAL", def: "global", want: "global"},
		{raw: "agent", def: "global", want: "agent"},
		{raw: "unknown", def: "global", err: true},
	}
	for _, tc := range cases {
		got, err := configScope(tc.raw, tc.def)
		if tc.err {
			if err == nil {
				t.Fatalf("configScope(%q,%q) = %v, want error", tc.raw, tc.def, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("configScope(%q,%q) err = %v", tc.raw, tc.def, err)
		}
		if got != tc.want {
			t.Fatalf("configScope(%q,%q) = %q, want %q", tc.raw, tc.def, got, tc.want)
		}
	}

	r := errf("boom %d", 7)
	if !r.IsError || r.Output != "boom 7" {
		t.Fatalf("errf = %+v", r)
	}
}

func TestConfigCoverageInvokeUnknownOpAndParseError(t *testing.T) {
	tl := New(t.TempDir())
	// Parse error path: hard error.
	_, err := tl.Invoke(context.Background(), json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "parse input") {
		t.Fatalf("parse error = %v", err)
	}

	// Unknown op.
	res, err := tl.Invoke(context.Background(), json.RawMessage(`{"op":"delete"}`))
	if err != nil || !res.IsError || !strings.Contains(res.Output, "unknown op") {
		t.Fatalf("unknown op = %+v err %v", res, err)
	}
}

func TestConfigCoverageDoGetValidationBranches(t *testing.T) {
	tl := New(t.TempDir())
	// Register a namespaced field so the agent-scope path is reachable.
	section := map[string]any{
		"id":   "weather",
		"name": "Weather Skill",
		"fields": []map[string]any{
			{"env": "AGEZT_X_WEATHER_API_KEY", "label": "API key", "type": "password", "secret": true},
			{"env": "AGEZT_X_WEATHER_UNITS", "label": "Units", "type": "text"},
		},
	}
	raw, _ := json.Marshal(section)
	if _, err := tl.doRegister(input{Section: raw}); err != nil {
		t.Fatalf("doRegister: %v", err)
	}

	// Missing name.
	res, err := tl.doGet(context.Background(), input{})
	if err != nil || !res.IsError || !strings.Contains(res.Output, "name required") {
		t.Fatalf("missing name = %+v err %v", res, err)
	}

	// Invalid scope (exercised before field lookup).
	res, err = tl.doGet(context.Background(), input{Name: "AGEZT_X_WEATHER_UNITS", Scope: "nope"})
	if err != nil || !res.IsError || !strings.Contains(res.Output, "scope") {
		t.Fatalf("invalid scope = %+v err %v", res, err)
	}

	// Unknown setting name.
	res, err = tl.doGet(context.Background(), input{Name: "AGEZT_NOT_REAL", Scope: "effective"})
	if err != nil || !res.IsError || !strings.Contains(res.Output, "unknown setting") {
		t.Fatalf("unknown setting = %+v err %v", res, err)
	}

	// scope=agent on a non-secret field without a named agent.
	res, err = tl.doGet(context.Background(), input{Name: "AGEZT_X_WEATHER_UNITS", Scope: "agent"})
	if err != nil || !res.IsError || !strings.Contains(res.Output, "requires a named agent") {
		t.Fatalf("agent scope without slug = %+v err %v", res, err)
	}

	// scope=agent on a secret field (always denied for agent overrides).
	res, err = tl.doGet(agent.WithAgent(context.Background(), "tester"), input{Name: "AGEZT_X_WEATHER_API_KEY", Scope: "agent"})
	if err != nil || !res.IsError || !strings.Contains(res.Output, "secret") {
		t.Fatalf("agent scope on secret = %+v err %v", res, err)
	}
}

func TestConfigCoverageDoSetValidationBranches(t *testing.T) {
	tl := New(t.TempDir())
	section := map[string]any{
		"id":   "weather",
		"name": "Weather Skill",
		"fields": []map[string]any{
			{"env": "AGEZT_X_WEATHER_UNITS", "label": "Units", "type": "text"},
		},
	}
	raw, _ := json.Marshal(section)
	if _, err := tl.doRegister(input{Section: raw}); err != nil {
		t.Fatalf("doRegister: %v", err)
	}

	res, err := tl.doSet(context.Background(), input{})
	if err != nil || !res.IsError || !strings.Contains(res.Output, "name required") {
		t.Fatalf("missing name = %+v err %v", res, err)
	}

	res, err = tl.doSet(context.Background(), input{Name: "AGEZT_X_WEATHER_UNITS", Scope: "nope"})
	if err != nil || !res.IsError || !strings.Contains(res.Output, "scope") {
		t.Fatalf("invalid scope = %+v err %v", res, err)
	}

	res, err = tl.doSet(context.Background(), input{Name: "AGEZT_NOT_REAL", Scope: "global"})
	if err != nil || !res.IsError || !strings.Contains(res.Output, "unknown setting") {
		t.Fatalf("unknown setting = %+v err %v", res, err)
	}

	res, err = tl.doSet(context.Background(), input{Name: "AGEZT_X_WEATHER_UNITS", Scope: "agent"})
	if err != nil || !res.IsError || !strings.Contains(res.Output, "named agent run") {
		t.Fatalf("agent scope without slug = %+v err %v", res, err)
	}

	res, err = tl.doRegister(input{})
	if err != nil || !res.IsError || !strings.Contains(res.Output, "section required") {
		t.Fatalf("missing section = %+v err %v", res, err)
	}

	res, err = tl.doRegister(input{Section: json.RawMessage(`{not json}`)})
	if err != nil || !res.IsError || !strings.Contains(res.Output, "decode section") {
		t.Fatalf("bad section = %+v err %v", res, err)
	}

	res, err = tl.doUnregister(input{})
	if err != nil || !res.IsError || !strings.Contains(res.Output, "id required") {
		t.Fatalf("missing id = %+v err %v", res, err)
	}
}
