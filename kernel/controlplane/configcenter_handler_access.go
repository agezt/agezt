// SPDX-License-Identifier: MIT

// Config-center governance handlers: handleConfigCenterSetRating +
// handleConfigCenterSetAccess + handleConfigCenterAccessLog.
// Carved out of configcenter_handler.go during the Day 186 god-file
// split so the main file can stay focused on CRUD (Set/Get/List/Delete)
// and the audit file can stay focused on observability + helpers.
// Public API unchanged.
package controlplane


import (
	"net"
	"time"

	"github.com/agezt/agezt/kernel/configcenter"
)

// handleConfigCenterSetRating sets the rating for a config entry.
func (s *Server) handleConfigCenterSetRating(conn net.Conn, req Request) {
	key, err := requiredArgString(req.Args, "key")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	ratingStr, err := requiredArgString(req.Args, "rating")
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	rating, err := configcenter.ParseRating(ratingStr)
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	if s.k.ConfigCenter() == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "config center not available"})
		return
	}

	// Check if entry exists
	entry, err := s.k.ConfigCenter().GetEntry(key)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "key not found: " + key})
		return
	}

	// Check if this is a manual override
	autoRating := s.k.ConfigCenter().Classifier().Classify(key, entry.Value)
	isOverride := rating != autoRating

	entry.Rating = rating
	if err := s.k.ConfigCenter().Set(entry); err != nil {
		s.fail(conn, req, err)
		return
	}

	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"override": isOverride},
	})
}

func (s *Server) handleConfigCenterSetAccess(conn net.Conn, req Request) {
	key, err := requiredArgString(req.Args, "key")
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	if s.k.ConfigCenter() == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "config center not available"})
		return
	}

	entry, err := s.k.ConfigCenter().GetEntry(key)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "key not found: " + key})
		return
	}
	entry.AllowedAgents = configCenterStringList(req.Args["allowed_agents"])
	entry.ExcludedAgents = configCenterStringList(req.Args["excluded_agents"])
	if err := s.k.ConfigCenter().Set(entry); err != nil {
		s.fail(conn, req, err)
		return
	}
	updated, err := s.k.ConfigCenter().GetEntry(key)
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"entry": entryToMap(updated)},
	})
}

// handleConfigCenterAccessLog returns the access log.
func (s *Server) handleConfigCenterAccessLog(conn net.Conn, req Request) {
	sa, err := argStrings(req.Args, "key", "agent_id", "since")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	key, agentID, sinceStr := sa["key"], sa["agent_id"], sa["since"]

	var since time.Duration
	if sinceStr != "" {
		var err error
		since, err = time.ParseDuration(sinceStr)
		if err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "invalid duration: " + sinceStr})
			return
		}
	}

	if s.k.ConfigCenter() == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "config center not available"})
		return
	}

	logs := s.k.ConfigCenter().AccessLog(key, agentID, since)

	result := make([]map[string]any, len(logs))
	for i, l := range logs {
		result[i] = map[string]any{
			"timestamp": l.Timestamp,
			"key":       l.Key,
			"agent_id":  l.AgentID,
			"run_id":    l.RunID,
			"rating":    string(l.Rating),
			"decision":  string(l.Decision),
			"reason":    l.Reason,
			"value_log": l.ValueLog,
		}
	}

	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"logs": result, "count": len(result)},
	})
}

