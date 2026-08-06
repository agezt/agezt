// SPDX-License-Identifier: MIT

package controlplane

import (
	"net"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/creds"
)

// handleConfigCenterSet sets a config entry.
func (s *Server) handleConfigCenterSet(conn net.Conn, req Request) {
	key, err := requiredArgString(req.Args, "key")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	value, err := requiredArgString(req.Args, "value")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	sa, err := argStrings(req.Args, "rating", "description")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	ratingStr, description := sa["rating"], sa["description"]
	allowedAgents := configCenterStringList(req.Args["allowed_agents"])
	excludedAgents := configCenterStringList(req.Args["excluded_agents"])

	// Determine rating
	rating := configcenter.RatingInternal
	if ratingStr != "" {
		switch strings.ToLower(ratingStr) {
		case "public":
			rating = configcenter.RatingPublic
		case "internal":
			rating = configcenter.RatingInternal
		case "restricted":
			rating = configcenter.RatingRestricted
		case "secret":
			rating = configcenter.RatingSecret
		default:
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "invalid rating: " + ratingStr})
			return
		}
	}

	entry := configcenter.NewConfigEntry(key, value)
	entry.Rating = rating
	if description != "" {
		entry.Description = description
	}
	entry.AllowedAgents = allowedAgents
	entry.ExcludedAgents = excludedAgents

	if s.k.ConfigCenter() == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "config center not available"})
		return
	}

	if err := s.k.ConfigCenter().Set(entry); err != nil {
		s.fail(conn, req, err)
		return
	}

	// Reload to get the computed entry
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

// handleConfigCenterGet retrieves a config entry.
func (s *Server) handleConfigCenterGet(conn net.Conn, req Request) {
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

	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"entry": entryToMap(entry)},
	})
}

// handleConfigCenterList lists all config entries.
func (s *Server) handleConfigCenterList(conn net.Conn, req Request) {
	ratingStr, _, err := argString(req.Args, "rating")
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	var rating configcenter.Rating
	if ratingStr != "" {
		switch strings.ToLower(ratingStr) {
		case "public":
			rating = configcenter.RatingPublic
		case "internal":
			rating = configcenter.RatingInternal
		case "restricted":
			rating = configcenter.RatingRestricted
		case "secret":
			rating = configcenter.RatingSecret
		default:
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "invalid rating: " + ratingStr})
			return
		}
	}

	if s.k.ConfigCenter() == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "config center not available"})
		return
	}

	var entries []*configcenter.ConfigEntry

	if rating != "" {
		entries = s.k.ConfigCenter().ListByRating(rating)
	} else {
		entries = s.k.ConfigCenter().ListEntries()
	}

	result := make([]map[string]any, len(entries))
	for i, e := range entries {
		result[i] = entryToMap(e)
	}

	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"entries": result, "count": len(result)},
	})
}

// handleConfigCenterDelete deletes a config entry.
func (s *Server) handleConfigCenterDelete(conn net.Conn, req Request) {
	key, err := requiredArgString(req.Args, "key")
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	if s.k.ConfigCenter() == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "config center not available"})
		return
	}

	err = s.k.ConfigCenter().Delete(key)
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"deleted": true},
	})
}

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

// handleConfigCenterAudit returns the audit log.
func (s *Server) handleConfigCenterAudit(conn net.Conn, req Request) {
	sinceStr, _, err := argString(req.Args, "since")
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	var since time.Duration
	if sinceStr != "" {
		since, err = time.ParseDuration(sinceStr)
		if err != nil {
			s.failMsg(conn, req, "invalid duration: "+sinceStr)
			return
		}
	}

	if s.k.ConfigCenter() == nil {
		s.failMsg(conn, req, "config center not available")
		return
	}

	entries := s.k.ConfigCenter().AuditLog(since)

	result := make([]map[string]any, len(entries))
	for i, e := range entries {
		result[i] = map[string]any{
			"timestamp": e.Timestamp,
			"event":     e.Event,
			"key":       e.Key,
			"agent_id":  e.AgentID,
			"run_id":    e.RunID,
			"rating":    string(e.Rating),
			"reason":    e.Reason,
			"decision":  string(e.Decision),
			"policy":    e.Policy,
		}
	}

	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"entries": result, "count": len(result)},
	})
}

