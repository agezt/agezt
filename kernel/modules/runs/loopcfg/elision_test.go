// SPDX-License-Identifier: MIT

package loopcfg

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
)

// TestMakeElidedSummarizer_CapsTheInput covers the bounded input: however
// large the dropped output was, the summarizer sees at most the cap — its own
// cost does not grow with the output's size.
func TestMakeElidedSummarizer_CapsTheInput(t *testing.T) {
	prov := &fakeProvider{}
	sum := makeElidedSummarizer(prov, "fake-model", "corr-1", elidedSummaryMaxTokens)
	huge := strings.Repeat("x", elidedSummaryInputCap*3)
	if _, err := sum(context.Background(), huge); err != nil {
		t.Fatal(err)
	}
	if len(prov.last.Messages) != 1 {
		t.Fatalf("summarizer sent %d messages, want 1", len(prov.last.Messages))
	}
	content := prov.last.Messages[0].Content
	if len(content) > elidedSummaryInputCap+512 {
		t.Fatalf("the summarizer's input is %d bytes, want at most the cap", len(content))
	}
}

// TestMakeElidedSummarizer_TrimsTheSummary covers the returned line: it is
// trimmed, so the loop's working memory carries no stray whitespace.
func TestMakeElidedSummarizer_TrimsTheSummary(t *testing.T) {
	prov := &fakeProvider{}
	sum := makeElidedSummarizer(prov, "fake-model", "corr-1", elidedSummaryMaxTokens)
	out, err := sum(context.Background(), "output")
	if err != nil {
		t.Fatal(err)
	}
	if out != "a summary" {
		t.Fatalf("summary = %q, want %q", out, "a summary")
	}
}

// TestMakeElidedSummarizer_PropagatesTheProviderError covers the error path:
// the provider's failure is the loop's to swallow, not the summarizer's to
// hide.
func TestMakeElidedSummarizer_PropagatesTheProviderError(t *testing.T) {
	sum := makeElidedSummarizer(failingProvider{}, "fake-model", "corr-1", elidedSummaryMaxTokens)
	if _, err := sum(context.Background(), "output"); err == nil {
		t.Fatal("the provider's error was swallowed")
	}
}

// failingProvider always fails.
type failingProvider struct{}

func (failingProvider) Name() string { return "failing" }

func (failingProvider) Complete(context.Context, llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return nil, errProvider
}

var errProvider = &providerError{}

type providerError struct{}

func (*providerError) Error() string { return "provider is down" }
