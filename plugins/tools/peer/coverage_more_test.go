// SPDX-License-Identifier: MIT

package peer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestPeerCoverageDefinition(t *testing.T) {
	tool := NewWithTenants(map[string]Peer{
		"primary":  {Name: "primary", URL: "https://primary.example", Token: "k1"},
		"backup":   {Name: "backup", URL: "https://backup.example", Token: "k2"},
		"regional": {Name: "regional", URL: "https://regional.example", Token: "k3"},
	}, nil)
	def := tool.Definition()
	if def.Name != "remote_run" {
		t.Fatalf("Name = %q", def.Name)
	}
	if def.Effect.Class != agent.EffectCompensable {
		t.Fatalf("Effect.Class = %v, want %v", def.Effect.Class, agent.EffectCompensable)
	}
	if !strings.Contains(def.Description, "backup, primary, regional") {
		t.Fatalf("description should list sorted peers, got %q", def.Description)
	}
	if !strings.Contains(string(def.InputSchema), `"task"`) || !strings.Contains(string(def.InputSchema), `"model"`) {
		t.Fatalf("schema should list task and model, got %s", def.InputSchema)
	}
}

func TestPeerCoveragePeerNamesOf(t *testing.T) {
	cases := map[string]string{
		"":           "",
		"a":          "a",
		"a,b,c":      "a, b, c",
		"z,a,m":      "a, m, z",
		"alpha,beta": "alpha, beta",
	}
	for in, want := range cases {
		peers := map[string]Peer{}
		for _, n := range strings.Split(in, ",") {
			if n == "" {
				continue
			}
			peers[n] = Peer{Name: n}
		}
		if got := peerNamesOf(peers); got != want {
			t.Fatalf("peerNamesOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPeerCoverageParseErrorAndEmptyTask(t *testing.T) {
	tool := NewWithTenants(map[string]Peer{"p": {Name: "p", URL: "https://p"}}, nil)
	if tool == nil {
		t.Fatal("tool should not be nil with at least one peer")
	}

	res, err := tool.Invoke(context.Background(), json.RawMessage(`{`))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "invalid input") {
		t.Fatalf("parse error result = %+v", res)
	}

	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"task":"   "}`))
	if err != nil {
		t.Fatalf("empty task: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "task is required") {
		t.Fatalf("empty task result = %+v", res)
	}
}

func TestPeerCoverageNewWithTenantsNil(t *testing.T) {
	if NewWithTenants(nil, nil) != nil {
		t.Fatal("empty peer set should return nil")
	}
	if NewWithTenants(map[string]Peer{}, nil) != nil {
		t.Fatal("empty peer map should return nil")
	}
	if NewWithTenants(map[string]Peer{"a": {Name: "a"}}, nil) == nil {
		t.Fatal("non-empty peer map should return tool")
	}
}