// handleConfigCenterHealth returns health status.
func (s *Server) handleConfigCenterHealth(conn net.Conn, req Request) {
	if s.k.ConfigCenter() == nil {
		s.writeResp(conn, Response{
			ID:   req.ID,
			Type: RespResult,
			Result: map[string]any{
				"status": "unavailable",
				"checks": map[string]string{
					"config_center": "not configured",
				},
			},
		})
		return
	}

	center := s.k.ConfigCenter()
	stats := center.Stats()

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"status": "healthy",
			"checks": map[string]string{
				"config_center": "ok",
				"store":         "ok",
			},
			"stats": stats,
		},
	})
}

// entryToMap converts a ConfigEntry to a map. Secret-rated values are never
// emitted in cleartext: they are reduced to a masked fingerprint (and flagged
// with "masked":true) so the value can't be read off the API response body —
// the console already renders secrets as dots, and no consumer needs the raw
// value back from a read/echo path. Reading the real value requires a deliberate
// reveal path, not an incidental list/get.
func entryToMap(e *configcenter.ConfigEntry) map[string]any {
	value := e.Value
	masked := false
	if e.Rating == configcenter.RatingSecret {
		value = creds.MaskValue(e.Value)
		masked = true
	}
	m := map[string]any{
		"key":        e.Key,
		"value":      value,
		"rating":     string(e.Rating),
		"created_at": e.CreatedAt,
		"updated_at": e.UpdatedAt,
		"version":    e.Version,
	}
	if masked {
		m["masked"] = true
	}
	if e.Description != "" {
		m["description"] = e.Description
	}
	if len(e.Tags) > 0 {
		m["tags"] = e.Tags
	}
	if e.AccessPolicy != "" {
		m["access_policy"] = string(e.AccessPolicy)
	}
	if len(e.AllowedAgents) > 0 {
		m["allowed_agents"] = append([]string(nil), e.AllowedAgents...)
	}
	if len(e.ExcludedAgents) > 0 {
		m["excluded_agents"] = append([]string(nil), e.ExcludedAgents...)
	}
	return m
}

func configCenterStringList(raw any) []string {
	switch v := raw.(type) {
	case nil:
		return nil
	case string:
		return configCenterSplitList(v)
	case []string:
		return configCenterCleanList(v)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return configCenterCleanList(out)
	default:
		return nil
	}
}

func configCenterSplitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t' })
	return configCenterCleanList(parts)
}

func configCenterCleanList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, v)
	}
	return out
}

// registerConfigCenterCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerConfigCenterCommands() {
	register(
		commandSpec{Cmd: CmdConfigCenterSet, Handler: func(dc *DispatchCtx) { dc.S.handleConfigCenterSet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigCenterGet, Handler: func(dc *DispatchCtx) { dc.S.handleConfigCenterGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigCenterList, Handler: func(dc *DispatchCtx) { dc.S.handleConfigCenterList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigCenterDelete, Handler: func(dc *DispatchCtx) { dc.S.handleConfigCenterDelete(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigCenterSetRating, Handler: func(dc *DispatchCtx) { dc.S.handleConfigCenterSetRating(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigCenterSetAccess, Handler: func(dc *DispatchCtx) { dc.S.handleConfigCenterSetAccess(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigCenterAccessLog, Handler: func(dc *DispatchCtx) { dc.S.handleConfigCenterAccessLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigCenterAudit, Handler: func(dc *DispatchCtx) { dc.S.handleConfigCenterAudit(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigCenterHealth, Handler: func(dc *DispatchCtx) { dc.S.handleConfigCenterHealth(dc.Conn, dc.Req) }},
	)
}
