// SPDX-License-Identifier: MIT

package agentgw

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/agezt/agezt/kernel/configcenter"
)

// ConfigHandler handles config center API endpoints.
type ConfigHandler struct {
	center   *configcenter.Center
	capCheck *CapabilityChecker
}

// NewConfigHandler creates a new ConfigHandler.
func NewConfigHandler(center *configcenter.Center, capCheck *CapabilityChecker) *ConfigHandler {
	return &ConfigHandler{
		center:   center,
		capCheck: capCheck,
	}
}

// handleConfigGet handles GET /v1/config/{key} and GET /v1/config/{key}/.
func (h *ConfigHandler) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	if claims == nil {
		responseError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no claims")
		return
	}

	// Capability check
	if err := h.capCheck.Check(claims, CapConfigAccess); err != nil {
		responseError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	// Extract key from path: /v1/config/{key}
	key := strings.TrimPrefix(r.URL.Path, "/v1/config/")
	key = strings.TrimSuffix(key, "/")
	if key == "" {
		responseError(w, http.StatusBadRequest, "MISSING_KEY", "config key required")
		return
	}

	// Get reason from query
	reason := r.URL.Query().Get("reason")
	cachedHash := r.URL.Query().Get("cached_hash")

	// Build access request
	req := configcenter.ConfigAccessRequest{
		AgentID:        claims.SubprocessID,
		RunID:          claims.RunID,
		Key:            key,
		Reason:         reason,
		CachedValueHash: cachedHash,
	}

	value, err := h.center.Get(r.Context(), req)
	if err != nil {
		if cerr, ok := err.(*configcenter.ConfigError); ok {
			switch cerr.Code {
			case configcenter.ErrKeyNotFound:
				responseError(w, http.StatusNotFound, "KEY_NOT_FOUND", cerr.Message)
			case configcenter.ErrAccessDenied, configcenter.ErrRatingDenied:
				responseError(w, http.StatusForbidden, "ACCESS_DENIED", cerr.Message)
			case configcenter.ErrRateLimited:
				responseError(w, http.StatusTooManyRequests, "RATE_LIMITED", cerr.Message)
			case configcenter.ErrValueChanged:
				// Special case: value changed, include extra info
				resp := map[string]interface{}{
					"error": map[string]interface{}{
						"code":    "VALUE_CHANGED",
						"message": cerr.Message,
					},
				}
				if cerr.Extra != nil {
					resp["extra"] = cerr.Extra
				}
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(resp)
			default:
				responseError(w, http.StatusInternalServerError, "CONFIG_ERROR", cerr.Message)
			}
			return
		}
		responseError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	responseJSON(w, http.StatusOK, map[string]interface{}{
		"key":   key,
		"value": value,
	})
}

// handleConfigList handles GET /v1/config and GET /v1/config/.
func (h *ConfigHandler) handleConfigList(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	if claims == nil {
		responseError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no claims")
		return
	}

	// Capability check
	if err := h.capCheck.Check(claims, CapConfigList); err != nil {
		responseError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	entries := h.center.ListAccessible()
	keys := make([]string, len(entries))
	for i, e := range entries {
		keys[i] = e.Key
	}

	responseJSON(w, http.StatusOK, map[string]interface{}{
		"keys": keys,
		"count": len(keys),
	})
}

// handleConfigSearch handles GET /v1/config/search.
func (h *ConfigHandler) handleConfigSearch(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	if claims == nil {
		responseError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no claims")
		return
	}

	// Capability check
	if err := h.capCheck.Check(claims, CapConfigSearch); err != nil {
		responseError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		responseError(w, http.StatusBadRequest, "MISSING_QUERY", "search query required (q param)")
		return
	}

	results := h.center.Search(query, configcenter.SearchOptions{
		Limit: 50,
	})

	// Build response with safe fields
	type SearchResult struct {
		Key         string   `json:"key"`
		Description string   `json:"description,omitempty"`
		Tags        []string `json:"tags,omitempty"`
		Rating      string   `json:"rating,omitempty"`
	}

	sr := make([]SearchResult, len(results))
	for i, e := range results {
		sr[i] = SearchResult{
			Key:         e.Key,
			Description: e.Description,
			Tags:        e.Tags,
			Rating:      string(e.Rating),
		}
	}

	responseJSON(w, http.StatusOK, map[string]interface{}{
		"results": sr,
		"count":   len(sr),
	})
}

// handleConfigSet handles POST /v1/config (for admin/operator use).
func (h *ConfigHandler) handleConfigSet(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	if claims == nil {
		responseError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no claims")
		return
	}

	// This should be restricted to operator/admin capabilities
	// For now, allow if they have config.access (in production, add config.write capability)
	if err := h.capCheck.Check(claims, CapConfigAccess); err != nil {
		responseError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		responseError(w, http.StatusBadRequest, "INVALID_REQUEST", "failed to read body")
		return
	}
	defer r.Body.Close()

	var req ConfigSetRequest
	if err := json.Unmarshal(body, &req); err != nil {
		responseError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON: "+err.Error())
		return
	}

	if req.Key == "" {
		responseError(w, http.StatusBadRequest, "MISSING_KEY", "key is required")
		return
	}

	entry := configcenter.NewConfigEntry(req.Key, req.Value)
	if req.Rating != "" {
		entry.Rating = configcenter.Rating(req.Rating)
	}
	if req.Description != "" {
		entry.Description = req.Description
	}
	if len(req.Tags) > 0 {
		entry.Tags = req.Tags
	}

	if err := h.center.Set(entry); err != nil {
		responseError(w, http.StatusInternalServerError, "SET_FAILED", err.Error())
		return
	}

	responseJSON(w, http.StatusCreated, map[string]interface{}{
		"key":      entry.Key,
		"rating":   string(entry.Rating),
		"version":  entry.Version,
	})
}

// handleConfigAudit handles GET /v1/config/audit.
func (h *ConfigHandler) handleConfigAudit(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	if claims == nil {
		responseError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no claims")
		return
	}

	// This should be restricted to operator/admin
	if err := h.capCheck.Check(claims, CapConfigAccess); err != nil {
		responseError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	// Parse query filters
	opts := configcenter.AuditQuery{
		Limit: 100,
	}
	if agentID := r.URL.Query().Get("agent_id"); agentID != "" {
		opts.AgentID = &agentID
	}
	if key := r.URL.Query().Get("key"); key != "" {
		opts.Key = &key
	}

	if limit := r.URL.Query().Get("limit"); limit != "" {
		fmt.Sscanf(limit, "%d", &opts.Limit)
	}

	entries := h.center.GetAuditLog(opts)

	// Marshal entries
	type AuditEntryResponse struct {
		ID         string `json:"id"`
		Timestamp  int64  `json:"timestamp"`
		AgentID    string `json:"agent_id"`
		Key        string `json:"key"`
		Rating     string `json:"rating"`
		Decision   string `json:"decision"`
		Policy     string `json:"policy"`
		ReasonCode string `json:"reason_code,omitempty"`
		Approver   string `json:"approver,omitempty"`
	}

	resp := make([]AuditEntryResponse, len(entries))
	for i, e := range entries {
		resp[i] = AuditEntryResponse{
			ID:         e.ID,
			Timestamp:  e.Timestamp,
			AgentID:    e.AgentID,
			Key:        e.Key,
			Rating:     string(e.Rating),
			Decision:   string(e.Decision),
			Policy:     e.Policy,
			ReasonCode: e.ReasonCode,
			Approver:   e.Approver,
		}
	}

	responseJSON(w, http.StatusOK, map[string]interface{}{
		"entries": resp,
		"count":   len(resp),
	})
}

// ConfigSetRequest is the request body for setting a config value.
type ConfigSetRequest struct {
	Key         string   `json:"key"`
	Value       string   `json:"value"`
	Rating      string   `json:"rating,omitempty"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}
