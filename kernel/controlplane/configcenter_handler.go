// SPDX-License-Identifier: MIT

// Control-plane configcenter command handlers + helpers (entryToMap + string/split/clean helpers).
// Code extracted from configcenter_handler.go during the Day-114 god-file split.
// Public API unchanged.
package controlplane

import (
	"net"
	"strings"

	"github.com/agezt/agezt/kernel/configcenter"
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

