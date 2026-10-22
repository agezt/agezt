// SPDX-License-Identifier: MIT

package reasoning

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
)

// serviceForTest builds a module over the smallest possible set of ports: a
// provider that is present, a completion funnel that answers "ok" and no tools.
func serviceForTest(provider bool) *Service {
	return New(Dependencies{
		HasProvider: func() bool { return provider },
		Complete: func(_ context.Context, _, _ string, _ llm.CompletionRequest) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}}, nil
		},
	})
}

// TestCouncilGuards: the panel's own errors are unchanged by the move.
func TestCouncilGuards(t *testing.T) {
	s := serviceForTest(true)
	if _, err := s.Council(context.Background(), "c", "   ", nil, 1); err == nil || !strings.Contains(err.Error(), "council: question required") {
		t.Errorf("empty question: %v", err)
	}
	if _, err := s.Council(context.Background(), "c", "why?", nil, 1); err == nil || !strings.Contains(err.Error(), "council: no members available") {
		t.Errorf("empty panel: %v", err)
	}
}

// TestConductGuards: the Conductor's own errors are unchanged by the move.
func TestConductGuards(t *testing.T) {
	s := serviceForTest(true)
	if _, err := s.Conduct(context.Background(), "c", ConductorConfig{Task: "  "}); err == nil || !strings.Contains(err.Error(), "conductor: task required") {
		t.Errorf("empty task: %v", err)
	}
	if _, err := s.Conduct(context.Background(), "c", ConductorConfig{Task: "x"}); err == nil || !strings.Contains(err.Error(), "conductor: no models available") {
		t.Errorf("no models: %v", err)
	}
}

// TestResearchGuards: the research harness refuses to start without a provider
// and without a question — the same texts the host used before the move.
func TestResearchGuards(t *testing.T) {
	noProvider := serviceForTest(false)
	if _, err := noProvider.Research(context.Background(), "c", "why?", ResearchOptions{}); err == nil || !strings.Contains(err.Error(), "research: no provider configured") {
		t.Errorf("no provider: %v", err)
	}
	withProvider := serviceForTest(true)
	if _, err := withProvider.Research(context.Background(), "c", "  ", ResearchOptions{}); err == nil || !strings.Contains(err.Error(), "research: question required") {
		t.Errorf("empty question: %v", err)
	}
}

// TestResearchWithoutToolsDegrades: with no tool invoker the harness gathers no
// sources and degrades to a note instead of failing — the same shape as a
// refused search.
func TestResearchWithoutToolsDegrades(t *testing.T) {
	s := serviceForTest(true)
	report, err := s.Research(context.Background(), "c", "why?", ResearchOptions{})
	if err != nil {
		t.Fatalf("Research: %v", err)
	}
	if len(report.Sources) != 0 {
		t.Errorf("sources = %+v", report.Sources)
	}
	if len(report.Notes) == 0 || !strings.Contains(report.Notes[0], "no sources gathered") {
		t.Errorf("notes = %+v", report.Notes)
	}
	if report.Markdown == "" {
		t.Error("a degraded run still answers")
	}
}

// TestCouncilUsesTheMembersPort: with no explicit panel the module asks the
// host for the default one.
func TestCouncilUsesTheMembersPort(t *testing.T) {
	s := New(Dependencies{
		HasProvider: func() bool { return true },
		Complete: func(_ context.Context, _, _ string, _ llm.CompletionRequest) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}}, nil
		},
		Members: func() []CouncilMember {
			return []CouncilMember{{Seat: "Solo", Model: "model-x"}}
		},
	})
	res, err := s.Council(context.Background(), "c", "why?", nil, 0)
	if err != nil {
		t.Fatalf("Council: %v", err)
	}
	if len(res.Members) != 1 || res.Members[0].Model != "model-x" {
		t.Errorf("the default panel was not used: %+v", res.Members)
	}
	// rounds<=0 means the module's default: one deliberation round on top of the
	// independent opening round — two opinions for one seat.
	if res.Rounds != 1 {
		t.Errorf("the default rounds = %d, want 1", res.Rounds)
	}
	if len(res.Opinions) != 2 {
		t.Errorf("opinions = %d, want 2 (the opening round and one deliberation)", len(res.Opinions))
	}
}
