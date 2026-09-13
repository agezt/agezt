// SPDX-License-Identifier: MIT

// Config-center observability + helpers: handleConfigCenterAudit +
// handleConfigCenterHealth + entryToMap + configCenterStringList +
// configCenterSplitList + configCenterCleanList. Carved out of
// configcenter_handler.go during the Day 186 god-file split so the
// main file can stay focused on CRUD (Set/Get/List/Delete) and the
// access file can stay focused on governance (Rating/Access/AccessLog).
// Public API unchanged.
package controlplane


import (
	"net"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/creds"
)

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

