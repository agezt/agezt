// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// stubTool is a minimal agent.Tool for exercising filterTools by name.
type stubTool struct{ name string }

func (s stubTool) Definition() agent.ToolDef { return agent.ToolDef{Name: s.name} }
func (s stubTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	return agent.Result{Output: s.name}, nil
}

func toolSet(names ...string) map[string]agent.Tool {
	m := make(map[string]agent.Tool, len(names))
	for _, n := range names {
		m[n] = stubTool{name: n}
	}
	return m
}

func sortedKeys(m map[string]agent.Tool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestFilterTools(t *testing.T) {
	all := toolSet("read", "write", "shell", "notify")

	cases := []struct {
		name  string
		allow []string
		want  []string
	}{
		{"subset", []string{"read", "notify"}, []string{"notify", "read"}},
		{"single", []string{"read"}, []string{"read"}},
		{"empty allow = no tools", []string{}, []string{}},
		{"nil allow = no tools", nil, []string{}},
		{"unknown name ignored", []string{"read", "ghost"}, []string{"read"}},
		{"all names", []string{"read", "write", "shell", "notify"}, []string{"notify", "read", "shell", "write"}},
		{"duplicate names dedup to one", []string{"read", "read"}, []string{"read"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sortedKeys(filterTools(all, tc.allow))
			if len(got) != len(tc.want) {
				t.Fatalf("filterTools(%v) = %v, want %v", tc.allow, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("filterTools(%v) = %v, want %v", tc.allow, got, tc.want)
				}
			}
		})
	}

	// filterTools must not mutate the source map.
	if len(all) != 4 {
		t.Fatalf("source toolset mutated: got %d tools, want 4", len(all))
	}
}

func TestWithTools_RoundTrip(t *testing.T) {
	// No value set: ok=false means "unrestricted".
	if v, ok := toolsFromCtx(context.Background()); ok {
		t.Fatalf("bare context: ok=true (%v), want ok=false", v)
	}

	// Explicit non-empty allowlist round-trips.
	ctx := WithTools(context.Background(), []string{"read", "notify"})
	v, ok := toolsFromCtx(ctx)
	if !ok {
		t.Fatal("WithTools(non-empty): ok=false, want true")
	}
	if len(v) != 2 || v[0] != "read" || v[1] != "notify" {
		t.Fatalf("toolsFromCtx = %v, want [read notify]", v)
	}

	// Explicit empty allowlist: ok=true (restriction present) with zero names —
	// distinct from "unset". This is the --no-tools case.
	emptyCtx := WithTools(context.Background(), []string{})
	ev, eok := toolsFromCtx(emptyCtx)
	if !eok {
		t.Fatal("WithTools(empty): ok=false, want true (restriction is set)")
	}
	if len(ev) != 0 {
		t.Fatalf("WithTools(empty): got %v, want empty", ev)
	}
	// And filtering by it yields no tools.
	if got := filterTools(toolSet("read", "write"), ev); len(got) != 0 {
		t.Fatalf("filterTools(empty allow) = %v, want no tools", sortedKeys(got))
	}
}
