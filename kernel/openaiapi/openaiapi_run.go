// SPDX-License-Identifier: MIT
//
// /v1/chat/completions run helpers: runCapturingReasoning (captures
// llm.reasoning deltas during a non-streaming run) + streamChat (the SSE
// streaming handler). Split from openaiapi_stream.go during Day 211
// god-file refactor (#32). Public API unchanged.
package openaiapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/httpserver"
	"github.com/agezt/agezt/kernel/ulid"
)

// runCapturingReasoning runs the model and, in parallel, accumulates the run's
// llm.reasoning deltas (M323) so the non-streaming chat response can carry
// `reasoning_content`. Mirrors streamChat's no-race subscribe-then-run pattern;
// a subscription failure degrades gracefully to a plain run (empty reasoning).

func (s *Server) runCapturingReasoning(r *http.Request, eng Engine, b *bus.Bus, corr, intent, model string, images []string, jsonMode bool) (answer, reasoning string, err error) {
	type runRes struct {
		answer string
		err    error
	}
	done := make(chan runRes, 1)

	sub, suberr := b.Subscribe(eng.SubjectForRun(corr), 1024)
	if suberr == nil {
		defer sub.Cancel()
	}
	go func() {
		a, e := eng.RunModel(r.Context(), corr, intent, model, images, jsonMode)
		done <- runRes{a, e}
	}()

	if suberr != nil {
		rr := <-done // no subscription → run without reasoning capture
		return rr.answer, "", rr.err
	}

	var rb strings.Builder
	var rr runRes
capture:
	for {
		select {
		case ev, ok := <-sub.C:
			if !ok {
				rr = <-done
				break capture
			}
			if rt := reasoningText(ev); rt != "" {
				rb.WriteString(rt)
			}
		case rr = <-done:
			// Drain any reasoning still queued before responding.
			for drained := false; !drained; {
				select {
				case ev := <-sub.C:
					if rt := reasoningText(ev); rt != "" {
						rb.WriteString(rt)
					}
				default:
					drained = true
				}
			}
			break capture
		}
	}
	return rr.answer, rb.String(), rr.err
}

// streamChat runs the intent and relays the kernel's llm.token events as
// OpenAI chat.completion.chunk SSE frames. It subscribes to the run subject
// BEFORE starting the run so no early token is missed (the same no-race pattern
// the control plane's handleRun uses).

func (s *Server) streamChat(w http.ResponseWriter, r *http.Request, eng Engine, b *bus.Bus, intent, model string, images []string, includeUsage, jsonMode bool) {
	corr := eng.NewCorrelation()
	sub, err := b.Subscribe(eng.SubjectForRun(corr), 1024)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "subscribe_error", err.Error())
		return
	}
	defer sub.Cancel()

	// StartSSE applies the process-wide per-client stream cap (V-009/LD-8).
	sse, ok := httpserver.StartSSE(w, r)
	if !ok {
		return
	}
	defer sse.Close()

	id := "chatcmpl-" + ulid.New()
	created := time.Now().Unix()
	sendChunk := func(delta map[string]any, finish any) {
		_ = sse.WriteJSON(map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finish}},
		})
	}

	var full strings.Builder // accumulates the answer for the optional usage chunk
	sendContent := func(txt string) {
		full.WriteString(txt)
		sendChunk(map[string]any{"content": txt}, nil)
	}
	// sendReasoning relays a reasoning delta as a `reasoning_content` delta (M323),
	// the DeepSeek-R1 streaming convention. It does NOT feed `full` — reasoning is
	// not the answer and must not pollute the usage chunk's content estimate.
	sendReasoning := func(txt string) {
		sendChunk(map[string]any{"reasoning_content": txt}, nil)
	}
	// endStream writes the terminal finish chunk, then — if the client requested
	// stream_options.include_usage — a usage-only chunk (choices:[] + usage, the
	// OpenAI shape), then the [DONE] terminator (M237).
	endStream := func(finish string) {
		sendChunk(map[string]any{}, finish)
		if includeUsage {
			_ = sse.WriteJSON(map[string]any{
				"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
				"choices": []map[string]any{},
				"usage":   chatUsage(eng, corr, intent, full.String()),
			})
		}
		_ = sse.WriteData("[DONE]")
	}

	// Opening role chunk.
	sendChunk(map[string]any{"role": "assistant"}, nil)

	type res struct {
		ans string
		err error
	}
	done := make(chan res, 1)
	go func() {
		ans, err := eng.RunModel(r.Context(), corr, intent, model, images, jsonMode)
		done <- res{ans, err}
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub.C:
			if !ok {
				endStream("stop")
				return
			}
			if txt := tokenText(ev); txt != "" {
				sendContent(txt)
			}
			if rt := reasoningText(ev); rt != "" {
				sendReasoning(rt)
			}
		case r := <-done:
			// Drain any tokens still queued, then close the stream.
			for drained := false; !drained; {
				select {
				case ev := <-sub.C:
					if txt := tokenText(ev); txt != "" {
						sendContent(txt)
					}
					if rt := reasoningText(ev); rt != "" {
						sendReasoning(rt)
					}
				default:
					drained = true
				}
			}
			finish := "stop"
			if r.err != nil {
				finish = "error"
				sendContent("\n[error: " + redactErr(r.err.Error()) + "]")
			} else if full.Len() == 0 && r.ans != "" {
				// Defensive: the run produced an answer but emitted no llm.token
				// events — i.e. the provider is non-streaming (it satisfies
				// Complete but not StreamingProvider, e.g. a provider without a
				// native stream). Without this, a stream:true request to such a
				// provider would return only the role + stop chunks and silently
				// drop the answer, while the same provider via non-stream chat
				// returns it. Emit the assembled answer as one content delta so the
				// streamed and non-streamed paths agree.
				sendContent(r.ans)
			}
			endStream(finish)
			return
		}
	}
}
