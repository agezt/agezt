// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// guardedAutoApproveKernel is injectionKernel plus a daemon-wide auto-approve
// grant covering the probe capability — the shipped default, which covers every
// capability. Edict itself allows (UnknownAllow), so the ONLY thing that can
// route the probe to approval is a fail-closed guard.
func guardedAutoApproveKernel(t *testing.T, prov agent.Provider, invoked *int32, mode runtime.PromptInjectionMode) *runtime.Kernel {
	t.Helper()
	// Safety net: a wrongful gate denies on timeout, it never hangs the test.
	reg := approval.New(approval.Config{Timeout: 2 * time.Second})
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		Tools: map[string]agent.Tool{
			"browser.read":  untrustedReadTool{},
			"approvalprobe": probeTool{invoked: invoked},
		},
		Edict:                   edict.New(edict.Options{UnknownAllow: true}),
		Approvals:               reg,
		PromptInjectionGuard:    mode,
		AutoApproveCapabilities: map[string]bool{"approvalprobe": true},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

// TestPromptInjectionGuard_AutoApproveDoesNotSatisfyGuard is the AC-011
// regression.
//
// An auto-approve grant answers the Edict Ask axis: the operator said "don't
// stop me for this capability". It is NOT an answer to the prompt-injection
// guard, which the operator armed separately and deliberately (the default is
// warn, so reaching PromptInjectionOn takes an explicit act). Before the fix,
// the session-grant branch in policyHook ran unconditionally on
// requiresApproval and flipped the guard's deny straight back to allow — so an
// operator who armed the guard got no gate, and the journal recorded a plain
// L4 allow with no trace that the guard had fired at all.
//
// Same shape as TestPromptInjectionGuard_GatedWithinWindow: the effectful probe
// lands in the turn immediately after the directive-like untrusted observation,
// so it must be gated. Gating means the approval times out and denies, so the
// probe never executes.
func TestPromptInjectionGuard_AutoApproveDoesNotSatisfyGuard(t *testing.T) {
	var invoked int32
	prov := mock.New(
		testToolUse("read-1", "browser.read", map[string]any{}),
		testToolUse("probe-1", "approvalprobe", map[string]any{}),
		mock.FinalText("done"),
	)
	k := guardedAutoApproveKernel(t, prov, &invoked, runtime.PromptInjectionOn)
	_, _, _ = k.Run(context.Background(), "act now")
	if got := atomic.LoadInt32(&invoked); got != 0 {
		t.Fatalf("probe executed %d times, want 0: an auto-approve grant must not satisfy the prompt-injection guard", got)
	}
}

// TestPromptInjectionGuard_AutoApproveStillSatisfiesAskAxis is the other half of
// the fix, and the reason it is narrow. With no taint in play the guard never
// fires, so the grant must still do its job — otherwise the fix would have
// broken auto-approve for everyone rather than scoping it.
func TestPromptInjectionGuard_AutoApproveStillSatisfiesAskAxis(t *testing.T) {
	var invoked int32
	prov := mock.New(
		testToolUse("probe-1", "approvalprobe", map[string]any{}),
		mock.FinalText("done"),
	)
	reg := approval.New(approval.Config{Timeout: 2 * time.Second})
	k, err := runtime.Open(runtime.Config{
		BaseDir:                 t.TempDir(),
		Provider:                prov,
		Tools:                   map[string]agent.Tool{"approvalprobe": probeTool{invoked: &invoked}},
		Edict:                   edict.New(edict.Options{Levels: map[edict.Capability]edict.TrustLevel{"approvalprobe": edict.LevelAsk}, AskPolicy: edict.AskPrompt}),
		Approvals:               reg,
		PromptInjectionGuard:    runtime.PromptInjectionOn,
		AutoApproveCapabilities: map[string]bool{"approvalprobe": true},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.Run(context.Background(), "probe"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := atomic.LoadInt32(&invoked); got != 1 {
		t.Fatalf("probe invoked %d times, want 1: the grant must still satisfy an Edict Ask", got)
	}
	if pending := reg.Pending(); len(pending) != 0 {
		t.Fatalf("auto-approved run left %d pending approval(s)", len(pending))
	}
}
