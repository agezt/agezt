// SPDX-License-Identifier: MIT

package compat

// Provider-name wrapping helpers: FirstModelID + namedProvider +
// namedStreamingProvider + wrapNamed. Carved out of compat.go during
// the Day 183 god-file split so the main file can stay focused on
// error vars + CredLookup + Build + family dispatch.
// Public API unchanged.

import (
	"context"

	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/contract/llm"
)

func FirstModelID(p *catalog.Provider) string {
	if p == nil || len(p.Models) == 0 {
		return ""
	}
	best := ""
	for id := range p.Models {
		if best == "" || id < best {
			best = id
		}
	}
	return best
}

// namedProvider wraps an inner llm.Provider so the Name() it reports
// matches the catalog provider id instead of the wire-family default
// ("anthropic", "ollama"). The Governor's registry is keyed on Name();
// keeping it aligned with the catalog id is what lets `agt catalog
// list` and the daemon's logs use the same identifier.
type namedProvider struct {
	name  string
	inner llm.Provider
}

func (n *namedProvider) Name() string { return n.name }
func (n *namedProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return n.inner.Complete(ctx, req)
}

// namedStreamingProvider is the streaming-aware variant of
// namedProvider. It's returned by wrapNamed when the inner provider
// implements llm.StreamingProvider, so type-asserting on the
// wrapped value preserves the inner's streaming capability.
//
// This is split into a sibling type rather than always implementing
// StreamingProvider on namedProvider because Go's interface
// satisfaction is structural — if namedProvider always had a
// CompleteStream method, every caller would see it as a
// StreamingProvider even when the inner doesn't support streaming.
// Two types lets the type assertion at the call site mean exactly
// what it says.
type namedStreamingProvider struct {
	namedProvider
	streamingInner llm.StreamingProvider
}

func (n *namedStreamingProvider) CompleteStream(ctx context.Context, req llm.CompletionRequest, onChunk func(llm.Chunk) error) (*llm.CompletionResponse, error) {
	return n.streamingInner.CompleteStream(ctx, req, onChunk)
}

// wrapNamed returns a wrapper that preserves the inner provider's
// capabilities. Always implements llm.Provider; additionally
// implements llm.StreamingProvider if the inner does.
func wrapNamed(name string, p llm.Provider) llm.Provider {
	if sp, ok := p.(llm.StreamingProvider); ok {
		return &namedStreamingProvider{
			namedProvider:  namedProvider{name: name, inner: p},
			streamingInner: sp,
		}
	}
	return &namedProvider{name: name, inner: p}
}
