// SPDX-License-Identifier: MIT

package controlplane

// Verifiable journal export (M101). The durable, hash-chained journal is the
// system's source of truth; an operator running SLA/compliance workloads needs
// to archive it (or a recent window) to disk for audit, disaster-recovery, or
// off-system analysis — and to TRUST that archive later. `agt journal tail`
// can dump events, but it is count-capped and produces nothing an auditor can
// re-verify offline. This handler streams every event (optionally since a
// cutoff) with its hash + prev_hash intact, and stamps the bundle with the
// chain head at export time, so `agt journal verify --bundle <file>` can
// recompute every BLAKE3 hash and confirm prev-hash continuity without the
// daemon — the integrity guarantee that makes the export worth keeping.

import (
	"net"

	"github.com/agezt/agezt/kernel/event"
)

// maxJournalExportN bounds how many events one export bundles. An export is
// meant to be complete, so this sits far above the tail/grep caps; it exists
// only as a memory backstop against a pathologically large journal. A hit is
// surfaced as truncated=true rather than a silent cut.
const maxJournalExportN = 200_000

// MaxJournalExportN exposes the export size cap so the CLI can name it in the
// truncation notice without hardcoding the number twice.
func MaxJournalExportN() int { return maxJournalExportN }

func (s *Server) handleJournalExport(conn net.Conn, req Request) {
	cutoff := sinceCutoff(req.Args["since_ms"])
	// Optional correlation scope (M383, SPEC-09 §3): a surgical "cut" of one run's
	// (correlation's) event subgraph rather than a contiguous window. The result
	// is intentionally non-contiguous — the CLI marks the bundle scoped and the
	// offline verify path checks per-event integrity + scope membership instead of
	// prev-hash continuity.
	correlation, _ := req.Args["correlation"].(string)

	headSeq, headHash := s.k.Journal().Head()
	if headSeq < 0 {
		headSeq = 0
	}

	events := make([]any, 0, 256)
	var firstSeq, lastSeq int64 = -1, -1
	truncated := false
	err := s.k.Journal().Range(func(e *event.Event) error {
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		if correlation != "" && e.CorrelationID != correlation {
			return nil
		}
		if len(events) >= maxJournalExportN {
			truncated = true
			return stopWalkSentinel{}
		}
		if firstSeq < 0 {
			firstSeq = e.Seq
		}
		lastSeq = e.Seq
		events = append(events, e)
		return nil
	})
	if err != nil {
		if _, ok := err.(stopWalkSentinel); !ok {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
			return
		}
	}

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"events":      events,
			"count":       len(events),
			"first_seq":   firstSeq,
			"last_seq":    lastSeq,
			"head_seq":    headSeq,
			"head_hash":   headHash,
			"truncated":   truncated,
			"correlation": correlation,
		},
	})
}
