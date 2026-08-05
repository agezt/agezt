// SPDX-License-Identifier: MIT

package controlplane

// World-model operation log (M86) — a read-only timeline of the journal's
// worldmodel.entity.upserted / relation.upserted / forgotten events. `agt world
// list` shows the CURRENT graph (the projection); this shows the HISTORY of how
// it formed — what entities and relations the agent observed, reinforced, and
// forgot, and when. The world-model analogue of `agt memory log` (M85): both are
// knowledge stores, both keep an audit timeline.

import (
	"encoding/json"
	"net"

	"github.com/agezt/agezt/kernel/event"
)

func (s *Server) handleWorldLog(conn net.Conn, req Request) {
	kindFilter, _, err := argString(req.Args, "kind") // entity|relation
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.projectJournal(conn, req, "ops", func(e *event.Event) (map[string]any, bool) {
		var op, what, label string
		switch e.Kind {
		case event.KindWorldEntityUpserted:
			var p struct{ Action, Name, Kind string }
			_ = json.Unmarshal(e.Payload, &p)
			op, what, label = p.Action, "entity", p.Name
			if p.Kind != "" {
				label += " [" + p.Kind + "]"
			}
		case event.KindWorldRelationUpserted:
			var p struct{ Action, From, Verb, To string }
			_ = json.Unmarshal(e.Payload, &p)
			op, what, label = p.Action, "relation", p.From+" "+p.Verb+" "+p.To
		case event.KindWorldForgotten:
			var p struct {
				Name, Verb, What string
			}
			_ = json.Unmarshal(e.Payload, &p)
			op, what = "forget", p.What
			if p.Name != "" {
				label = p.Name
			} else {
				label = p.Verb
			}
		default:
			return nil, false
		}
		if op == "" {
			op = "upsert"
		}
		if kindFilter != "" && what != kindFilter {
			return nil, false
		}
		return map[string]any{"op": op, "what": what, "label": label}, true
	})
}
