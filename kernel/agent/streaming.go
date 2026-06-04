// SPDX-License-Identifier: MIT

package agent

import "context"

// StreamingProvider is the optional sibling of Provider for adapters
// that can emit partial output as the model generates it. Providers
// MUST implement Provider; implementing StreamingProvider on top is
// additive — callers type-assert to check.
//
//	if sp, ok := prov.(agent.StreamingProvider); ok {
//	    resp, err := sp.CompleteStream(ctx, req, onChunk)
//	} else {
//	    resp, err := prov.Complete(ctx, req)
//	}
//
// CompleteStream MUST return the same CompletionResponse that Complete
// would have returned for the same request — token usage, final
// assembled message, stop reason. The onChunk callback is for
// progressive UI; the canonical answer still flows through the
// returned response. This invariant lets callers safely fall back to
// Complete-style accounting (Governor pricing, journal usage stats)
// without worrying about partial state.
//
// Cancellation: ctx.Err() cancels the underlying HTTP stream. The
// onChunk callback may return an error to abort the stream early; in
// that case CompleteStream returns (nil, that error) and the
// provider is responsible for closing the HTTP connection.
type StreamingProvider interface {
	Provider
	// CompleteStream is like Complete but invokes onChunk for each
	// incremental piece of output as it arrives. onChunk MUST NOT
	// block for long — providers typically call it on the read
	// goroutine. Returning a non-nil error from onChunk aborts the
	// stream.
	CompleteStream(ctx context.Context, req CompletionRequest, onChunk func(Chunk) error) (*CompletionResponse, error)
}

// Chunk is one streamed unit. Exactly one of TextDelta /
// ToolUseStart / ToolInputJSONDelta / ToolUseStop is populated per
// chunk; the others are zero. Providers SHOULD coalesce small text
// fragments where convenient — operators want progress visibility,
// not literal one-byte-per-event.
type Chunk struct {
	// TextDelta is the next slice of assistant text. Concatenating
	// all TextDelta values across a stream reconstructs the final
	// text. Empty when this chunk carries a tool event instead.
	TextDelta string

	// ToolUseStart announces a new tool call starting. The ID +
	// Name are final; ToolInputJSONDelta chunks for this call will
	// follow until ToolUseStop. UIs typically render
	// "→ calling tool X..." at this point.
	ToolUseStart *ToolCall

	// ToolInputJSONDelta is the next slice of streamed JSON input
	// for the currently-open tool call. Anthropic and OpenAI both
	// stream tool inputs as incremental JSON fragments rather than
	// complete objects. Concatenating all deltas for one ID yields
	// the final input JSON.
	ToolInputJSONDelta string

	// ToolUseStop signals the currently-streaming tool call's
	// input is complete. ID identifies which call; UIs can finalize
	// any per-call progress widget here.
	ToolUseStop string

	// ReasoningDelta is the next slice of the model's reasoning / chain of
	// thought (M317), for reasoning models that stream it separately from the
	// answer (DeepSeek-R1 and compatible models' `reasoning_content`).
	// Concatenating all ReasoningDelta values reconstructs the full reasoning.
	// Empty for non-reasoning models and for tool/text chunks.
	ReasoningDelta string
}

// IsEmpty reports whether the chunk carries no signal. Stream
// implementations may emit periodic keep-alives that decode to
// empty Chunks; callers can use this to skip them.
func (c Chunk) IsEmpty() bool {
	return c.TextDelta == "" &&
		c.ToolUseStart == nil &&
		c.ToolInputJSONDelta == "" &&
		c.ToolUseStop == ""
}
