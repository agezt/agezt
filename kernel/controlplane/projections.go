// SPDX-License-Identifier: MIT

package controlplane

import (
	"net"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/journalview"
)

// projectJournal is the native map-row wrapper over journalview.Project: the
// shared limit clamp, since_ms cutoff, tenant resolution and cursor paging,
// with decode shaping each row (WITHOUT ts_unix_ms/seq, which are stamped here)
// or returning false to skip the event. decode runs for every event and the
// cutoff drops decoded rows, so cross-event stash state still accrues from
// events outside the window. Typed app reads call journalview.ProjectValues
// directly; this wrapper remains only for edict_log.
func (s *Server) projectJournal(conn net.Conn, req Request, resultKey string, decode func(*event.Event) (map[string]any, bool)) {
	limit := defaultRunsLimit
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
	if limit > maxRunsLimit {
		limit = maxRunsLimit
	}
	cutoff := sinceCutoff(req.Args["since_ms"])

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	output, err := journalview.Project(k.Journal(), journalview.Input{Limit: limit, CutoffMS: cutoff, Cursor: req.Args["cursor"]}, decode)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{resultKey: output.Rows, "count": output.Count, "next_cursor": output.NextCursor}})
}
