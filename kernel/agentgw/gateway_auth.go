// SPDX-License-Identifier: MIT

// Agent gateway: auth + ratelimit + response helpers + first handlers.
// Code extracted from gateway.go during the Day-84 god-file split.
// Public API unchanged.
package agentgw


import (
	"context"
	"fmt"
	"time"

	"encoding/json"
	"net/http"
)

func (g *Gateway) withAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract token from Authorization header
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, `{"error":"unauthorized","message":"missing token"}`, http.StatusUnauthorized)
			return
		}

		// Validate token
		claims, err := g.tokenMgr.ValidateToken(token)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"unauthorized","message":"%v"}`, err), http.StatusUnauthorized)
			return
		}

		// Check rate limit. Keyed by the token's unique TokenID, not
		// SubprocessID: top-level tokens all carry an empty SubprocessID
		// (the `agt token create` mint never sets one) and a bucket keeps
		// its creator's limits, so a shared key would let a tighter-limited
		// token ride a looser token's bucket (limit bypass) or throttle a
		// loose token on a tight one's budget.
		if !g.allowRate(claims.TokenID, claims.MaxRate, claims.MaxBurst) {
			http.Error(w, `{"error":"rate_limited","message":"too many requests"}`, http.StatusTooManyRequests)
			return
		}

		// Record the authorized access (audit trail).
		g.auditAccess(r, claims)

		// Store claims in request context
		ctx := context.WithValue(r.Context(), claimsKey{}, claims)
		handler(w, r.WithContext(ctx))
	}
}

// auditAccess records one authorized gateway request to the journal (no-op
// until SetAuditJournal wires a real journal).
func (g *Gateway) auditAccess(r *http.Request, claims *TokenClaims) {
	if g.auditLog == nil {
		return
	}
	g.auditLog.Log(AuditEntry{
		Timestamp:  time.Now(),
		TokenID:    claims.ParentTokenID,
		RunID:      claims.RunID,
		Subprocess: claims.SubprocessID,
		Operation:  r.Method,
		Path:       r.URL.Path,
		Success:    true,
		ClientIP:   r.RemoteAddr,
	})
}

// maxRateLimitEntries bounds the per-token rate-limit map so a flood of
// distinct subprocess IDs cannot exhaust memory (CWE-770). When the cap is hit
// we evict idle entries first, then (if all are fresh) drop one to make room.
const maxRateLimitEntries = 4096

// rateLimitIdleEvict is how long a rate-limit bucket may sit unused before it
// becomes eligible for eviction.
const rateLimitIdleEvictMs = 5 * 60_000 // 5 minutes

// allowRate checks and updates rate limit for a token.
func (g *Gateway) allowRate(tid string, maxRate, maxBurst int) bool {
	g.rlMu.Lock()
	rl, ok := g.rateLimit[tid]
	if !ok {
		if len(g.rateLimit) >= maxRateLimitEntries {
			g.evictStaleLocked()
		}
		rl = NewRateLimit(maxRate, maxBurst)
		g.rateLimit[tid] = rl
	}
	// Call Allow() while holding rlMu to prevent a race where two concurrent
	// requests both see count=0 before either has incremented.
	allowed := rl.Allow()
	g.rlMu.Unlock()
	return allowed
}

// evictStaleLocked removes idle rate-limit buckets. Caller must hold rlMu.
func (g *Gateway) evictStaleLocked() {
	cutoff := time.Now().UnixMilli() - rateLimitIdleEvictMs
	for k, rl := range g.rateLimit {
		if rl.LastSeen() < cutoff {
			delete(g.rateLimit, k)
		}
	}
	// All buckets still fresh: drop one arbitrary entry to bound memory.
	if len(g.rateLimit) >= maxRateLimitEntries {
		for k := range g.rateLimit {
			delete(g.rateLimit, k)
			break
		}
	}
}

// extractBearerToken extracts the token from the Authorization header.
func extractBearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && h[:7] == "Bearer " {
		return h[7:]
	}
	return ""
}

// claimsKey is the context key for token claims.
type claimsKey struct{}

// getClaims extracts token claims from the request context.
func getClaims(r *http.Request) *TokenClaims {
	if claims, ok := r.Context().Value(claimsKey{}).(*TokenClaims); ok {
		return claims
	}
	return nil
}

// responseError writes a JSON error response.
func responseError(w http.ResponseWriter, code int, errCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"code":    errCode,
			"message": message,
		},
	})
}

// responseJSON writes a JSON response.
func responseJSON[T any](w http.ResponseWriter, status int, data T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// handleHealth handles health check requests.
func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	responseJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

// handleTokenCreate mints a SUBPROCESS token derived from the caller's
// (authenticated) parent token. It runs behind withAuth, so getClaims is the
// parent. The minted token can never exceed the parent: capabilities are
// intersected with the parent's, expiry is clamped to the parent's, and the
// RunID is inherited (a child cannot mint into a different run). This closes
// the unauthenticated-mint / capability-escalation hole.
func (g *Gateway) handleTokenCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	parent := getClaims(r)
	if parent == nil {
		responseError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no claims")
		return
	}

	var req struct {
		SubID    string   `json:"sub_id"`
		Caps     []string `json:"caps"`
		MaxRate  int      `json:"max_rpm"`
		MaxBurst int      `json:"max_burst"`
		ExpiryMs int64    `json:"expiry_ms"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		responseError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	// Validate and normalize requested capabilities.
	caps, err := NormalizeCaps(req.Caps)
	if err != nil {
		responseError(w, http.StatusBadRequest, "INVALID_CAPABILITY", err.Error())
		return
	}
	// Reject (don't silently drop) any capability the parent lacks — a child
	// token must be a subset of its parent.
	if missing := CapsSubset(caps, parent.Caps); len(missing) > 0 {
		responseError(w, http.StatusForbidden, "CAP_ESCALATION",
			fmt.Sprintf("requested capabilities exceed parent grant: %v", missing))
		return
	}
	if len(caps) == 0 {
		// Default: inherit the full parent capability set.
		caps = append([]string(nil), parent.Caps...)
	}

	expiry := time.Duration(req.ExpiryMs) * time.Millisecond
	if expiry <= 0 {
		expiry = 10 * time.Minute
	}
	exp := time.Now().Add(expiry)
	if !parent.ExpiresAt.IsZero() && exp.After(parent.ExpiresAt) {
		exp = parent.ExpiresAt // never outlive the parent
	}

	// Clamp rate limits to the parent's (0 == inherit).
	maxRate := req.MaxRate
	if maxRate <= 0 || (parent.MaxRate > 0 && maxRate > parent.MaxRate) {
		maxRate = parent.MaxRate
	}
	maxBurst := req.MaxBurst
	if maxBurst <= 0 || (parent.MaxBurst > 0 && maxBurst > parent.MaxBurst) {
		maxBurst = parent.MaxBurst
	}

	claims := &TokenClaims{
		RunID:         parent.RunID, // inherited — cannot mint into another run
		Caps:          caps,
		MaxRate:       maxRate,
		MaxBurst:      maxBurst,
		ExpiresAt:     exp,
		ParentTokenID: parent.TokenID, // the parent TOKEN, not its run — all children of a
		// run share one RunID, so recording that here loses token-level
		// attribution in auditAccess (JWT-002). CreateSubprocessToken, the same
		// operation in the library, has always used parent.TokenID.
		SubprocessID: req.SubID,
	}

	token, err := g.tokenMgr.CreateToken(claims)
	if err != nil {
		responseError(w, http.StatusInternalServerError, "TOKEN_ERROR", err.Error())
		return
	}

	responseJSON(w, http.StatusCreated, map[string]string{"token": token})
}
