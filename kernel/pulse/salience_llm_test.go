// SPDX-License-Identifier: MIT

package pulse

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/llm"
)

// stubProvider satisfies llm.Provider without answering anything: the refine
// path is injected, so the provider is only ever handed to it.
type stubProvider struct{}

func (stubProvider) Name() string { return "stub" }

func (stubProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "{}"}}, nil
}

// genCall records what the injected path was asked: how often, and with what.
type genCall struct {
	calls int
	req   llm.CompletionRequest
}

// scoreGen is a generator that answers with the given score and reason.
func scoreGen(score float64, reason string, seen *genCall) llm.Generator {
	return func(ctx context.Context, p llm.Provider, req llm.CompletionRequest, schema json.RawMessage, out any) (llm.Usage, error) {
		if seen != nil {
			seen.calls++
			seen.req = req
		}
		data, err := json.Marshal(map[string]any{"score": score, "reason": reason})
		if err != nil {
			return llm.Usage{}, err
		}
		if err := json.Unmarshal(data, out); err != nil {
			return llm.Usage{}, err
		}
		return llm.Usage{}, nil
	}
}

// newRefineSalience builds a salience whose LLM refine is armed: a provider,
// the path, and the switch on.
func newRefineSalience(gen llm.Generator) *Salience {
	return &Salience{
		provider:   stubProvider{},
		generator:  gen,
		model:      "test-model",
		dial:       DialBalanced,
		useLLM:     true,
		noveltyTTL: 30 * time.Minute,
		now:        func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
}

// TestTheLLMRefineUsesTheInjectedPath: the refine is a PORT (W3.66) — the
// module asks the injected path for a typed value, and the score it gets back
// is the score the operator sees.
//
// The path must also be asked ABOUT THE DELTA: the model is told what
// changed, so its judgement is about this change and not about the last one.
func TestTheLLMRefineUsesTheInjectedPath(t *testing.T) {
	seen := &genCall{}
	s := newRefineSalience(scoreGen(0.9, "the disk is about to fill", seen))

	sc := s.Score(context.Background(), Delta{Source: "disk", Kind: "usage", Summary: "disk at 91%"})

	if seen.calls != 1 {
		t.Fatalf("the injected path was called %d times, want 1", seen.calls)
	}
	if sc.Value != 0.9 {
		t.Errorf("score = %v, want the refined 0.9", sc.Value)
	}
	if sc.Reason != "llm: the disk is about to fill" {
		t.Errorf("reason = %q, want the model's reason", sc.Reason)
	}
	if !strings.Contains(seen.req.System, "score") {
		t.Errorf("the refine must ask the model to score, system = %q", seen.req.System)
	}
	var asked string
	for _, m := range seen.req.Messages {
		if m.Role == llm.RoleUser {
			asked = m.Content
		}
	}
	if !strings.Contains(asked, "disk at 91%") {
		t.Errorf("the refine must ask about THIS change, asked %q", asked)
	}
}

// TestTheLLMRefineIsOffWithoutTheSwitch: the refine is OPT-IN (the daemon
// reads PULSE_LLM=on). Without the switch the path is never asked — a pulse
// that refines on every beat would cost a model call per delta, unasked.
func TestTheLLMRefineIsOffWithoutTheSwitch(t *testing.T) {
	seen := &genCall{}
	s := newRefineSalience(scoreGen(0.9, "the disk is about to fill", seen))
	s.useLLM = false

	sc := s.Score(context.Background(), Delta{Source: "disk", Kind: "usage", Summary: "disk at 91%"})

	if seen.calls != 0 {
		t.Fatalf("the path was called %d times with the switch off, want 0", seen.calls)
	}
	if strings.HasPrefix(sc.Reason, "llm: ") {
		t.Errorf("reason = %q, want the severity reason with the switch off", sc.Reason)
	}
}

// TestTheLLMRefineSkipsWhenNoPathIsInjected: a host that wires no structured-
// output path gets NO refine — the severity score stands and nothing pretends
// a model was asked. (The path is optional; the refine must never panic on a
// nil one.)
func TestTheLLMRefineSkipsWhenNoPathIsInjected(t *testing.T) {
	s := newRefineSalience(nil)

	sc := s.Score(context.Background(), Delta{Source: "disk", Kind: "usage", Summary: "disk at 91%"})

	if strings.HasPrefix(sc.Reason, "llm: ") {
		t.Errorf("reason = %q, want the severity reason (no refine)", sc.Reason)
	}
	if sc.Value == 0.9 {
		t.Errorf("score = %v, want the severity score, not a refined one", sc.Value)
	}
}

// TestTheLLMRefineSkipsWhenThePathFails: an unusable answer is a skip, not a
// zero score — the severity score stands.
func TestTheLLMRefineSkipsWhenThePathFails(t *testing.T) {
	s := newRefineSalience(func(ctx context.Context, p llm.Provider, req llm.CompletionRequest, schema json.RawMessage, out any) (llm.Usage, error) {
		return llm.Usage{}, errors.New("provider down")
	})

	sc := s.Score(context.Background(), Delta{Source: "disk", Kind: "usage", Summary: "disk at 91%"})

	if sc.Value == 0.9 {
		t.Errorf("score = %v, want the severity score after a failed refine", sc.Value)
	}
	if strings.HasPrefix(sc.Reason, "llm: ") {
		t.Errorf("reason = %q, want the severity reason after a failed refine", sc.Reason)
	}
}

// TestTheLLMRefineClampsTheScore: the model's score is bounded to [0,1] — a
// model that answers 5.0 must not make a medium delta an alert.
func TestTheLLMRefineClampsTheScore(t *testing.T) {
	for _, tc := range []struct {
		name  string
		given float64
		want  float64
	}{
		{"above one", 5, 1},
		{"below zero", -3, 0},
		{"inside", 0.42, 0.42},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newRefineSalience(scoreGen(tc.given, "r", nil))
			sc := s.Score(context.Background(), Delta{Source: "disk", Kind: "usage", Summary: "disk at 91%"})
			if sc.Value != tc.want {
				t.Errorf("score = %v, want %v", sc.Value, tc.want)
			}
		})
	}
}
