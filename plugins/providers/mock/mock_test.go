// SPDX-License-Identifier: MIT

package mock

import (
	"context"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
)

func TestName(t *testing.T) {
	p := New()
	if n := p.Name(); n != "mock" {
		t.Errorf("Name = %q, want %q", n, "mock")
	}
}

func TestNew_CopiesInput(t *testing.T) {
	original := []llm.CompletionResponse{{StopReason: llm.StopEndTurn}}
	p := New(original...)
	original[0] = llm.CompletionResponse{} // mutate original
	if p.responses[0].StopReason != llm.StopEndTurn {
		t.Error("New did not copy the input slice")
	}
}

func TestComplete_ReturnsResponsesInOrder(t *testing.T) {
	p := New(
		llm.CompletionResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "first"}},
		llm.CompletionResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "second"}},
	)
	r1, err := p.Complete(context.Background(), llm.CompletionRequest{})
	if err != nil {
		t.Fatalf("first Complete: %v", err)
	}
	if r1.Message.Content != "first" {
		t.Errorf("first response = %q, want %q", r1.Message.Content, "first")
	}
	r2, _ := p.Complete(context.Background(), llm.CompletionRequest{})
	if r2.Message.Content != "second" {
		t.Errorf("second response = %q, want %q", r2.Message.Content, "second")
	}
}

func TestComplete_Exhausted(t *testing.T) {
	p := New(llm.CompletionResponse{StopReason: llm.StopEndTurn})
	p.Complete(context.Background(), llm.CompletionRequest{})
	_, err := p.Complete(context.Background(), llm.CompletionRequest{})
	if !errors.Is(err, ErrExhausted) {
		t.Errorf("exhausted error = %v, want %v", err, ErrExhausted)
	}
}

func TestComplete_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := New(llm.CompletionResponse{StopReason: llm.StopEndTurn})
	_, err := p.Complete(ctx, llm.CompletionRequest{})
	if err == nil {
		t.Error("Complete with cancelled context should error")
	}
}

func TestComplete_OnRequestCalled(t *testing.T) {
	var seen llm.CompletionRequest
	p := New(llm.CompletionResponse{StopReason: llm.StopEndTurn})
	p.OnRequest = func(req llm.CompletionRequest) {
		seen = req
	}
	p.Complete(context.Background(), llm.CompletionRequest{Model: "test-model"})
	if seen.Model != "test-model" {
		t.Errorf("OnRequest received model=%q, want %q", seen.Model, "test-model")
	}
}

func TestComplete_ResponderTakesPrecedence(t *testing.T) {
	p := New(
		llm.CompletionResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "scripted"}},
	)
	p.Responder = func(req llm.CompletionRequest) llm.CompletionResponse {
		return llm.CompletionResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "responded"}}
	}
	r, _ := p.Complete(context.Background(), llm.CompletionRequest{})
	if r.Message.Content != "responded" {
		t.Errorf("Responder content = %q, want %q", r.Message.Content, "responded")
	}
}

func TestCallCount(t *testing.T) {
	p := New(llm.CompletionResponse{StopReason: llm.StopEndTurn})
	if c := p.CallCount(); c != 0 {
		t.Errorf("initial CallCount = %d, want 0", c)
	}
	p.Complete(context.Background(), llm.CompletionRequest{})
	if c := p.CallCount(); c != 1 {
		t.Errorf("after one Complete, CallCount = %d, want 1", c)
	}
}

func TestCallCount_ThreadSafe(t *testing.T) {
	p := New(llm.CompletionResponse{StopReason: llm.StopEndTurn})
	done := make(chan struct{})
	go func() {
		p.Complete(context.Background(), llm.CompletionRequest{})
		close(done)
	}()
	// Concurrent call to CallCount should not race.
	p.CallCount()
	<-done
}

func TestFinalText(t *testing.T) {
	r := FinalText("hello world")
	if r.Message.Role != llm.RoleAssistant {
		t.Errorf("FinalText role = %q, want %q", r.Message.Role, llm.RoleAssistant)
	}
	if r.Message.Content != "hello world" {
		t.Errorf("FinalText content = %q, want %q", r.Message.Content, "hello world")
	}
	if r.StopReason != llm.StopEndTurn {
		t.Errorf("FinalText StopReason = %q, want %q", r.StopReason, llm.StopEndTurn)
	}
}
