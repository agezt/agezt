// SPDX-License-Identifier: MIT

package mock

import (
	"context"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestName(t *testing.T) {
	p := New()
	if n := p.Name(); n != "mock" {
		t.Errorf("Name = %q, want %q", n, "mock")
	}
}

func TestNew_CopiesInput(t *testing.T) {
	original := []agent.CompletionResponse{{StopReason: agent.StopEndTurn}}
	p := New(original...)
	original[0] = agent.CompletionResponse{} // mutate original
	if p.responses[0].StopReason != agent.StopEndTurn {
		t.Error("New did not copy the input slice")
	}
}

func TestComplete_ReturnsResponsesInOrder(t *testing.T) {
	p := New(
		agent.CompletionResponse{Message: agent.Message{Role: agent.RoleAssistant, Content: "first"}},
		agent.CompletionResponse{Message: agent.Message{Role: agent.RoleAssistant, Content: "second"}},
	)
	r1, err := p.Complete(context.Background(), agent.CompletionRequest{})
	if err != nil {
		t.Fatalf("first Complete: %v", err)
	}
	if r1.Message.Content != "first" {
		t.Errorf("first response = %q, want %q", r1.Message.Content, "first")
	}
	r2, _ := p.Complete(context.Background(), agent.CompletionRequest{})
	if r2.Message.Content != "second" {
		t.Errorf("second response = %q, want %q", r2.Message.Content, "second")
	}
}

func TestComplete_Exhausted(t *testing.T) {
	p := New(agent.CompletionResponse{StopReason: agent.StopEndTurn})
	p.Complete(context.Background(), agent.CompletionRequest{})
	_, err := p.Complete(context.Background(), agent.CompletionRequest{})
	if !errors.Is(err, ErrExhausted) {
		t.Errorf("exhausted error = %v, want %v", err, ErrExhausted)
	}
}

func TestComplete_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := New(agent.CompletionResponse{StopReason: agent.StopEndTurn})
	_, err := p.Complete(ctx, agent.CompletionRequest{})
	if err == nil {
		t.Error("Complete with cancelled context should error")
	}
}

func TestComplete_OnRequestCalled(t *testing.T) {
	var seen agent.CompletionRequest
	p := New(agent.CompletionResponse{StopReason: agent.StopEndTurn})
	p.OnRequest = func(req agent.CompletionRequest) {
		seen = req
	}
	p.Complete(context.Background(), agent.CompletionRequest{Model: "test-model"})
	if seen.Model != "test-model" {
		t.Errorf("OnRequest received model=%q, want %q", seen.Model, "test-model")
	}
}

func TestComplete_ResponderTakesPrecedence(t *testing.T) {
	p := New(
		agent.CompletionResponse{Message: agent.Message{Role: agent.RoleAssistant, Content: "scripted"}},
	)
	p.Responder = func(req agent.CompletionRequest) agent.CompletionResponse {
		return agent.CompletionResponse{Message: agent.Message{Role: agent.RoleAssistant, Content: "responded"}}
	}
	r, _ := p.Complete(context.Background(), agent.CompletionRequest{})
	if r.Message.Content != "responded" {
		t.Errorf("Responder content = %q, want %q", r.Message.Content, "responded")
	}
}

func TestCallCount(t *testing.T) {
	p := New(agent.CompletionResponse{StopReason: agent.StopEndTurn})
	if c := p.CallCount(); c != 0 {
		t.Errorf("initial CallCount = %d, want 0", c)
	}
	p.Complete(context.Background(), agent.CompletionRequest{})
	if c := p.CallCount(); c != 1 {
		t.Errorf("after one Complete, CallCount = %d, want 1", c)
	}
}

func TestCallCount_ThreadSafe(t *testing.T) {
	p := New(agent.CompletionResponse{StopReason: agent.StopEndTurn})
	done := make(chan struct{})
	go func() {
		p.Complete(context.Background(), agent.CompletionRequest{})
		close(done)
	}()
	// Concurrent call to CallCount should not race.
	p.CallCount()
	<-done
}

func TestFinalText(t *testing.T) {
	r := FinalText("hello world")
	if r.Message.Role != agent.RoleAssistant {
		t.Errorf("FinalText role = %q, want %q", r.Message.Role, agent.RoleAssistant)
	}
	if r.Message.Content != "hello world" {
		t.Errorf("FinalText content = %q, want %q", r.Message.Content, "hello world")
	}
	if r.StopReason != agent.StopEndTurn {
		t.Errorf("FinalText StopReason = %q, want %q", r.StopReason, agent.StopEndTurn)
	}
}
