// SPDX-License-Identifier: MIT

package agentgw

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/memory"
)

// handleEventbusSubscribe handles Server-Sent Events subscription to the event bus.
func (g *Gateway) handleEventbusSubscribe(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	if claims == nil {
		responseError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no claims")
		return
	}

	// Check capability
	if err := g.capCheck.Check(claims, CapEventbusSubscribe); err != nil {
		responseError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	// Get subscription pattern from query
	pattern := r.URL.Query().Get("pattern")
	if pattern == "" {
		pattern = ">"
	}

	if g.bus == nil {
		responseError(w, http.StatusServiceUnavailable, "BUS_UNAVAILABLE", "event bus not connected")
		return
	}

	// Subscribe to bus
	sub, err := g.bus.Subscribe(pattern, 256)
	if err != nil {
		responseError(w, http.StatusInternalServerError, "SUBSCRIBE_ERROR", err.Error())
		return
	}
	defer sub.Cancel()

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Flush headers
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Stream events
	for {
		select {
		case ev, ok := <-sub.C:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

// handleEventbusPublish handles publishing events to the bus.
func (g *Gateway) handleEventbusPublish(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	if claims == nil {
		responseError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no claims")
		return
	}

	if err := g.capCheck.Check(claims, CapEventbusPublish); err != nil {
		responseError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	if g.bus == nil {
		responseError(w, http.StatusServiceUnavailable, "BUS_UNAVAILABLE", "event bus not connected")
		return
	}

	var req struct {
		Event   string                 `json:"event"`
		Payload map[string]interface{} `json:"payload,omitempty"`
		Tags    map[string]string      `json:"tags,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		responseError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	spec := event.Spec{
		Subject: req.Event,
		Kind:    event.KindInfo,
		Actor:   claims.RunID,
		Payload: req.Payload,
		Tags:    req.Tags,
	}

	if _, err := g.bus.Publish(spec); err != nil {
		responseError(w, http.StatusInternalServerError, "PUBLISH_ERROR", err.Error())
		return
	}

	responseJSON(w, http.StatusAccepted, map[string]string{"status": "published"})
}

// handleMemoryWrite handles writing a memory record (Remember).
func (g *Gateway) handleMemoryWrite(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	if claims == nil {
		responseError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no claims")
		return
	}

	if err := g.capCheck.Check(claims, CapMemoryWrite); err != nil {
		responseError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	var req struct {
		Type    string            `json:"type"`
		Subject string            `json:"subject"`
		Content string            `json:"content"`
		Tags    map[string]string `json:"tags,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		responseError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if g.mem == nil {
		responseError(w, http.StatusServiceUnavailable, "MEMORY_UNAVAILABLE", "memory manager not connected")
		return
	}

	recType := memory.Type(req.Type)
	if req.Type == "" {
		recType = memory.TypeFact
	}

	spec := memory.RememberSpec{
		Type:    recType,
		Subject: req.Subject,
		Content: req.Content,
		Tags:    req.Tags,
		Actor:   claims.RunID,
	}

	record, created, err := g.mem.Remember(claims.RunID, spec)
	if err != nil {
		responseError(w, http.StatusInternalServerError, "WRITE_ERROR", err.Error())
		return
	}

	responseJSON(w, http.StatusCreated, map[string]interface{}{
		"record":  record,
		"created": created,
	})
}

// handleMemorySearch handles searching memory (Recall).
func (g *Gateway) handleMemorySearch(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	if claims == nil {
		responseError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no claims")
		return
	}

	if err := g.capCheck.Check(claims, CapMemorySearch); err != nil {
		responseError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		responseError(w, http.StatusBadRequest, "MISSING_QUERY", "search query required")
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	if g.mem == nil {
		responseError(w, http.StatusServiceUnavailable, "MEMORY_UNAVAILABLE", "memory manager not connected")
		return
	}

	results, err := g.mem.Recall(claims.RunID, query, limit)
	if err != nil {
		responseError(w, http.StatusInternalServerError, "SEARCH_ERROR", err.Error())
		return
	}

	// Convert Scored results to a serializable format
	type ScoredResult struct {
		ID         string  `json:"id"`
		Type       string  `json:"type"`
		Subject    string  `json:"subject"`
		Content    string  `json:"content"`
		Confidence float64 `json:"confidence"`
		LastSeenMS int64   `json:"last_seen_ms"`
		Score      float64 `json:"score"`
	}

	out := make([]ScoredResult, len(results))
	for i, r := range results {
		out[i] = ScoredResult{
			ID:         r.Record.ID,
			Type:       string(r.Record.Type),
			Subject:    r.Record.Subject,
			Content:    r.Record.Content,
			Confidence: r.Record.Confidence,
			LastSeenMS: r.Record.LastSeenMS,
			Score:      r.Score,
		}
	}

	responseJSON(w, http.StatusOK, map[string]interface{}{"results": out})
}

// handleMemoryDelete handles deleting a memory record (Forget).
func (g *Gateway) handleMemoryDelete(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	if claims == nil {
		responseError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no claims")
		return
	}

	if err := g.capCheck.Check(claims, CapMemoryDelete); err != nil {
		responseError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		responseError(w, http.StatusBadRequest, "MISSING_ID", "memory record id required")
		return
	}

	if g.mem == nil {
		responseError(w, http.StatusServiceUnavailable, "MEMORY_UNAVAILABLE", "memory manager not connected")
		return
	}

	deleted, err := g.mem.Forget(claims.RunID, id)
	if err != nil {
		responseError(w, http.StatusInternalServerError, "DELETE_ERROR", err.Error())
		return
	}

	responseJSON(w, http.StatusOK, map[string]interface{}{"deleted": deleted})
}

// handleLogWrite handles writing logs.
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

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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

	// For now, just return a message about using eventbus.subscribe
	// Full implementation would require a journal reader
	responseJSON(w, http.StatusOK, map[string]interface{}{
		"message": "use eventbus.subscribe to receive log events in real-time",
	})
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
