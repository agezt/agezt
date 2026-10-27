// SPDX-License-Identifier: MIT

package agentgw

// Memory handlers: handleMemoryWrite + handleMemorySearch +
// memorySearchLimit + handleMemoryDelete. Carved out of
// handlers.go during the Day 194 god-file split so the main file
// can stay focused on eventbus handlers and the log file can stay
// focused on log + agent handlers.
// Public API unchanged.

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/contract/memoryapi"
)

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

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		responseError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if g.mem == nil {
		responseError(w, http.StatusServiceUnavailable, "MEMORY_UNAVAILABLE", "memory manager not connected")
		return
	}

	// The memory's type vocabulary is upper-case ("FACT", "SUMMARY"), and the
	// contract validates it strictly — so a client that wrote "fact" got a
	// 500 WRITE_ERROR for a type that exists. The gateway is the transport:
	// it canonicalises the spelling it accepts.
	recType := memoryRecordType(req.Type)

	spec := memoryapi.RememberSpec{
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

// memoryRecordType canonicalises the type a client wrote into the memory's
// own vocabulary.
//
// The contract's types are upper-case ("FACT", "SUMMARY") and the memory
// validates them strictly, so a client that wrote "fact" — the spelling the
// type's NAME uses — was refused with a 500 WRITE_ERROR for a type that
// exists. The gateway is the transport: it accepts the spelling and
// canonicalises it. Split out of the handler so the mapping is assertable
// without a memory manager behind it.
func memoryRecordType(raw string) memoryapi.Type {
	canonical := memoryapi.Type(strings.ToUpper(strings.TrimSpace(raw)))
	if canonical == "" {
		return memoryapi.TypeFact
	}
	return canonical
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

	limit := memorySearchLimit(r.URL.Query().Get("limit"))

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

func memorySearchLimit(raw string) int {
	if raw == "" {
		return defaultMemorySearchLimit
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return defaultMemorySearchLimit
	}
	switch {
	case parsed < 1:
		return 1
	case parsed > maxMemorySearchLimit:
		return maxMemorySearchLimit
	default:
		return parsed
	}
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
