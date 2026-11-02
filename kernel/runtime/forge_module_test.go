// SPDX-License-Identifier: MIT

package runtime_test

// The daemon's forge verbs: the FACE it keeps and the BODY it gave away.
//
// The daemon used to hold the forge's whole lifecycle — the catalogue AND the
// operations over it, the sandbox test, the operator's queue and the bridge
// that folds the promoted scripts into a run's tool map. It now holds the
// extensions module's face and asks the module. These tests pin that with a
// fake forge: if the daemon grew the lifecycle back, the fake would record
// nothing and the daemon's answer would be its own.

import (
	"context"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/approvalapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// stubForgeStore is the forge's store port, empty on purpose: the fake
// service answers for the module and never reaches the store.
type stubForgeStore struct{ toolforgeapi.HostStore }

// recordingForge is a fake extensions module: it records what the daemon asks
// and answers with a row of its own.
//
// The embedded interface is nil on purpose: a verb the daemon must not reach
// for panics instead of silently answering.
type recordingForge struct {
	extapi.ForgeRunner
	calls  []string
	merged int
}

func (r *recordingForge) DraftScriptTool(corr string, st toolforgeapi.ScriptTool) (toolforgeapi.ScriptTool, error) {
	r.calls = append(r.calls, "draft|"+corr+"|"+st.Name)
	return toolforgeapi.ScriptTool{Name: st.Name, ID: "owned"}, nil
}

func (r *recordingForge) UpdateScriptTool(corr, ref string, _ func(*toolforgeapi.ScriptTool)) (toolforgeapi.ScriptTool, bool, error) {
	r.calls = append(r.calls, "update|"+corr+"|"+ref)
	return toolforgeapi.ScriptTool{Name: ref}, true, nil
}

func (r *recordingForge) TestScriptTool(_ context.Context, corr, ref, input string) (toolforgeapi.ScriptTool, string, error) {
	r.calls = append(r.calls, "test|"+corr+"|"+ref+"|"+input)
	return toolforgeapi.ScriptTool{Name: ref}, "from the module", nil
}

func (r *recordingForge) PromoteScriptTool(corr, ref string) (toolforgeapi.ScriptTool, error) {
	r.calls = append(r.calls, "promote|"+corr+"|"+ref)
	return toolforgeapi.ScriptTool{Name: ref, Status: toolforgeapi.StatusActive}, nil
}

func (r *recordingForge) RequestToolPromotion(_ context.Context, corr, ref string) (toolforgeapi.ScriptTool, approvalapi.Decision, string, error) {
	r.calls = append(r.calls, "request-promotion|"+corr+"|"+ref)
	return toolforgeapi.ScriptTool{Name: ref}, approvalapi.DecisionGrant, "the operator said yes", nil
}

func (r *recordingForge) QuarantineScriptTool(corr, ref, reason string) (toolforgeapi.ScriptTool, error) {
	r.calls = append(r.calls, "quarantine|"+corr+"|"+ref+"|"+reason)
	return toolforgeapi.ScriptTool{Name: ref, Status: toolforgeapi.StatusQuarantined}, nil
}

func (r *recordingForge) RemoveScriptTool(corr, ref string) (bool, error) {
	r.calls = append(r.calls, "remove|"+corr+"|"+ref)
	return true, nil
}

func (r *recordingForge) MergeTools(tools map[string]toolapi.Tool) map[string]toolapi.Tool {
	r.calls = append(r.calls, "merge-tools")
	r.merged++
	out := make(map[string]toolapi.Tool, len(tools)+1)
	for name, t := range tools {
		out[name] = t
	}
	out["forge_owned"] = mcpStubTool{"forge_owned"}
	return out
}

// TestTheDaemonHoldsNoForgeRunnerWithoutTheModule: a daemon with no forge
// module is a daemon with no script tools — the daemon holds NO runner at
// all, so nothing can call into it.
func TestTheDaemonHoldsNoForgeRunnerWithoutTheModule(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if k.ForgeRunner() != nil {
		t.Fatal("a daemon without the module must hold no forge runner")
	}
	if k.ToolForge() != nil {
		t.Fatal("a daemon without the module must hold no forge store")
	}
}

// TestTheDaemonHoldsTheModulesForgeRunner: the daemon keeps the module's
// FACE and nothing of the lifecycle. The fake records what the module was
// asked for, so the answer is provably the module's.
//
// The fake is handed over the way the composition root hands it over — over
// runtime.Config.ForgeBuilder (W3.76k: the daemon's SetForge setter had no
// caller outside this test and is gone; the builder is the production path).
func TestTheDaemonHoldsTheModulesForgeRunner(t *testing.T) {
	fake := &recordingForge{}
	cfg := runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}
	cfg.ToolForge = stubForgeStore{}
	cfg.ForgeBuilder = func(extapi.ForgeDependencies) extapi.ForgeRunner { return fake }
	k, err := runtime.Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	runner := k.ForgeRunner()
	if runner == nil {
		t.Fatal("the daemon must hold the module's face")
	}
	if _, err := runner.DraftScriptTool("c1", toolforgeapi.ScriptTool{Name: "owned"}); err != nil {
		t.Fatalf("DraftScriptTool: %v", err)
	}
	if _, _, err := runner.UpdateScriptTool("c2", "owned", func(*toolforgeapi.ScriptTool) {}); err != nil {
		t.Fatalf("UpdateScriptTool: %v", err)
	}
	if _, _, err := runner.TestScriptTool(context.Background(), "c3", "owned", "{}"); err != nil {
		t.Fatalf("TestScriptTool: %v", err)
	}
	if _, err := runner.PromoteScriptTool("c4", "owned"); err != nil {
		t.Fatalf("PromoteScriptTool: %v", err)
	}
	if _, _, _, err := runner.RequestToolPromotion(context.Background(), "c5", "owned"); err != nil {
		t.Fatalf("RequestToolPromotion: %v", err)
	}
	if _, err := runner.QuarantineScriptTool("c6", "owned", "it misbehaved"); err != nil {
		t.Fatalf("QuarantineScriptTool: %v", err)
	}
	if _, err := runner.RemoveScriptTool("c7", "owned"); err != nil {
		t.Fatalf("RemoveScriptTool: %v", err)
	}

	want := []string{
		"draft|c1|owned",
		"update|c2|owned",
		"test|c3|owned|{}",
		"promote|c4|owned",
		"request-promotion|c5|owned",
		"quarantine|c6|owned|it misbehaved",
		"remove|c7|owned",
	}
	if !reflect.DeepEqual(fake.calls, want) {
		t.Fatalf("the module was asked for %v, want %v", fake.calls, want)
	}
}

// TestTheDaemonKeepsNoForgeVerbsOfItsOwn: the daemon's OWN public verbs must
// NOT name the forge's lifecycle. It kept seven forwarders over the module's
// operations once — a second spelling of each — and the application layer
// called the daemon for them. The module answers in the contract's names now
// (W3.76t) and the daemon spells none of them.
//
// The daemon keeps two: ToolForge (the store's FILE) and ForgeRunner (the
// module's FACE).
func TestTheDaemonKeepsNoForgeVerbsOfItsOwn(t *testing.T) {
	forbidden := []string{
		"DraftScriptTool", "UpdateScriptTool", "TestScriptTool",
		"PromoteScriptTool", "RequestToolPromotion", "QuarantineScriptTool", "RemoveScriptTool",
	}
	typ := reflect.TypeOf(&runtime.Kernel{})
	for _, name := range forbidden {
		if _, ok := typ.MethodByName(name); ok {
			t.Fatalf("the daemon grew the forge verb %s back", name)
		}
	}
	for _, name := range []string{"ToolForge", "ForgeRunner"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Fatalf("the daemon lost the verb %s", name)
		}
	}
}
