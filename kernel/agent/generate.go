// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrNoObjectGenerated is the sentinel returned (wrapped in *ObjectError) when
// GenerateObject cannot obtain schema-valid JSON within its repair budget.
var ErrNoObjectGenerated = errors.New("no schema-valid object generated")

// ObjectError carries the diagnostic context of a failed GenerateObject call:
// the last raw model text, the cumulative token usage spent across attempts,
// and the underlying cause (a validation error, or ErrNoObjectGenerated).
type ObjectError struct {
	LastText string
	Usage    Usage
	Err      error
}

func (e *ObjectError) Error() string {
	return fmt.Sprintf("generate object: %v (last output: %.200q)", e.Err, e.LastText)
}
func (e *ObjectError) Unwrap() error { return e.Err }

// DefaultObjectRepairs is how many additional attempts GenerateObject makes
// after the first, feeding each failure back to the model as a repair turn.
const DefaultObjectRepairs = 2

// GenerateObject drives a provider to produce a single JSON value conforming to
// schema, validates it, and unmarshals it into out. It is the unified
// structured-output path (M997), analogous to the Vercel AI SDK's
// generateObject: a schema-in / typed-value-out call with automatic repair.
//
// Strategy: it sets JSONMode (honoured natively by OpenAI/Gemini/Ollama,
// ignored elsewhere), appends the schema to the system prompt so models without
// schema-constrained decoding still conform, then calls the provider. The reply
// is JSON-extracted (tolerating ```json fences and surrounding prose) and
// validated with ValidateJSON. On any failure it appends the model's output and
// the validation error as a repair turn and retries up to DefaultObjectRepairs
// times. The returned Usage is the sum across all attempts.
//
// schema may be empty to accept any JSON value. out must be a non-nil pointer.
func GenerateObject(ctx context.Context, p Provider, req CompletionRequest, schema json.RawMessage, out any) (Usage, error) {
	if p == nil {
		return Usage{}, errors.New("generate object: nil provider")
	}
	if out == nil {
		return Usage{}, errors.New("generate object: nil out")
	}
	req.JSONMode = true
	req.System = appendSchemaInstruction(req.System, schema)

	var total Usage
	var lastText string
	var lastErr error
	for attempt := 0; attempt <= DefaultObjectRepairs; attempt++ {
		resp, err := p.Complete(ctx, req)
		if err != nil {
			// A provider/transport error is terminal — no point repairing.
			return total, fmt.Errorf("generate object: %w", err)
		}
		total = addUsage(total, resp.Usage)
		lastText = resp.Message.Content

		candidate, ok := extractJSON(resp.Message.Content)
		if ok {
			if verr := ValidateJSON(schema, candidate); verr == nil {
				if uerr := json.Unmarshal(candidate, out); uerr == nil {
					return total, nil
				} else {
					lastErr = uerr
				}
			} else {
				lastErr = verr
			}
		} else {
			lastErr = errors.New("response contained no JSON value")
		}

		if attempt == DefaultObjectRepairs {
			break
		}
		// Repair turn: show the model its output and the precise problem.
		req.Messages = append(req.Messages,
			Message{Role: RoleAssistant, Content: resp.Message.Content},
			Message{Role: RoleUser, Content: fmt.Sprintf(
				"That response was not valid against the required schema: %v. "+
					"Reply again with ONLY the corrected JSON value and no other text.", lastErr)},
		)
	}
	return total, &ObjectError{LastText: lastText, Usage: total, Err: errors.Join(ErrNoObjectGenerated, lastErr)}
}

// appendSchemaInstruction appends a schema-conformance instruction to a system
// prompt. Empty schema leaves the prompt unchanged.
func appendSchemaInstruction(system string, schema json.RawMessage) string {
	s := strings.TrimSpace(string(schema))
	if s == "" {
		return system
	}
	instr := "Respond with a single JSON value that strictly conforms to this JSON Schema. " +
		"Output only the JSON value, with no markdown fences or surrounding prose:\n" + s
	if strings.TrimSpace(system) == "" {
		return instr
	}
	return system + "\n\n" + instr
}

// extractJSON pulls the first complete JSON value (object or array) out of a
// model reply, tolerating ```json code fences and leading/trailing prose. It
// returns ok=false when no balanced JSON value is found.
func extractJSON(s string) (json.RawMessage, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	// Strip a leading code fence (```json / ```) and any trailing fence.
	if strings.HasPrefix(s, "```") {
		if nl := strings.IndexByte(s, '\n'); nl >= 0 {
			s = s[nl+1:]
		}
		if end := strings.LastIndex(s, "```"); end >= 0 {
			s = s[:end]
		}
		s = strings.TrimSpace(s)
	}
	// Fast path: the whole (trimmed) string is valid JSON.
	if json.Valid([]byte(s)) {
		return json.RawMessage(s), true
	}
	// Otherwise scan for the first balanced {...} or [...] span.
	start := strings.IndexAny(s, "{[")
	if start < 0 {
		return nil, false
	}
	if span, ok := balancedSpan(s[start:]); ok {
		return json.RawMessage(span), true
	}
	return nil, false
}

// balancedSpan returns the prefix of s that is a single balanced JSON object or
// array, honouring strings and escapes so braces inside string literals don't
// throw off the depth count.
func balancedSpan(s string) (string, bool) {
	if len(s) == 0 {
		return "", false
	}
	open := s[0]
	var closer byte
	switch open {
	case '{':
		closer = '}'
	case '[':
		closer = ']'
	default:
		return "", false
	}
	depth := 0
	inStr := false
	esc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case open:
			depth++
		case closer:
			depth--
			if depth == 0 {
				return s[:i+1], true
			}
		}
	}
	return "", false
}

// addUsage sums two Usage records field-wise, preserving the latest model id.
func addUsage(a, b Usage) Usage {
	a.InputTokens += b.InputTokens
	a.OutputTokens += b.OutputTokens
	a.CachedInputTokens += b.CachedInputTokens
	a.CacheWriteInputTokens += b.CacheWriteInputTokens
	if b.Model != "" {
		a.Model = b.Model
	}
	return a
}
