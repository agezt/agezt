// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/policy/guards"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The guards are the PLATFORM's rules (W3.74). This test pins the delegation:
// the reason the operator is shown is the PLATFORM's reason for the same
// evidence, not a copy of it kept in the daemon.
//
// A daemon that grew its own copy of the rule would produce the same string
// here — until the two drifted. The pin is the equality itself: the daemon
// asks, and what comes back is what the operator is told.
func TestThePromptInjectionGuardIsThePlatformsRule(t *testing.T) {
	var invoked int32
	reg := approval.New(approval.Config{Timeout: 5 * time.Second})
	prov := mock.New(
		testToolUse("read-1", "browser.read", map[string]any{}),
		testToolUse("probe-1", "approvalprobe", map[string]any{}),
		mock.FinalText("done"),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:              t.TempDir(),
		Provider:             prov,
		Tools:                map[string]toolapi.Tool{"browser.read": untrustedReadTool{}, "approvalprobe": probeTool{invoked: &invoked}},
		Edict:                edict.New(edict.Options{UnknownAllow: true}),
		Approvals:            reg,
		PromptInjectionGuard: runtime.PromptInjectionOn,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	done := make(chan error, 1)
	go func() {
		_, _, err := k.Run(context.Background(), "summarize the page")
		done <- err
	}()

	req := waitForPending(t, reg)

	// The platform's answer for the very same evidence.
	want := guards.Evaluate(guards.Input{
		Allow:           true,
		EffectClass:     string(toolapi.EffectIrreversible),
		PromptInjection: guards.PromptInjectionOn,
		DirectiveLike:   true,
		TaintSources:    []string{"https://evil.example/"},
	})
	if want.Guard != guards.GuardPromptInjection {
		t.Fatalf("the platform's rule must raise the guard, got %q", want.Guard)
	}
	if req.Reason != want.Reason {
		t.Fatalf("approval reason = %q, want the platform's %q", req.Reason, want.Reason)
	}

	if err := reg.Resolve(req.ID, approval.DecisionDeny, "external page attempted instruction injection", "operator"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("run did not finish after denial")
	}
	if atomic.LoadInt32(&invoked) != 0 {
		t.Fatalf("prompt-injection-gated tool executed %d times, want 0", invoked)
	}
}

// TestTheGuardIsJournaledAsAnAsk: a guard that raised is JOURNALED as a call
// that would have asked — the verdict's own "would ask" flag, not only the
// approval request. Without this the daemon could raise a guard and journal a
// verdict that says it never would have asked.
func TestTheGuardIsJournaledAsAnAsk(t *testing.T) {
	var invoked int32
	reg := approval.New(approval.Config{Timeout: 5 * time.Second})
	prov := mock.New(
		testToolUse("read-1", "browser.read", map[string]any{}),
		testToolUse("probe-1", "approvalprobe", map[string]any{}),
		mock.FinalText("done"),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:              t.TempDir(),
		Provider:             prov,
		Tools:                map[string]toolapi.Tool{"browser.read": untrustedReadTool{}, "approvalprobe": probeTool{invoked: &invoked}},
		Edict:                edict.New(edict.Options{UnknownAllow: true}),
		Approvals:            reg,
		PromptInjectionGuard: runtime.PromptInjectionOn,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	done := make(chan error, 1)
	go func() {
		_, _, err := k.Run(context.Background(), "summarize the page")
		done <- err
	}()
	req := waitForPending(t, reg)
	if err := reg.Resolve(req.ID, approval.DecisionDeny, "external page attempted instruction injection", "operator"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("run did not finish after denial")
	}

	var found bool
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindPolicyDecision {
			return nil
		}
		var p struct {
			Tool      string `json:"tool"`
			Allow     bool   `json:"allow"`
			WouldAsk  bool   `json:"would_ask"`
			Reason    string `json:"reason"`
			HardDeny  bool   `json:"hard_denied"`
			Capabilty string `json:"capability"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatalf("policy payload: %v", err)
		}
		if p.Tool != "approvalprobe" {
			return nil
		}
		found = true
		if !p.WouldAsk {
			t.Errorf("would_ask = false, want true (the guard asked for a human)")
		}
		if p.Allow {
			t.Errorf("allow = true, want false (the guard stopped the call)")
		}
		if p.HardDeny {
			t.Errorf("hard_denied = true, want false (a guard asks, it does not deny)")
		}
		return nil
	})
	if !found {
		t.Fatal("no policy.decision event for the gated tool")
	}
}

// TestThePromptInjectionGuardJournalsItsWarning: in warn mode the call runs —
// and the warning is JOURNALED, because that is what the chat surfaces as a
// passive banner. A daemon that stopped warning would still let the call
// run, so the call alone cannot witness the warning.
func TestThePromptInjectionGuardJournalsItsWarning(t *testing.T) {
	var invoked int32
	prov := mock.New(
		testToolUse("read-1", "browser.read", map[string]any{}),
		testToolUse("probe-1", "approvalprobe", map[string]any{}),
		mock.FinalText("done"),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:              t.TempDir(),
		Provider:             prov,
		Tools:                map[string]toolapi.Tool{"browser.read": untrustedReadTool{}, "approvalprobe": probeTool{invoked: &invoked}},
		Edict:                edict.New(edict.Options{UnknownAllow: true}),
		PromptInjectionGuard: runtime.PromptInjectionWarn,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.Run(context.Background(), "summarize the page"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := atomic.LoadInt32(&invoked); got != 1 {
		t.Fatalf("probe executed %d times, want 1 (warn mode must not block)", got)
	}

	var found bool
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindPromptInjectionWarned {
			return nil
		}
		found = true
		var p struct {
			Tool       string   `json:"tool"`
			Sources    []string `json:"sources"`
			TrustedRun bool     `json:"trusted_run"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatalf("warning payload: %v", err)
		}
		if p.Tool != "approvalprobe" {
			t.Errorf("warned tool = %q, want approvalprobe", p.Tool)
		}
		if len(p.Sources) == 0 {
			t.Errorf("warned sources = %v, want the observation's source", p.Sources)
		}
		if p.TrustedRun {
			t.Errorf("trusted_run = true, want false")
		}
		return nil
	})
	if !found {
		t.Fatal("no prompt_injection.warned event")
	}
}

