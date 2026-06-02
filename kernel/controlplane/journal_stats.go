// SPDX-License-Identifier: MIT

package controlplane

// Journal size/shape observability (M132). The journal is append-only and
// full-retention — projections are rebuilt from it on boot, so it is NOT pruned
// in place. That makes "how big is the journal, and WHAT is filling it" a real
// operator question (the input to an archival / bigger-disk decision), which
// neither `agt disk` (bytes only) nor `agt status` (head seq only) answers. This
// folds the journal once into an event count, a per-kind breakdown, the time
// span, and the on-disk size + segment count. Read-only; tenant-routed so a
// future `--tenant` can scope it to a tenant's own journal.

import (
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/kernel/event"
)

func (s *Server) handleJournalStats(conn net.Conn, req Request) {
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	var count, oldest, newest int64
	byKind := map[string]int64{}
	if err := k.Journal().Range(func(e *event.Event) error {
		count++
		byKind[string(e.Kind)]++
		if e.TSUnixMS > 0 {
			if oldest == 0 || e.TSUnixMS < oldest {
				oldest = e.TSUnixMS
			}
			if e.TSUnixMS > newest {
				newest = e.TSUnixMS
			}
		}
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	journalDir := filepath.Join(k.BaseDir(), "journal")
	byKindAny := make(map[string]any, len(byKind))
	for kind, n := range byKind {
		byKindAny[kind] = n
	}

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"events":         count,
			"segments":       countSegments(journalDir),
			"bytes":          dirSize(journalDir),
			"by_kind":        byKindAny,
			"oldest_unix_ms": oldest,
			"newest_unix_ms": newest,
		},
	})
}

// countSegments counts the journal's rotated segment files (*.jsonl) under dir.
// Best-effort: a missing/unreadable directory counts 0.
func countSegments(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			n++
		}
	}
	return n
}
