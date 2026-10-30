// SPDX-License-Identifier: MIT

package runtime_test

// The daemon's forge verbs: the NAMES it keeps and the BODY it gave away.
//
// The daemon used to hold the forge's whole lifecycle — the catalogue AND the
// operations over it, the sandbox test, the operator's queue and the bridge
// that folds the promoted scripts into a run's tool map. It now holds the
// extensions module's face and asks the module. These tests pin that with a
// fake forge: if the daemon grew the lifecycle back, the fake would record
// nothing and the daemon's answer would be its own.

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/approvalapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// recordingForge is a fake extensions module: it records what the daemon asks
// and answers with a row of its own.
//
// The embedded interface is nil on purpose: a verb the daemon must not reach
// for panics instead of silently answering.
type recordingForge struct {
	extapi.ForgeService
	calls  []string
	merged int
}

func (r *recordingForge) Draft(corr string, st toolforgeapi.ScriptTool) (toolforgeapi.ScriptTool, error) {
	r.calls = append(r.calls, "draft|"+corr+"|"+st.Name)
	return toolforgeapi.ScriptTool{Name: st.Name, ID: "owned"}, nil
}

func (r *recordingForge) Update(corr, ref string, _ func(*toolforgeapi.ScriptTool)) (toolforgeapi.ScriptTool, bool, error) {
	r.calls = append(r.calls, "update|"+corr+"|"+ref)
	return toolforgeapi.ScriptTool{Name: ref}, true, nil
}

func (r *recordingForge) Test(_ context.Context, corr, ref, input string) (toolforgeapi.ScriptTool, string, error) {
	r.calls = append(r.calls, "test|"+corr+"|"+ref+"|"+input)
	return toolforgeapi.ScriptTool{Name: ref}, "from the module", nil
}

func (r *recordingForge) Promote(corr, ref string) (toolforgeapi.ScriptTool, error) {
	r.calls = append(r.calls, "promote|"+corr+"|"+ref)
	return toolforgeapi.ScriptTool{Name: ref, Status: toolforgeapi.StatusActive}, nil
}

func (r *recordingForge) RequestPromotion(_ context.Context, corr, ref string) (toolforgeapi.ScriptTool, approvalapi.Decision, string, error) {
	r.calls = append(r.calls, "request-promotion|"+corr+"|"+ref)
	return toolforgeapi.ScriptTool{Name: ref}, approvalapi.DecisionGrant, "the operator said yes", nil
}

func (r *recordingForge) Quarantine(corr, ref, reason string) (toolforgeapi.ScriptTool, error) {
	r.calls = append(r.calls, "quarantine|"+corr+"|"+ref+"|"+reason)
	return toolforgeapi.ScriptTool{Name: ref, Status: toolforgeapi.StatusQuarantined}, nil
}

func (r *recordingForge) Remove(corr, ref string) (bool, error) {
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

// TestTheDaemonRefusesEveryForgeVerbWithoutTheModule: a daemon with no
// forge module is a daemon with no script tools — the writes must SAY so
// rather than call into nothing.
func TestTheDaemonRefusesEveryForgeVerbWithoutTheModule(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, err := k.DraftScriptTool("corr", toolforgeapi.ScriptTool{Name: "owned"}); !errors.Is(err, runtime.ErrNoForge) {
		t.Fatalf("draft without the module must refuse, got %v", err)
	}
	if _, _, err := k.UpdateScriptTool("corr", "owned", func(*toolforgeapi.ScriptTool) {}); !errors.Is(err, runtime.ErrNoForge) {
		t.Fatalf("update without the module must refuse, got %v", err)
	}
	if _, _, err := k.TestScriptTool(context.Background(), "corr", "owned", "{}"); !errors.Is(err, runtime.ErrNoForge) {
		t.Fatalf("test without the module must refuse, got %v", err)
	}
	if _, err := k.PromoteScriptTool("corr", "owned"); !errors.Is(err, runtime.ErrNoForge) {
		t.Fatalf("promote without the module must refuse, got %v", err)
	}
	if _, _, _, err := k.RequestToolPromotion(context.Background(), "corr", "owned"); !errors.Is(err, runtime.ErrNoForge) {
		t.Fatalf("request-promotion without the module must refuse, got %v", err)
	}
	if _, err := k.QuarantineScriptTool("corr", "owned", "why"); !errors.Is(err, runtime.ErrNoForge) {
		t.Fatalf("quarantine without the module must refuse, got %v", err)
	}
	if _, err := k.RemoveScriptTool("corr", "owned"); !errors.Is(err, runtime.ErrNoForge) {
		t.Fatalf("remove without the module must refuse, got %v", err)
	}
}

// TestTheDaemonForwardsEveryForgeVerbToTheModule: the daemon keeps the
// NAMES; the module answers. The fake records what the daemon asked for.
func TestTheDaemonForwardsEveryForgeVerbToTheModule(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	fake := &recordingForge{}
	k.SetForge(fake)

	if _, err := k.DraftScriptTool("c1", toolforgeapi.ScriptTool{Name: "owned"}); err != nil {
		t.Fatalf("DraftScriptTool: %v", err)
	}
	if _, _, err := k.UpdateScriptTool("c2", "owned", func(*toolforgeapi.ScriptTool) {}); err != nil {
		t.Fatalf("UpdateScriptTool: %v", err)
	}
	if _, _, err := k.TestScriptTool(context.Background(), "c3", "owned", "{}"); err != nil {
		t.Fatalf("TestScriptTool: %v", err)
	}
	if _, err := k.PromoteScriptTool("c4", "owned"); err != nil {
		t.Fatalf("PromoteScriptTool: %v", err)
	}
	if _, _, _, err := k.RequestToolPromotion(context.Background(), "c5", "owned"); err != nil {
		t.Fatalf("RequestToolPromotion: %v", err)
	}
	if _, err := k.QuarantineScriptTool("c6", "owned", "it misbehaved"); err != nil {
		t.Fatalf("QuarantineScriptTool: %v", err)
	}
	if _, err := k.RemoveScriptTool("c7", "owned"); err != nil {
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
		t.Fatalf("the daemon asked the module for %v, want %v", fake.calls, want)
	}
}

// TestTheDaemonKeepsTheForgeNames: the host's public API is unchanged — the
// eight forge verbs are still the daemon's.
func TestTheDaemonKeepsTheForgeNames(t *testing.T) {
	want := []string{
		"ToolForge", "DraftScriptTool", "UpdateScriptTool", "TestScriptTool",
		"PromoteScriptTool", "RequestToolPromotion", "QuarantineScriptTool", "RemoveScriptTool",
	}
	typ := reflect.TypeOf(&runtime.Kernel{})
	for _, name := range want {
		if _, ok := typ.MethodByName(name); !ok {
			t.Fatalf("the daemon lost the verb %s", name)
		}
	}
}
