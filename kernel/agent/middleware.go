// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"strings"
)

// CompleteFunc is the non-streaming provider call as a plain function, so
// middleware can wrap it. It mirrors Provider.Complete.
type CompleteFunc func(context.Context, CompletionRequest) (*CompletionResponse, error)

// StreamFunc is the streaming provider call as a plain function. It mirrors
// StreamingProvider.CompleteStream.
type StreamFunc func(context.Context, CompletionRequest, func(Chunk) error) (*CompletionResponse, error)

// Middleware is a composable wrapper around a provider call (M997), analogous to
// the Vercel AI SDK's wrapLanguageModel middleware. Every field is optional; a
// zero Middleware is a no-op. Middlewares are applied outermost-first: the first
// in the list to Wrap sees the request first and the response last.
//
//   - TransformRequest mutates the request before it reaches the provider
//     (e.g. inject default sampling params).
//   - WrapComplete wraps the non-streaming call (logging, caching, output
//     rewriting such as reasoning extraction).
//   - WrapStream wraps the streaming call.
//   - SynthesizeStream lets a non-streaming provider be presented as a streaming
//     one: when set and the underlying provider has no native CompleteStream,
//     Wrap manufactures a stream that emits the final answer as a single chunk.
type Middleware struct {
	Name             string
	TransformRequest func(*CompletionRequest)
	WrapComplete     func(next CompleteFunc) CompleteFunc
	WrapStream       func(next StreamFunc) StreamFunc
	SynthesizeStream bool
}

// Wrap composes mws around p and returns a new Provider. The result implements
// StreamingProvider when p does, or when any middleware sets SynthesizeStream;
// otherwise it is a plain Provider (preserving p's streaming posture by
// default). An empty mws list returns p unchanged.
func Wrap(p Provider, mws ...Middleware) Provider {
	if len(mws) == 0 {
		return p
	}

	// Combined request transform, applied in list order before the call.
	transform := func(req *CompletionRequest) {
		for _, m := range mws {
			if m.TransformRequest != nil {
				m.TransformRequest(req)
			}
		}
	}

	// Fold WrapComplete from innermost (the provider) outward. Iterating the
	// list in reverse makes mws[0] the outermost wrapper.
	complete := CompleteFunc(p.Complete)
	for i := len(mws) - 1; i >= 0; i-- {
		if mws[i].WrapComplete != nil {
			complete = mws[i].WrapComplete(complete)
		}
	}
	finalComplete := func(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
		transform(&req)
		return complete(ctx, req)
	}

	sp, native := p.(StreamingProvider)
	synth := false
	for _, m := range mws {
		if m.SynthesizeStream {
			synth = true
		}
	}

	base := &wrapped{name: p.Name(), complete: finalComplete}
	if !native && !synth {
		return base
	}

	// Base stream: native when available, else synthesized from the (already
	// middleware-wrapped) complete path so transforms/logging still apply.
	var baseStream StreamFunc
	if native {
		baseStream = sp.CompleteStream
	} else {
		baseStream = synthesizeStream(finalComplete)
	}
	stream := baseStream
	for i := len(mws) - 1; i >= 0; i-- {
		if mws[i].WrapStream != nil {
			stream = mws[i].WrapStream(stream)
		}
	}
	finalStream := func(ctx context.Context, req CompletionRequest, onChunk func(Chunk) error) (*CompletionResponse, error) {
		transform(&req)
		return stream(ctx, req, onChunk)
	}
	return &streamWrapped{wrapped: *base, stream: finalStream}
}

type wrapped struct {
	name     string
	complete CompleteFunc
}

func (w *wrapped) Name() string { return w.name }
func (w *wrapped) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	return w.complete(ctx, req)
}

type streamWrapped struct {
	wrapped
	stream StreamFunc
}

func (w *streamWrapped) CompleteStream(ctx context.Context, req CompletionRequest, onChunk func(Chunk) error) (*CompletionResponse, error) {
	return w.stream(ctx, req, onChunk)
}

