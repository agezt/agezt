// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// scriptedProvider returns a queued sequence of assistant contents, one per
// Complete call, so a test can drive the repair loop deterministically.
type scriptedProvider struct {
	replies []string
	calls   int
}

func (s *scriptedProvider) Name() string { return "scripted" }
func (s *scriptedProvider) Complete(_ context.Context, _ CompletionRequest) (*CompletionResponse, error) {
	i := s.calls
	s.calls++
	if i >= len(s.replies) {
		i = len(s.replies) - 1
	}
	return &CompletionResponse{
		Message: Message{Role: RoleAssistant, Content: s.replies[i]},
		Usage:   Usage{InputTokens: 10, OutputTokens: 5, Model: "scripted"},
	}, nil
}

var personSchema = json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"age":{"type":"integer"}},"required":["name","age"],"additionalProperties":false}`)

func TestGenerateObject_Success(t *testing.T) {
	p := &scriptedProvider{replies: []string{`{"name":"Ada","age":36}`}}
	var got struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	u, err := GenerateObject(context.Background(), p, CompletionRequest{}, personSchema, &got)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Ada" || got.Age != 36 {
		t.Fatalf("decoded wrong: %+v", got)
	}
	if p.calls != 1 {
		t.Fatalf("expected 1 call, got %d", p.calls)
	}
	if u.OutputTokens != 5 {
		t.Fatalf("usage not summed: %+v", u)
	}
}

func TestGenerateObject_FenceAndProse(t *testing.T) {
	p := &scriptedProvider{replies: []string{"Here you go:\n```json\n{\"name\":\"Bo\",\"age\":7}\n```\nHope that helps!"}}
	var got map[string]any
	if _, err := GenerateObject(context.Background(), p, CompletionRequest{}, personSchema, &got); err != nil {
		t.Fatal(err)
	}
	if got["name"] != "Bo" {
		t.Fatalf("fence/prose extraction failed: %v", got)
	}
}

func TestGenerateObject_Repair(t *testing.T) {
	// First reply violates the schema (age is a string), second is valid.
	p := &scriptedProvider{replies: []string{
		`{"name":"X","age":"oops"}`,
		`{"name":"X","age":40}`,
	}}
	var got struct {
		Age int `json:"age"`
	}
	u, err := GenerateObject(context.Background(), p, CompletionRequest{}, personSchema, &got)
	if err != nil {
		t.Fatal(err)
	}
	if got.Age != 40 {
		t.Fatalf("repair failed: %+v", got)
	}
	if p.calls != 2 {
		t.Fatalf("expected 2 calls (one repair), got %d", p.calls)
	}
	if u.OutputTokens != 10 {
		t.Fatalf("usage should sum both attempts: %+v", u)
	}
}

func TestGenerateObject_GivesUp(t *testing.T) {
	p := &scriptedProvider{replies: []string{`not json at all`}}
	var got map[string]any
	_, err := GenerateObject(context.Background(), p, CompletionRequest{}, personSchema, &got)
	if !errors.Is(err, ErrNoObjectGenerated) {
		t.Fatalf("expected ErrNoObjectGenerated, got %v", err)
	}
	var oe *ObjectError
	if !errors.As(err, &oe) {
		t.Fatalf("expected *ObjectError, got %T", err)
	}
	if oe.LastText == "" {
		t.Fatal("ObjectError should carry last text")
	}
	// 1 initial + DefaultObjectRepairs retries.
	if p.calls != 1+DefaultObjectRepairs {
		t.Fatalf("expected %d calls, got %d", 1+DefaultObjectRepairs, p.calls)
	}
}
