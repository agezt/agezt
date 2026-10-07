// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
)

func TestForgeLifecycleNativeTenantDenialBeforeAnyMutation(t *testing.T) {
	provider := mock.New()
	k, server, _, dir := startPair(t, provider)
	registry := withTenants(t, server, dir)
	client := tenantClient(t, dir, mustTenant(t, registry, "acme"))
	head, hash := k.Journal().Head()
	before := k.ToolForge().Count()
	for _, cmd := range []string{controlplane.CmdToolforgeDraft, controlplane.CmdToolforgeEdit, controlplane.CmdToolforgeTest, controlplane.CmdToolforgePromote, controlplane.CmdToolforgeQuarantine, controlplane.CmdToolforgeRemove} {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"ref": "missing", "tool": nil, "tenant": "acme"}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatal(cmd, err)
		}
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || before != k.ToolForge().Count() || provider.CallCount() != 0 {
		t.Fatal(head, after, before, k.ToolForge().Count(), provider.CallCount())
	}
}

type forgeInputRunner struct {
	input string
	calls int
}

func (r *forgeInputRunner) RunScript(_ context.Context, _, _, input string) (string, bool, error) {
	r.input = input
	r.calls++
	return "owned output", false, nil
}
func TestForgeLifecycleNativeSampleReachesRunnerExactly(t *testing.T) {
	runner := &forgeInputRunner{}
	_, _, owner, _ := startPairWithConfig(t, runtime.Config{Provider: mock.New(), ScriptRunner: runner})
	ctx := context.Background()
	if _, err := owner.Call(ctx, controlplane.CmdToolforgeDraft, map[string]any{"tool": map[string]any{"name": "owned", "description": "owned fixture", "language": "python", "code": "never executed"}}); err != nil {
		t.Fatal(err)
	}
	for _, sample := range []string{" raw input ö \n", `{"x":1}`, ""} {
		out, err := owner.Call(ctx, controlplane.CmdToolforgeTest, map[string]any{"ref": "owned", "input": sample})
		want := sample
		if want == "" {
			want = "{}"
		}
		if err != nil || runner.input != want || out["output"] != "owned output" || out["ok"] != true {
			t.Fatal(sample, out, err, runner)
		}
	}
	if runner.calls != 3 {
		t.Fatal(runner.calls)
	}
}
