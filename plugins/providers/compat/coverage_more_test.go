// SPDX-License-Identifier: MIT

package compat

import (
	"context"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
)

func TestCompatCoverageLookupHelpers(t *testing.T) {
	if got := envLookup(nil, "anything"); got != "" {
		t.Fatalf("nil lookup = %q, want empty", got)
	}
	lookup := func(name string) string {
		switch name {
		case "FIRST":
			return "1"
		case "SECOND":
			return "2"
		}
		return ""
	}
	if got := envLookup(lookup, "first"); got != "" {
		t.Fatalf("missing key = %q", got)
	}
	if got := envLookup(lookup, "FIRST"); got != "1" {
		t.Fatalf("first = %q", got)
	}

	provider := &catalog.Provider{ID: "p", Env: []string{"FIRST", "SECOND"}}
	if got := providerEnvLookup(provider, nil, "x"); got != "" {
		t.Fatalf("nil lookup = %q", got)
	}
	if got := providerEnvLookup(provider, lookup, "third"); got != "" {
		t.Fatalf("no match = %q", got)
	}
	if got := providerEnvLookup(provider, lookup, "FIRST"); got != "1" {
		t.Fatalf("first lookup = %q", got)
	}
}

func TestCompatCoverageBaseURLHelpers(t *testing.T) {
	if got := compatVendorBaseURL("@ai-sdk/groq"); got != "https://api.groq.com/openai/v1" {
		t.Fatalf("groq base = %q", got)
	}
	if got := compatVendorBaseURL("@openrouter/ai-sdk-provider"); got != "https://openrouter.ai/api/v1" {
		t.Fatalf("openrouter base = %q", got)
	}
	if got := compatVendorBaseURL("@ai-sdk/openai-compatible"); got != "" {
		t.Fatalf("unknown vendor = %q, want empty", got)
	}
	if got := defaultBaseURL(catalog.FamilyAnthropic); got != "https://api.anthropic.com/v1" {
		t.Fatalf("anthropic base = %q", got)
	}
	if got := defaultBaseURL(catalog.FamilyOpenAICompatible); got != "" {
		t.Fatalf("openai-compatible default = %q, want empty", got)
	}
	if got := defaultBaseURL(catalog.FamilyAWSBedrock); got != "" {
		t.Fatalf("bedrock default = %q, want empty", got)
	}
}

func TestCompatCoverageBuildUnknownFamily(t *testing.T) {
	if _, _, err := Build(nil, "m", nil); err == nil || err.Error() == "" {
		t.Fatal("nil entry must error")
	}
	if _, _, err := Build(&catalog.Provider{ID: "x", NPM: "@ai-sdk/openai-compatible", Env: []string{"X_API_KEY"}, Models: map[string]*catalog.Model{"x": {ID: "x"}}}, "", nil); err == nil {
		t.Fatal("empty model id must error")
	}
	// Unknown npm package → ErrFamilyUnsupported (or wrapped).
	if _, _, err := Build(&catalog.Provider{ID: "x", NPM: "@ai-sdk/unknown", Models: map[string]*catalog.Model{"x": {ID: "x"}}}, "x", nil); err == nil {
		t.Fatal("unknown npm must error")
	}
}

func TestCompatCoverageWrapNamedAndStreaming(t *testing.T) {
	np := wrapNamed("alias", fakeProvider{name: "alias", reply: "ok"})
	if got := np.Name(); got != "alias" {
		t.Fatalf("named provider Name = %q", got)
	}
	resp, err := np.Complete(context.Background(), agent.CompletionRequest{Model: "m", Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}}})
	if err != nil || resp.Message.Content != "ok" {
		t.Fatalf("named complete = %+v err %v", resp, err)
	}

	sp := wrapNamed("streaming", fakeStreamingProvider{outer: fakeProvider{name: "streaming", reply: "streamed"}})
	if _, ok := sp.(agent.StreamingProvider); !ok {
		t.Fatal("expected streaming wrapper to satisfy StreamingProvider")
	}
	// Drive the streaming surface; should return the canned reply.
	streamed, err := sp.(agent.StreamingProvider).CompleteStream(context.Background(), agent.CompletionRequest{Model: "m", Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}}}, func(agent.Chunk) error { return nil })
	if err != nil || streamed.Message.Content != "streamed" {
		t.Fatalf("streamed complete = %+v err %v", streamed, err)
	}
}

type fakeProvider struct {
	name  string
	reply string
}

func (f fakeProvider) Name() string { return f.name }

func (f fakeProvider) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	return &agent.CompletionResponse{Message: agent.Message{Role: agent.RoleAssistant, Content: f.reply}}, nil
}

type fakeStreamingProvider struct {
	outer fakeProvider
}

func (f fakeStreamingProvider) Name() string { return f.outer.name }

func (f fakeStreamingProvider) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	return f.outer.Complete(ctx, req)
}

func (f fakeStreamingProvider) CompleteStream(ctx context.Context, req agent.CompletionRequest, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	if err := onChunk(agent.Chunk{TextDelta: "x"}); err != nil {
		return nil, err
	}
	return &agent.CompletionResponse{Message: agent.Message{Role: agent.RoleAssistant, Content: f.outer.reply}}, nil
}

var _ = errors.New
