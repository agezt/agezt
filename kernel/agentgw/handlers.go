// SPDX-License-Identifier: MIT

package agentgw

import (
	"encoding/json"
	"net/http"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/httpserver"
)


const (
	defaultMemorySearchLimit = 20
	maxMemorySearchLimit     = 200
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
	// StartSSE writes the header triple and applies the process-wide
	// per-client stream cap (V-009/LD-8 — this endpoint had no cap before).
	// No CORS header: this SSE stream is consumed by agent subprocesses over a
	// local socket, never a cross-origin browser. A wildcard ACAO would let any
	// web page read the event bus if the gateway is ever TCP-exposed.
	sse, ok := httpserver.StartSSE(w, r)
	if !ok {
		return
	}
	defer sse.Close()

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
			if err := sse.WriteData(string(data)); err != nil {
				return
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

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
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
