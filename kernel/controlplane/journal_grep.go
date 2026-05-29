// SPDX-License-Identifier: MIT

package controlplane

// Server-side journal filter. Sister handler to handleJournalTail
// (which returns the trailing window unconditionally); journal_grep
// adds an AND-of-filters predicate and walks the chain itself so
// the client doesn't have to receive every event just to discard
// most. The semantic shape mirrors `agt journal tail` so renderers
// don't need a second code path.
//
// Memory: matches accumulate in a slice capped by `limit`. A
// pathological "match everything" call with limit=10000 holds at
// most 10000 event pointers — fine on any host that can run the
// kernel. Walking stops at the cap so the daemon is not penalised
// for a verbose query.

import (
	"net"
	"strings"

	"github.com/agezt/agezt/kernel/event"
)

const (
	defaultJournalGrepLimit = 100
	maxJournalGrepLimit     = 10_000
)

func (s *Server) handleJournalGrep(conn net.Conn, req Request) {
	pattern, _ := req.Args["pattern"].(string)
	patternLower := strings.ToLower(pattern)
	kindFilter, _ := req.Args["kind"].(string)
	subjectFilter, _ := req.Args["subject"].(string)
	actorFilter, _ := req.Args["actor"].(string)
	corrFilter, _ := req.Args["correlation_id"].(string)

	limit := defaultJournalGrepLimit
	if raw, ok := req.Args["limit"]; ok {
		switch v := raw.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		case int64:
			limit = int(v)
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > maxJournalGrepLimit {
		limit = maxJournalGrepLimit
	}

	headSeq, _ := s.k.Journal().Head()
	if headSeq < 0 {
		headSeq = 0
	}

	matches := make([]*event.Event, 0, limit)
	// errStopWalk is the sentinel we return from the Range callback
	// to short-circuit once `limit` matches have accumulated. The
	// Journal's Range contract treats any non-nil error as "abort";
	// we filter this specific sentinel out at the end so the caller
	// doesn't see it.
	errStopWalk := stopWalkSentinel{}

	err := s.k.Journal().Range(func(e *event.Event) error {
		if kindFilter != "" && string(e.Kind) != kindFilter {
			return nil
		}
		if subjectFilter != "" && e.Subject != subjectFilter {
			return nil
		}
		if actorFilter != "" && e.Actor != actorFilter {
			return nil
		}
		if corrFilter != "" && e.CorrelationID != corrFilter {
			return nil
		}
		if patternLower != "" && !matchesPattern(e, patternLower) {
			return nil
		}
		matches = append(matches, e)
		if len(matches) >= limit {
			return errStopWalk
		}
		return nil
	})
	if err != nil {
		if _, ok := err.(stopWalkSentinel); !ok {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
			return
		}
	}

	out := make([]any, 0, len(matches))
	for _, e := range matches {
		out = append(out, e)
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"events": out,
			"count":  len(matches),
			"head":   headSeq,
		},
	})
}

// stopWalkSentinel is a typed empty error used to short-circuit
// Journal.Range without conflating with real I/O errors. The grep
// handler ignores it; any other error bubbles up as a server error.
type stopWalkSentinel struct{}

func (stopWalkSentinel) Error() string { return "stop walk" }

// matchesPattern checks whether `pattern` (already lowercased)
// appears in any free-text field of the event. The payload is
// included so operators can find "rm -rf" inside a shell tool
// invocation without knowing which Kind it was emitted under.
func matchesPattern(e *event.Event, pattern string) bool {
	if strings.Contains(strings.ToLower(string(e.Kind)), pattern) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Subject), pattern) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Actor), pattern) {
		return true
	}
	if strings.Contains(strings.ToLower(e.CorrelationID), pattern) {
		return true
	}
	if len(e.Payload) > 0 && strings.Contains(strings.ToLower(string(e.Payload)), pattern) {
		return true
	}
	return false
}
