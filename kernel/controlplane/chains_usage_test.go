// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestChainsGet_Usage (M964): chains_get reports, per chain, which agents and
// task types reference "@name", whether it is the default, and any dangling
// references (to a deleted chain). This is what lets the UI warn before an edit
// or delete breaks something downstream.
func TestChainsGet_Usage(t *testing.T) {
	reg := governor.NewRegistry()
	if err := reg.Register(&governor.ProviderInfo{
		Name: "mock", Provider: mock.New(mock.FinalText("ok")), AuthMode: governor.AuthLocal,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	gov, err := governor.New(governor.Config{
		Registry:               reg,
		DailyCeilingMicrocents: 1_000_000_000,
		FallbackChains:         map[string][]string{"fast": {"m1", "m2"}, "big": {"m3"}},
		DefaultChain:           "big",
		TaskModelChains:        map[string][]string{"code": {"@fast"}, "plan": {"real-model"}},
		Now:                    func() time.Time { return time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("governor.New: %v", err)
	}
	k, _, c, _ := startPair(t, gov)

	// Two agents reference @fast (one as model, one as a fallback); one agent
	// references a deleted chain @gone.
	if _, err := k.AddProfile(roster.Profile{Slug: "researcher", Model: "@fast"}); err != nil {
		t.Fatalf("AddProfile researcher: %v", err)
	}
	if _, err := k.AddProfile(roster.Profile{Slug: "coder", Model: "real-model", Fallbacks: []string{"@fast"}}); err != nil {
		t.Fatalf("AddProfile coder: %v", err)
	}
	if _, err := k.AddProfile(roster.Profile{Slug: "ghost", Model: "@gone"}); err != nil {
		t.Fatalf("AddProfile ghost: %v", err)
	}

	res, err := c.Call(context.Background(), controlplane.CmdChainsGet, nil)
	if err != nil {
		t.Fatalf("chains_get: %v", err)
	}
	usage, ok := res["usage"].(map[string]any)
	if !ok {
		t.Fatalf("chains_get result has no usage map: %v", res["usage"])
	}

	fast, _ := usage["fast"].(map[string]any)
	if fast == nil {
		t.Fatalf("usage missing fast: %v", usage)
	}
	agents := toStrings(fast["agents"])
	if len(agents) != 2 || agents[0] != "coder" || agents[1] != "researcher" {
		t.Errorf("fast.agents = %v, want [coder researcher] (sorted, deduped)", agents)
	}
	tasks := toStrings(fast["tasks"])
	if len(tasks) != 1 || tasks[0] != "code" {
		t.Errorf("fast.tasks = %v, want [code]", tasks)
	}

	// big is the default with no other references — still surfaced.
	big, _ := usage["big"].(map[string]any)
	if big == nil || big["default"] != true {
		t.Errorf("usage.big should be marked default: %v", usage["big"])
	}

	// @gone is dangling (no such chain).
	dangling := toStrings(usage["__dangling__"])
	if len(dangling) != 1 || dangling[0] != "gone" {
		t.Errorf("dangling = %v, want [gone]", dangling)
	}
}

func toStrings(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
