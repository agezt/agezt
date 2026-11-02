// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
)

// recordingProvider captures the request it received and returns a fixed reply.
type recordingProvider struct {
	got   llm.CompletionRequest
	reply llm.CompletionResponse
}

func (r *recordingProvider) Name() string { return "rec" }
func (r *recordingProvider) Complete(_ context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	r.got = req
	resp := r.reply
	return &resp, nil
}

func TestWrap_NoMiddlewareReturnsSame(t *testing.T) {
	p := &recordingProvider{}
	if Wrap(p) != llm.Provider(p) {
		t.Fatal("Wrap with no middleware should return the provider unchanged")
	}
}

func TestWrap_NonStreamingStaysNonStreaming(t *testing.T) {
	p := &recordingProvider{}
	w := Wrap(p, Middleware{
		Name: "test",
		WrapComplete: func(next CompleteFunc) CompleteFunc {
			return func(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
				return next(ctx, req)
			}
		},
	})
	if _, ok := w.(llm.StreamingProvider); ok {
		t.Fatal("wrapping a non-streaming provider should not advertise streaming")
	}
}

func TestDefaultParamsMiddleware_FillsOnlyUnset(t *testing.T) {
	p := &recordingProvider{}
	temp := 0.7
	w := Wrap(p, DefaultParamsMiddleware(llm.Params{Temperature: &temp, ReasoningEffort: "low"}))

	// Caller set ReasoningEffort explicitly; default must not override it.
	_, _ = w.Complete(context.Background(), llm.CompletionRequest{Params: llm.Params{ReasoningEffort: "high"}})
	if p.got.Params.Temperature == nil || *p.got.Params.Temperature != 0.7 {
		t.Fatalf("default temperature not applied: %+v", p.got.Params)
	}
	if p.got.Params.ReasoningEffort != "high" {
		t.Fatalf("explicit ReasoningEffort overridden: %q", p.got.Params.ReasoningEffort)
	}
}

func TestExtractReasoningMiddleware(t *testing.T) {
	p := &recordingProvider{reply: llm.CompletionResponse{
		Message: llm.Message{Role: llm.RoleAssistant, Content: "<think>step one\nstep two</think>The answer is 42."},
	}}
	w := Wrap(p, ExtractReasoningMiddleware("<think>", "</think>"))
	resp, err := w.Complete(context.Background(), llm.CompletionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ReasoningContent != "step one\nstep two" {
		t.Fatalf("reasoning not extracted: %q", resp.ReasoningContent)
	}
	if resp.Message.Content != "The answer is 42." {
		t.Fatalf("answer not cleaned: %q", resp.Message.Content)
	}
}

func TestSimulateStreamingMiddleware(t *testing.T) {
	p := &recordingProvider{reply: llm.CompletionResponse{
		Message: llm.Message{Role: llm.RoleAssistant, Content: "hello world"},
	}}
	w := Wrap(p, SimulateStreamingMiddleware())
	sp, ok := w.(llm.StreamingProvider)
	if !ok {
		t.Fatal("SimulateStreaming should make the provider streamable")
	}
	var chunks []string
	resp, err := sp.CompleteStream(context.Background(), llm.CompletionRequest{}, func(c llm.Chunk) error {
		if c.TextDelta != "" {
			chunks = append(chunks, c.TextDelta)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 || chunks[0] != "hello world" {
		t.Fatalf("expected one synthetic chunk, got %v", chunks)
	}
	if resp.Message.Content != "hello world" {
		t.Fatalf("final response wrong: %q", resp.Message.Content)
	}
}
