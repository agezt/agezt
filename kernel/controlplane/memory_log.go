// SPDX-License-Identifier: MIT

package controlplane

// Memory-operation log (M85) — a read-only timeline of the journal's
// memory.written / memory.forgotten / memory.superseded events. `agt memory
// list` shows the CURRENT records (the projection); this shows the HISTORY of
// how that state came to be — what the agent learned, forgot, and replaced, and
// when. For a persistent-memory agent that audit is a trust surface: it answers
// "why does it believe this?" and "when did it forget that?".

import (
	"encoding/json"
	"net"

	"github.com/agezt/agezt/kernel/event"
)

func (s *Server) handleMemoryLog(conn net.Conn, req Request) {
	opFilter, _, err := argString(req.Args, "op") // written|forgotten|superseded
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.projectJournal(conn, req, "ops", func(e *event.Event) (map[string]any, bool) {
		var op, id, subject, mtyp string
		switch e.Kind {
		case event.KindMemoryWritten:
			var p struct {
				Action, ID, Type, Subject string
			}
			_ = json.Unmarshal(e.Payload, &p)
			op = p.Action // "write" or "revive"
			if op == "" {
				op = "write"
			}
			id, subject, mtyp = p.ID, p.Subject, p.Type
		case event.KindMemoryForgotten:
			var p struct{ ID, Subject string }
			_ = json.Unmarshal(e.Payload, &p)
			op, id, subject = "forget", p.ID, p.Subject
		case event.KindMemorySuperseded:
			var p struct {
				OldID string `json:"old_id"`
				NewID string `json:"new_id"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			op, id, subject = "supersede", p.OldID, "→ "+p.NewID
		case event.KindMemoryPromoted:
			var p struct {
				ID        string `json:"id"`
				Subject   string `json:"subject"`
				FromScope string `json:"from_scope"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			op, id, subject = "promote", p.ID, p.Subject+" (was private to "+p.FromScope+")"
		default:
			return nil, false
		}
		// op filter (M85): "written" matches write+revive (both are
		// memory.written); the others match by their own verb.
		if opFilter != "" && !memOpMatches(opFilter, e.Kind, op) {
			return nil, false
		}
		return map[string]any{"op": op, "id": id, "type": mtyp, "subject": subject}, true
	})
}

// memOpMatches maps the user's --op filter to the event kind. "written" keeps
// both write and revive (the memory.written kind); "forgotten"/"superseded"/
// "promoted" match their kinds.
func memOpMatches(filter string, kind event.Kind, op string) bool {
	switch filter {
	case "written", "write":
		return kind == event.KindMemoryWritten
	case "forgotten", "forget":
		return kind == event.KindMemoryForgotten
	case "superseded", "supersede":
		return kind == event.KindMemorySuperseded
	case "promoted", "promote":
		return kind == event.KindMemoryPromoted
	default:
		return op == filter
	}
}
