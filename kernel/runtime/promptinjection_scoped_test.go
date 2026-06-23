// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// benignReadTool returns untrusted-but-not-directive content, used to advance an
// iteration without re-tainting the run.
type benignReadTool struct{}

func (benignReadTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name:        "benign.read",
		Description: "benign web reader",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		Effect:      agent.ToolEffect{Class: agent.EffectReadOnly},
	}
}

func (benignReadTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	return agent.Result{
		Output:            "A normal search result about pasta recipes and cooking times.",
		ObservationTrust:  agent.ObservationUntrusted,
		ObservationSource: "https://ok.example/",
	}, nil
}

func injectionKernel(t *testing.T, prov agent.Provider, invoked *int32, mode runtime.PromptInjectionMode) (*runtime.Kernel, *approval.Registry) {
	t.Helper()
	reg := approval.New(approval.Config{Timeout: 2 * time.Second}) // safety net: a wrongful gate denies, never hangs
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		Tools: map[string]agent.Tool{
			"browser.read":  untrustedReadTool{},
			"benign.read":   benignReadTool{},
			"approvalprobe": probeTool{invoked: invoked},
		},
		Edict:                edict.New(edict.Options{UnknownAllow: true}),
		Approvals:            reg,
		PromptInjectionGuard: mode,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k, reg
}

// TestPromptInjectionGuard_DecaysAfterWindow: an effectful action two turns
// after the directive-like observation is NOT gated — the run-wide-sticky-taint
// fix. read(directive) → benign read → probe.
func TestPromptInjectionGuard_DecaysAfterWindow(t *testing.T) {
	var invoked int32
	prov := mock.New(
		mock.ToolUse("read-1", "browser.read", map[string]any{}),
		mock.ToolUse("benign-1", "benign.read", map[string]any{}),
		mock.ToolUse("probe-1", "approvalprobe", map[string]any{}),
		mock.FinalText("done"),
	)
	k, _ := injectionKernel(t, prov, &invoked, runtime.PromptInjectionOn)
	if _, _, err := k.Run(context.Background(), "research then act"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := atomic.LoadInt32(&invoked); got != 1 {
		t.Fatalf("probe executed %d times, want 1 (action past the decay window must not be gated)", got)
	}
}

// TestPromptInjectionGuard_GatedWithinWindow: the existing guarantee still holds
// — an effectful action in the turn immediately after the directive observation
// IS gated (here it auto-denies via the 2s approval timeout, so the probe never
// runs).
func TestPromptInjectionGuard_GatedWithinWindow(t *testing.T) {
	var invoked int32
	prov := mock.New(
		mock.ToolUse("read-1", "browser.read", map[string]any{}),
		mock.ToolUse("probe-1", "approvalprobe", map[string]any{}),
		mock.FinalText("done"),
	)
	k, _ := injectionKernel(t, prov, &invoked, runtime.PromptInjectionOn)
	_, _, _ = k.Run(context.Background(), "act now") // probe gated → approval times out → denied
	if got := atomic.LoadInt32(&invoked); got != 0 {
		t.Fatalf("probe executed %d times, want 0 (action in the window must be gated)", got)
	}
}

// TestPromptInjectionGuard_WarnModeDoesNotBlock: in warn mode the in-window
// effectful action runs without approval.
func TestPromptInjectionGuard_WarnModeDoesNotBlock(t *testing.T) {
	var invoked int32
	prov := mock.New(
		mock.ToolUse("read-1", "browser.read", map[string]any{}),
		mock.ToolUse("probe-1", "approvalprobe", map[string]any{}),
		mock.FinalText("done"),
	)
	k, _ := injectionKernel(t, prov, &invoked, runtime.PromptInjectionWarn)
	if _, _, err := k.Run(context.Background(), "act now"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := atomic.LoadInt32(&invoked); got != 1 {
		t.Fatalf("probe executed %d times, want 1 (warn mode must not block)", got)
	}
}

// TestPromptInjectionGuard_TrustedRunDoesNotBlock: an On-mode run that the
// operator has trusted (chat "trust this run") runs the in-window effectful
// action without approval.
func TestPromptInjectionGuard_TrustedRunDoesNotBlock(t *testing.T) {
	var invoked int32
	prov := mock.New(
		mock.ToolUse("read-1", "browser.read", map[string]any{}),
		mock.ToolUse("probe-1", "approvalprobe", map[string]any{}),
		mock.FinalText("done"),
	)
	k, _ := injectionKernel(t, prov, &invoked, runtime.PromptInjectionOn)
	ctx := runtime.WithTrustedObservations(context.Background())
	if _, _, err := k.Run(ctx, "act now"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := atomic.LoadInt32(&invoked); got != 1 {
		t.Fatalf("probe executed %d times, want 1 (trusted run must not block)", got)
	}
}
