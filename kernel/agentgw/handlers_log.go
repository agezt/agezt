// SPDX-License-Identifier: MIT

package agentgw

// Log + Agent handlers: handleLogWrite + handleLogRead +
// handleAgentList + handleAgentQuery. Carved out of handlers.go
// during the Day 194 god-file split so the main file can stay
// focused on eventbus handlers and the memory file can stay
// focused on memory handlers.
// Public API unchanged.

import (
	"encoding/json"
	"net/http"

	"github.com/agezt/agezt/kernel/event"
)

func (g *Gateway) handleLogWrite(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	if claims == nil {
		responseError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no claims")
		return
	}

	if err := g.capCheck.Check(claims, CapLogWrite); err != nil {
		responseError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	var req struct {
		Level   string                 `json:"level"`
		Message string                 `json:"message"`
		Meta    map[string]interface{} `json:"meta,omitempty"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		responseError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	// Publish as a log event
	if g.bus != nil {
		g.bus.Publish(event.Spec{
			Subject: "agent.log." + req.Level,
			Kind:    event.KindInfo,
			Actor:   claims.RunID,
			Payload: map[string]interface{}{
				"message": req.Message,
				"meta":    req.Meta,
			},
			Tags: map[string]string{
				"run_id": claims.RunID,
				"sub_id": claims.SubprocessID,
			},
		})
	}

	responseJSON(w, http.StatusAccepted, map[string]string{"status": "logged"})
}

// handleLogRead handles reading logs.
func (g *Gateway) handleLogRead(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	if claims == nil {
		responseError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no claims")
		return
	}

	if err := g.capCheck.Check(claims, CapLogRead); err != nil {
		responseError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	// No journal reader is wired into the gateway yet. Answer honestly with
	// 501 instead of a placeholder 200 that callers could mistake for data —
	// eventbus.subscribe is the supported way to receive log events live.
	responseError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"log read is not implemented; use eventbus.subscribe to receive log events in real-time")
}

// handleAgentList handles listing agents.
func (g *Gateway) handleAgentList(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	if claims == nil {
		responseError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no claims")
		return
	}

	if err := g.capCheck.Check(claims, CapAgentList); err != nil {
		responseError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	if g.roster == nil {
		responseError(w, http.StatusServiceUnavailable, "ROSTER_UNAVAILABLE", "roster not connected")
		return
	}

	agents := g.roster.List()
	out := make([]map[string]interface{}, len(agents))
	for i, a := range agents {
		out[i] = map[string]interface{}{
			"id":      a.ID,
			"slug":    a.Slug,
			"name":    a.Name,
			"model":   a.Model,
			"enabled": a.Enabled,
			"retired": a.Retired,
		}
	}

	responseJSON(w, http.StatusOK, map[string]interface{}{"agents": out})
}

// handleAgentQuery handles querying agent status.
func (g *Gateway) handleAgentQuery(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	if claims == nil {
		responseError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no claims")
		return
	}

	if err := g.capCheck.Check(claims, CapAgentQuery); err != nil {
		responseError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	agentID := r.URL.Query().Get("id")
	if agentID == "" {
		responseError(w, http.StatusBadRequest, "MISSING_ID", "agent id required")
		return
	}

	if g.roster == nil {
		responseError(w, http.StatusServiceUnavailable, "ROSTER_UNAVAILABLE", "roster not connected")
		return
	}

	profile, found := g.roster.Get(agentID)
	if !found {
		responseError(w, http.StatusNotFound, "NOT_FOUND", "agent not found")
		return
	}

	responseJSON(w, http.StatusOK, map[string]interface{}{
		"id":        profile.ID,
		"slug":      profile.Slug,
		"name":      profile.Name,
		"model":     profile.Model,
		"enabled":   profile.Enabled,
		"retired":   profile.Retired,
		"createdMs": profile.CreatedMS,
	})
}

