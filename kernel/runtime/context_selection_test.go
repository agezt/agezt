// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestRunWith_ContextSelectionJournalsChosenAndRejectedMemory(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:      t.TempDir(),
		Provider:     mock.New(mock.FinalText("done")),
		MemoryInject: true,
		MemoryTopK:   1,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	_, _, _ = k.Memory().Remember("seed", memory.RememberSpec{Type: memory.TypeFact, Subject: "alpha", Content: "alpha context chosen release"})
	_, _, _ = k.Memory().Remember("seed", memory.RememberSpec{Type: memory.TypeFact, Subject: "alpha", Content: "alpha context rejected deployment"})

	if _, _, err := k.Run(context.Background(), "alpha context"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var found bool
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindContextSelection {
			return nil
		}
		var p struct {
			Phase    string `json:"phase"`
			Chosen   []any  `json:"chosen"`
			Rejected []any  `json:"rejected"`
			Summary  struct {
				Chosen   int `json:"chosen"`
				Rejected int `json:"rejected"`
			} `json:"summary"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatalf("context.selection payload: %v", err)
		}
		if p.Phase != "memory" {
			return nil
		}
		found = true
		if len(p.Chosen) != 1 || len(p.Rejected) != 1 {
			t.Fatalf("chosen/rejected = %d/%d, want 1/1", len(p.Chosen), len(p.Rejected))
		}
		if p.Summary.Chosen != 1 || p.Summary.Rejected != 1 {
			t.Fatalf("summary chosen/rejected = %d/%d, want 1/1", p.Summary.Chosen, p.Summary.Rejected)
		}
		return nil
	})
	if !found {
		t.Fatal("no memory context.selection event")
	}
}

func TestRunWith_ContextFailureAnalysisUsesRejectedSet(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:      t.TempDir(),
		Provider:     failingProvider{},
		MemoryInject: true,
		MemoryTopK:   1,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	_, _, _ = k.Memory().Remember("seed", memory.RememberSpec{Type: memory.TypeFact, Subject: "alpha", Content: "alpha context chosen release"})
	_, _, _ = k.Memory().Remember("seed", memory.RememberSpec{Type: memory.TypeFact, Subject: "alpha", Content: "alpha context rejected deployment"})

	if _, _, err := k.Run(context.Background(), "alpha context"); err == nil {
		t.Fatal("Run succeeded; want provider failure")
	}

	var found bool
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindContextSelection {
			return nil
		}
		var p struct {
			Phase   string `json:"phase"`
			Summary struct {
				Classifier       string   `json:"classifier"`
				SuspectOmission  bool     `json:"suspect_omission"`
				Counterfactuals  []string `json:"counterfactuals"`
				CreditAssignment string   `json:"credit_assignment"`
			} `json:"summary"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatalf("context.selection payload: %v", err)
		}
		if p.Phase != "failure_analysis" {
			return nil
		}
		found = true
		if p.Summary.Classifier != "heuristic_external" || !p.Summary.SuspectOmission {
			t.Fatalf("failure classifier summary = %+v", p.Summary)
		}
		if len(p.Summary.Counterfactuals) == 0 || p.Summary.CreditAssignment == "" {
			t.Fatalf("counterfactual failure analysis incomplete: %+v", p.Summary)
		}
		return nil
	})
	if !found {
		t.Fatal("no failure_analysis context.selection event")
	}
}

type failingProvider struct{}

func (failingProvider) Name() string { return "failing" }
func (failingProvider) Complete(context.Context, agent.CompletionRequest) (*agent.CompletionResponse, error) {
	return nil, errors.New("simulated provider failure")
}
