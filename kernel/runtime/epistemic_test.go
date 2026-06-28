// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type epistemicTool struct {
	name       string
	class      agent.EffectClass
	confidence float64
	failFirst  bool
	calls      *int32
}

func (t epistemicTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name:        t.name,
		Description: "epistemic test tool",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"target":{"type":"string"}},
			"required":["target"],
			"additionalProperties":false
		}`),
		Effect: agent.ToolEffect{
			Class:             t.class,
			PredictedEffects:  []string{"touch epistemic target"},
			AffectedResources: []string{"epistemic target"},
			RollbackNotes:     "test rollback",
			Confidence:        t.confidence,
		},
	}
}

func (t epistemicTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	n := atomic.AddInt32(t.calls, 1)
	if t.failFirst && n == 1 {
		return agent.Result{Output: "simulated matched failure", IsError: true}, nil
	}
	return agent.Result{Output: "ok"}, nil
}

func TestRunWith_EpistemicSignalsJournaledWithoutEscalation(t *testing.T) {
	var calls int32
	prov := mock.New(
		testToolUse("c1", "freshprobe", map[string]any{"target": "latest release"}),
		mock.FinalText("done"),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		Tools: map[string]agent.Tool{
			"freshprobe": epistemicTool{
				name: "freshprobe", class: agent.EffectReadOnly, confidence: 0.9, calls: &calls,
			},
		},
		Edict: edict.New(edict.Options{UnknownAllow: true}),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.Run(context.Background(), "check latest release"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("tool calls = %d, want 1", calls)
	}

	var found bool
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindPolicyDecision {
			return nil
		}
		var p struct {
			EpistemicAction     string   `json:"epistemic_action"`
			EpistemicSignals    []string `json:"epistemic_signals"`
			TemporalSensitive   bool     `json:"temporal_sensitive"`
			NovelTool           bool     `json:"novel_tool"`
			SchemaHash          string   `json:"schema_hash"`
			InputShape          string   `json:"input_shape"`
			EpistemicConfidence float64  `json:"epistemic_confidence"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatalf("policy payload: %v", err)
		}
		found = true
		if p.EpistemicAction != "allow" {
			t.Errorf("epistemic_action = %q want allow", p.EpistemicAction)
		}
		if !p.TemporalSensitive || !p.NovelTool {
			t.Errorf("temporal/novel = %v/%v, want true/true", p.TemporalSensitive, p.NovelTool)
		}
		if p.SchemaHash == "" || !strings.Contains(p.InputShape, "target:string") {
			t.Errorf("schema/input condition missing: hash=%q shape=%q", p.SchemaHash, p.InputShape)
		}
		if p.EpistemicConfidence != 0.9 {
			t.Errorf("epistemic confidence = %v want 0.9", p.EpistemicConfidence)
		}
		return nil
	})
	if !found {
		t.Fatal("no policy.decision event")
	}
}

func TestRunWith_EpistemicEscalationRoutesLowConfidenceToApproval(t *testing.T) {
	var calls int32
	reg := approval.New(approval.Config{Timeout: 5 * time.Second})
	prov := mock.New(
		testToolUse("c1", "riskprobe", map[string]any{"target": "prod"}),
		mock.FinalText("done"),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:                t.TempDir(),
		Provider:               prov,
		Approvals:              reg,
		EpistemicEscalation:    true,
		Tools:                  map[string]agent.Tool{"riskprobe": epistemicTool{name: "riskprobe", class: agent.EffectCompensable, confidence: 0.3, calls: &calls}},
		Edict:                  edict.New(edict.Options{UnknownAllow: true}),
		ApprovalTimeout:        5 * time.Second,
		ToolCapabilities:       nil,
		ObservationDeltas:      false,
		ToolDiscoveryMax:       0,
		ContextBudgetAuto:      false,
		ContextProtectFirst:    0,
		ContextSummarize:       false,
		DisableHeuristicBypass: true,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	done := make(chan error, 1)
	go func() {
		_, _, err := k.Run(context.Background(), "run risky probe")
		done <- err
	}()

	req := waitForPending(t, reg)
	if !strings.Contains(req.Reason, "epistemic policy requires human review") {
		t.Fatalf("approval reason = %q, want epistemic escalation", req.Reason)
	}
	if req.Confidence != 0.3 {
		t.Fatalf("approval confidence = %v want 0.3", req.Confidence)
	}
	if err := reg.Resolve(req.ID, approval.DecisionDeny, "not enough certainty", "operator"); err != nil {
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
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("low-confidence escalated tool executed %d times, want 0", calls)
	}
}

func TestRunWith_EpistemicEscalationMatchesHistoricalFailureConditions(t *testing.T) {
	var calls int32
	reg := approval.New(approval.Config{Timeout: 5 * time.Second})
	prov := mock.New(
		testToolUse("c1", "histprobe", map[string]any{"target": "prod"}),
		mock.FinalText("first done"),
		testToolUse("c2", "histprobe", map[string]any{"target": "prod"}),
		mock.FinalText("second done"),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:             t.TempDir(),
		Provider:            prov,
		Approvals:           reg,
		EpistemicEscalation: true,
		Tools: map[string]agent.Tool{
			"histprobe": epistemicTool{
				name: "histprobe", class: agent.EffectCompensable, confidence: 0.95, failFirst: true, calls: &calls,
			},
		},
		Edict: edict.New(edict.Options{UnknownAllow: true}),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.Run(context.Background(), "first historical run"); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("after first run calls = %d want 1", calls)
	}

	done := make(chan error, 1)
	go func() {
		_, _, err := k.Run(context.Background(), "second historical run")
		done <- err
	}()
	req := waitForPending(t, reg)
	if !strings.Contains(req.Reason, "matched_failure_conditions") {
		t.Fatalf("approval reason = %q, want matched failure condition", req.Reason)
	}
	if err := reg.Resolve(req.ID, approval.DecisionDeny, "same failure mode", "operator"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second Run: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second run did not finish after denial")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("historically risky second call executed; calls = %d want 1", calls)
	}
}
