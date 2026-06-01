// SPDX-License-Identifier: MIT

package controlplane

// Redaction confidence check (M104). Secret redaction (M15 / SPEC-06) scrubs
// secrets from every durably-published event before it enters the permanent
// journal, but an operator had no way to confirm it actually catches THEIR
// secret shape — a vault key rotated mid-run might slip through a gap. This
// exercises the LIVE redactor (built-in patterns + configured literals) against
// a candidate and reports whether it would be scrubbed, returning only the
// redacted form (never the raw input) so the response is safe to surface.

import (
	"net"

	"github.com/agezt/agezt/kernel/redact"
)

func (s *Server) handleRedactTest(conn net.Conn, req Request) {
	text, _ := req.Args["text"].(string)

	red := s.k.Bus().Redactor()
	enabled := red != nil
	redacted := text
	if enabled {
		redacted = red.Redact(text)
	}
	categories := redact.MatchedCategories(text)
	wouldRedact := redacted != text
	// A literal hit is a change the built-in patterns don't explain — i.e. a
	// configured secret literal matched. We never reveal which one.
	literalHit := wouldRedact && len(categories) == 0

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"enabled":      enabled,
			"would_redact": wouldRedact,
			"redacted":     redacted,
			"categories":   toAnySlice(categories),
			"literal_hit":  literalHit,
		},
	})
}

func toAnySlice(ss []string) []any {
	out := make([]any, 0, len(ss))
	for _, s := range ss {
		out = append(out, s)
	}
	return out
}