// synthesizeStream turns a non-streaming call into a streaming one by emitting
// the final reasoning + answer as one chunk each, then returning the response.
func synthesizeStream(complete CompleteFunc) StreamFunc {
	return func(ctx context.Context, req CompletionRequest, onChunk func(Chunk) error) (*CompletionResponse, error) {
		resp, err := complete(ctx, req)
		if err != nil {
			return nil, err
		}
		if resp.ReasoningContent != "" {
			if cerr := onChunk(Chunk{ReasoningDelta: resp.ReasoningContent}); cerr != nil {
				return nil, cerr
			}
		}
		if resp.Message.Content != "" {
			if cerr := onChunk(Chunk{TextDelta: resp.Message.Content}); cerr != nil {
				return nil, cerr
			}
		}
		for _, tc := range resp.Message.ToolCalls {
			call := tc
			if cerr := onChunk(Chunk{ToolUseStart: &call}); cerr != nil {
				return nil, cerr
			}
			if cerr := onChunk(Chunk{ToolUseStop: call.ID}); cerr != nil {
				return nil, cerr
			}
		}
		return resp, nil
	}
}

// ----- built-in middlewares -----

// ExtractReasoningMiddleware moves inline reasoning wrapped in openTag..closeTag
// (e.g. "<think>"/"</think>") out of the answer text into
// CompletionResponse.ReasoningContent, for models that emit chain-of-thought
// inline rather than in a dedicated field (M997; DeepSeek-R1 style via Ollama /
// OpenAI-compatible gateways). It only rewrites the final non-streaming response
// and the assembled response of a stream — streamed chunks pass through
// unchanged, so the tag text may still flash in a live view.
func ExtractReasoningMiddleware(openTag, closeTag string) Middleware {
	rewrite := func(resp *CompletionResponse) {
		if resp == nil || resp.ReasoningContent != "" {
			return
		}
		reasoning, answer, ok := splitTagged(resp.Message.Content, openTag, closeTag)
		if !ok {
			return
		}
		resp.ReasoningContent = reasoning
		resp.Message.Content = answer
	}
	return Middleware{
		Name: "extract-reasoning",
		WrapComplete: func(next CompleteFunc) CompleteFunc {
			return func(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
				resp, err := next(ctx, req)
				if err == nil {
					rewrite(resp)
				}
				return resp, err
			}
		},
		WrapStream: func(next StreamFunc) StreamFunc {
			return func(ctx context.Context, req CompletionRequest, onChunk func(Chunk) error) (*CompletionResponse, error) {
				resp, err := next(ctx, req, onChunk)
				if err == nil {
					rewrite(resp)
				}
				return resp, err
			}
		},
	}
}

// splitTagged extracts the text inside the first open..close tag pair and
// returns (inside, remainder-with-tag-removed, true). ok=false when the pair is
// absent.
func splitTagged(s, open, closeTag string) (inside, rest string, ok bool) {
	if open == "" || closeTag == "" {
		return "", "", false
	}
	i := strings.Index(s, open)
	if i < 0 {
		return "", "", false
	}
	j := strings.Index(s[i+len(open):], closeTag)
	if j < 0 {
		return "", "", false
	}
	start := i + len(open)
	inside = strings.TrimSpace(s[start : start+j])
	rest = strings.TrimSpace(s[:i] + s[start+j+len(closeTag):])
	return inside, rest, true
}

// SimulateStreamingMiddleware presents a non-streaming provider as a streaming
// one (the answer arrives as a single chunk). It is a no-op for providers that
// already stream natively.
func SimulateStreamingMiddleware() Middleware {
	return Middleware{Name: "simulate-streaming", SynthesizeStream: true}
}

// DefaultParamsMiddleware fills in sampling params and provider options that the
// caller left unset, without overriding any explicitly-set value. It is the
// composable home for per-provider generation defaults.
func DefaultParamsMiddleware(defaults Params) Middleware {
	return Middleware{
		Name: "default-params",
		TransformRequest: func(req *CompletionRequest) {
			p := &req.Params
			if p.Temperature == nil {
				p.Temperature = defaults.Temperature
			}
			if p.TopP == nil {
				p.TopP = defaults.TopP
			}
			if p.TopK == nil {
				p.TopK = defaults.TopK
			}
			if p.Seed == nil {
				p.Seed = defaults.Seed
			}
			if p.FrequencyPenalty == nil {
				p.FrequencyPenalty = defaults.FrequencyPenalty
			}
			if p.PresencePenalty == nil {
				p.PresencePenalty = defaults.PresencePenalty
			}
			if len(p.Stop) == 0 {
				p.Stop = defaults.Stop
			}
			if p.ReasoningEffort == "" {
				p.ReasoningEffort = defaults.ReasoningEffort
			}
		},
	}
}
