package reranktool

import (
	"context"
	"encoding/json"
	"testing"
)

// badReranker is a test stub that simulates a misbehaving or adversarial
// reranker returning indices outside the document range.
type badReranker struct {
	indices []int
	scores  []float64
}

func (b *badReranker) Rerank(_ context.Context, _ string, docs []string, _ int) ([]int, []float64, error) {
	return b.indices, b.scores, nil
}

func (b *badReranker) HasRerank() bool { return true }

// goodReranker is a well-behaved stub for positive-coverage tests.
type goodReranker struct {
	indices []int
	scores  []float64
}

func (g *goodReranker) Rerank(_ context.Context, _ string, _ []string, _ int) ([]int, []float64, error) {
	return g.indices, g.scores, nil
}

func (g *goodReranker) HasRerank() bool { return true }

// TestInvoke_OutOfRangeIndexReturnsError_NotPanic verifies that a reranker
// returning an out-of-bounds document index does not cause a panic — it must
// return an error Result instead.
func TestInvoke_OutOfRangeIndexReturnsError_NotPanic(t *testing.T) {
	tl := New(&badReranker{
		indices: []int{len([]string{"doc0", "doc1"}) + 99}, // index 101 for 2 docs
		scores:  []float64{0.9},
	})
	raw, _ := json.Marshal(map[string]any{
		"query":     "test",
		"documents": []string{"doc0", "doc1"},
		"top_n":     1,
	})
	// Must not panic; must return an error result.
	result, err := tl.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke must not return a Go error wrapping a panic: %v", err)
	}
	if !result.IsError {
		t.Fatalf("out-of-range index from reranker produced no error result: output len=%d", len(result.Output))
	}
}

// TestInvoke_GoodReranker_PassesThrough verifies the normal path where the
// reranker returns valid indices.
func TestInvoke_GoodReranker_PassesThrough(t *testing.T) {
	tl := New(&goodReranker{
		indices: []int{1, 0},
		scores:  []float64{0.95, 0.8},
	})
	raw, _ := json.Marshal(map[string]any{
		"query":     "test query",
		"documents": []string{"first doc", "second doc"},
		"top_n":     2,
	})
	result, err := tl.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Output)
	}
	// Output must be valid JSON array of rankedItem.
	var items []map[string]any
	if err := json.Unmarshal([]byte(result.Output), &items); err != nil {
		t.Fatalf("output is not valid JSON: %s", result.Output)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 ranked items, got %d", len(items))
	}
}

// TestInvoke_EmptyDocuments returns an error.
func TestInvoke_EmptyDocuments(t *testing.T) {
	tl := New(&goodReranker{})
	raw, _ := json.Marshal(map[string]any{
		"query":     "test",
		"documents": []string{},
	})
	result, err := tl.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !result.IsError {
		t.Fatalf("want error for empty documents, got output len=%d", len(result.Output))
	}
}

// TestInvoke_BlankQuery returns an error.
func TestInvoke_BlankQuery(t *testing.T) {
	tl := New(&goodReranker{})
	raw, _ := json.Marshal(map[string]any{
		"query":     "   ",
		"documents": []string{"doc"},
	})
	result, err := tl.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !result.IsError {
		t.Fatalf("want error for blank query, got output len=%d", len(result.Output))
	}
}

// TestInvoke_NotConfigured returns an error without calling the reranker.
func TestInvoke_NotConfigured(t *testing.T) {
	tl := New(nil)
	raw, _ := json.Marshal(map[string]any{
		"query":     "test",
		"documents": []string{"doc"},
	})
	result, err := tl.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !result.IsError {
		t.Fatalf("want error when not configured, got output len=%d", len(result.Output))
	}
}

// TestInvoke_Definition smoke-tests the tool definition.
func TestInvoke_Definition(t *testing.T) {
	tl := New(&goodReranker{})
	def := tl.Definition()
	if def.Name != "rerank" {
		t.Errorf("Name = %q, want rerank", def.Name)
	}
	if def.InputSchema == nil {
		t.Error("InputSchema is nil")
	}
}
