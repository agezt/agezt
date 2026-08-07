// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The mirror of plugins/builtintools' capability guard, for the tools the
// RUNTIME registers rather than the boot registry: memory, world, delegate,
// delegate_await, market, voice, image_generate, rerank. That package cannot see
// these (they are wired inside Open), and these are precisely the ones that had
// drifted — market, voice, image_generate and rerank all resolved to a
// capability Edict does not govern, which is default-DENIED, so every call was
// refused with "no trust level configured for <tool>".

// stubVoice/stubImageGen/stubReranker exist only so Open registers the tools
// their presence gates; the guard never invokes them.
type stubVoice struct{}

func (stubVoice) Transcribe(context.Context, []byte, string) (string, error) { return "", nil }
func (stubVoice) Speak(context.Context, string) ([]byte, string, error)      { return nil, "", nil }
func (stubVoice) HasSTT() bool                                               { return true }
func (stubVoice) HasTTS() bool                                               { return true }

type stubImageGen struct{}

func (stubImageGen) GenerateImage(context.Context, string, string, string, int) ([][]byte, string, error) {
	return nil, "", nil
}
func (stubImageGen) HasImage() bool { return true }

type stubReranker struct{}

func (stubReranker) Rerank(context.Context, string, []string, int) ([]int, []float64, error) {
	return nil, nil, nil
}
func (stubReranker) HasRerank() bool { return true }

// openFullToolKernel opens a kernel with every in-process-tool knob on, so the
// guards below see the whole runtime tool surface rather than whatever this
// test's defaults happen to be.
func openFullToolKernel(t *testing.T) map[string]agent.Tool {
	t.Helper()
	k, err := runtime.Open(runtime.Config{
		BaseDir:        t.TempDir(),
		Provider:       mock.New(mock.FinalText("ok")),
		MemoryTool:     true,
		WorldTool:      true,
		SubAgentTool:   true,
		MarketTool:     true,
		Voice:          stubVoice{},
		ImageGenerator: stubImageGen{},
		Reranker:       stubReranker{},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	tools := k.Tools()
	// Guard the guard: if a rename made those knobs no-ops, an empty set would
	// let every check below pass vacuously.
	for _, want := range []string{"memory", "world", "delegate", "delegate_await", "market", "voice", "image_generate", "rerank"} {
		if _, ok := tools[want]; !ok {
			t.Fatalf("tool %q not registered — these guards are only meaningful over the real tool set", want)
		}
	}
	return tools
}

func TestRuntimeTools_ResolveToGovernedCapabilities(t *testing.T) {
	for name := range openFullToolKernel(t) {
		cap := edict.CapabilityForToolCall(name, json.RawMessage(`{}`))
		if !edict.KnownCapability(string(cap)) {
			t.Errorf("tool %q resolves to capability %q, which Edict does not govern — an unknown "+
				"capability is DEFAULT-DENIED, so every call to this tool is refused. Map it in "+
				"kernel/edict/toolmap.go.", name, cap)
		}
	}
}

// The runtime-side half of the Phase 3.4 commit-B parity net: for every tool
// Open registers, what the tool DECLARES must equal what edict's name switch
// resolved before the declaration existed, and every one of them must declare.
// A wrong annotation would silently re-gate a tool, possibly onto a looser axis.
func TestRuntimeTools_DeclarationMatchesTheNameSwitch(t *testing.T) {
	for name, tool := range openFullToolKernel(t) {
		declared := tool.Definition().Capability
		if declared.IsZero() {
			t.Errorf("tool %q declares no Capability — set ToolDef.Capability in the tool's own "+
				"package so its policy axis lives next to its behaviour", name)
			continue
		}
		in := json.RawMessage(`{}`)
		want := edict.CapabilityForToolCall(name, in)
		if got := declared.For(in); got != string(want) {
			t.Errorf("%s: declares %q but the name switch resolves %q", name, got, want)
		}
	}
}

// The four that were dead, pinned individually with the axis each now rides —
// so a future remap is a deliberate edit here rather than a silent regression.
func TestPreviouslyUngovernedTools_HaveTheirAxis(t *testing.T) {
	for tool, want := range map[string]edict.Capability{
		"market":         edict.CapMarket,
		"voice":          edict.CapProviderCall,
		"image_generate": edict.CapProviderCall,
		"rerank":         edict.CapProviderCall,
		// Not runtime-registered, but it belongs to the same fix: the Conductor
		// rides code.exec because its verifier runs the worker's code through an
		// in-kernel call that never returns to the policy engine.
		"conductor": edict.CapCodeExec,
	} {
		if got := edict.CapabilityForToolCall(tool, json.RawMessage(`{}`)); got != want {
			t.Errorf("%s → %q, want %q", tool, got, want)
		}
	}
	// file op=glob was implemented and advertised in the schema but had no case,
	// so it landed on the unknown capability "file.glob" and was always denied.
	if got := edict.CapabilityForToolCall("file", json.RawMessage(`{"op":"glob"}`)); got != edict.CapFileList {
		t.Errorf("file op=glob → %q, want %q (it enumerates matching paths)", got, edict.CapFileList)
	}
}

// Regression: every one of them is now actually callable under the default
// posture. Resolving to a known axis is the mechanism; being allowed is the
// behaviour the operator sees.
func TestPreviouslyUngovernedTools_AreAllowedByDefault(t *testing.T) {
	eng := edict.New(edict.Options{})
	for _, tc := range []struct {
		tool  string
		input string
	}{
		{"market", `{"op":"install"}`},
		{"voice", `{}`},
		{"image_generate", `{}`},
		{"rerank", `{}`},
		{"conductor", `{}`},
		{"file", `{"op":"glob"}`},
	} {
		cap := edict.CapabilityForToolCall(tc.tool, json.RawMessage(tc.input))
		out := eng.Decide(cap, tc.input)
		if out.Decision != edict.DecisionAllow {
			t.Errorf("%s%s decides %v (%s) — it must be callable under the default allow-everything posture",
				tc.tool, tc.input, out.Decision, out.Reason)
		}
	}
}