// TestTheIntentGateJournalsItsConfirmationRequest: the intent/regret gate's
// question is JOURNALED as well as asked — the operator sees the frame and
// the prompt in the policy log.
func TestTheIntentGateJournalsItsConfirmationRequest(t *testing.T) {
	var invoked int32
	reg := approval.New(approval.Config{Timeout: 5 * time.Second})
	prov := mock.New(
		testToolUse("c1", "approvalprobe", map[string]any{"path": "legacy/2023"}),
		mock.FinalText("done"),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:                t.TempDir(),
		Provider:               prov,
		Tools:                  map[string]toolapi.Tool{"approvalprobe": probeTool{invoked: &invoked}},
		Edict:                  edict.New(edict.Options{UnknownAllow: true}),
		Approvals:              reg,
		IntentRegretGating:     true,
		DisableHeuristicBypass: true,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	done := make(chan error, 1)
	go func() {
		_, _, err := k.Run(context.Background(), "clean files")
		done <- err
	}()
	req := waitForPending(t, reg)
	if err := reg.Resolve(req.ID, approval.DecisionDeny, "scope is ambiguous", "operator"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("run did not finish after denial")
	}

	var found bool
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindIntentConfirmationRequired {
			return nil
		}
		found = true
		var p struct {
			CanonicalIntent    string             `json:"canonical_intent"`
			ConfirmationPrompt string             `json:"confirmation_prompt"`
			RegretAxes         map[string]float64 `json:"regret_axes"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatalf("confirmation payload: %v", err)
		}
		if p.CanonicalIntent != "clean files" {
			t.Errorf("canonical intent = %q, want clean files", p.CanonicalIntent)
		}
		if p.ConfirmationPrompt == "" {
			t.Error("the journaled confirmation must carry the prompt")
		}
		if p.RegretAxes["informational"] < 0.75 {
			t.Errorf("regret axes = %+v, want high informational risk", p.RegretAxes)
		}
		return nil
	})
	if !found {
		t.Fatal("no intent.confirmation_required event")
	}
}
