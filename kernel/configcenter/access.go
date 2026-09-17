// SPDX-License-Identifier: MIT

package configcenter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/approval"
)


// AccessPolicy evaluates access requests against ratings and policies.
type AccessPolicy struct {
	config     *Config
	store      *Store
	classifier *SecretClassifier
	auditLog   *AuditLogger
	registry   *approval.Registry // nil means HITL not available

	// Rate limiting
	rateLimits *RateLimitMap
}

// RateLimitMap tracks access counts for rate limiting.
type RateLimitMap struct {
	mu     sync.Mutex
	agents map[string]*timeWindow // agent -> count
	keys   map[string]*timeWindow // key -> count
	window time.Duration
}

// timeWindow tracks events within a time window.
type timeWindow struct {
	mu       sync.Mutex
	counts   []time.Time
	window   time.Duration
	maxCount int
}

// newTimeWindow creates a new time window tracker.
func newTimeWindow(window time.Duration, maxCount int) *timeWindow {
	return &timeWindow{
		counts:   make([]time.Time, 0),
		window:   window,
		maxCount: maxCount,
	}
}

// Allow checks if a new event is allowed within the time window.
func (tw *timeWindow) Allow() bool {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-tw.window)

	// Remove old entries
	newCounts := make([]time.Time, 0, len(tw.counts))
	for _, t := range tw.counts {
		if t.After(cutoff) {
			newCounts = append(newCounts, t)
		}
	}
	tw.counts = newCounts

	// Check if we're at the limit
	if len(tw.counts) >= tw.maxCount {
		return false
	}

	// Add new event
	tw.counts = append(tw.counts, now)
	return true
}

// NewAccessPolicy creates a new access policy evaluator.
func NewAccessPolicy(cfg *Config, store *Store, auditLog *AuditLogger) *AccessPolicy {
	classifier := NewSecretClassifier()
	for key, rating := range cfg.Classifier.Overrides {
		classifier.SetOverride(key, rating)
	}

	return &AccessPolicy{
		config:     cfg,
		store:      store,
		classifier: classifier,
		auditLog:   auditLog,
		rateLimits: &RateLimitMap{
			agents: make(map[string]*timeWindow),
			keys:   make(map[string]*timeWindow),
			window: time.Minute,
		},
	}
}

// SetRegistry sets the approval registry for HITL support.
func (ap *AccessPolicy) SetRegistry(reg *approval.Registry) {
	ap.registry = reg
}

// Evaluate checks if an access request should be allowed.
func (ap *AccessPolicy) Evaluate(ctx context.Context, req *ConfigAccessRequest) (*ConfigAccessResponse, error) {
	// 1. Rate limit check - per agent
	if !ap.checkRateLimit(req.AgentID, true) {
		ap.auditLog.Log(req, AccessDenied, "rate_limit", "agent_rate_exceeded", "")
		return &ConfigAccessResponse{
			Decision: AccessDenied,
			Reason:   fmt.Sprintf("rate limit exceeded (%d/min)", ap.config.RateLimits.PerAgentPerMinute),
		}, nil
	}

	// 2. Rate limit check - per key
	if !ap.checkRateLimit(req.Key, false) {
		ap.auditLog.Log(req, AccessDenied, "rate_limit", "key_rate_exceeded", "")
		return &ConfigAccessResponse{
			Decision: AccessDenied,
			Reason:   fmt.Sprintf("rate limit exceeded for key (%d/min)", ap.config.RateLimits.PerKeyPerMinute),
		}, nil
	}

	// 3. Lookup entry
	entry, err := ap.store.Get(req.Key)
	if err != nil {
		return &ConfigAccessResponse{
			Decision: AccessDenied,
			Reason:   err.Error(),
		}, err
	}

	// 4. Agent ID restrictions
	if len(entry.AllowedAgents) > 0 {
		allowed := false
		for _, agent := range entry.AllowedAgents {
			if agent == req.AgentID {
				allowed = true
				break
			}
		}
		if !allowed {
			ap.auditLog.Log(req, AccessDenied, "deny", "agent_not_in_allowed_list", "")
			return &ConfigAccessResponse{
				Decision: AccessDenied,
				Reason:   fmt.Sprintf("agent %q not allowed to access this config", req.AgentID),
			}, nil
		}
	}

	// 5. Agent ID exclusions
	for _, agent := range entry.ExcludedAgents {
		if agent == req.AgentID {
			ap.auditLog.Log(req, AccessDenied, "deny", "agent_in_excluded_list", "")
			return &ConfigAccessResponse{
				Decision: AccessDenied,
				Reason:   fmt.Sprintf("agent %q explicitly excluded from this config", req.AgentID),
			}, nil
		}
	}

	// 6. Determine rating (use stored or auto-classify)
	rating := entry.Rating
	if rating == "" {
		rating = ap.classifier.Classify(req.Key, entry.Value)
	}

	// 7. Check if value changed (for cached values)
	if req.CachedValueHash != "" && entry.ValueHash != "" {
		if req.CachedValueHash != entry.ValueHash {
			return &ConfigAccessResponse{
				Decision: AccessDenied,
				Reason:   "config value has changed since last access, please re-fetch",
				Extra: map[string]string{
					"old_hash": req.CachedValueHash,
					"new_hash": entry.ValueHash,
				},
			}, nil
		}
	}

	// 8. Resolve effective policy
	policy := ap.resolvePolicy(entry, rating)

	// 9. Evaluate policy
	switch policy {
	case PolicyAuto:
		// Auto-allow with logging
		ap.auditLog.Log(req, AccessAllowed, "auto", "", entry.Value)
		return &ConfigAccessResponse{
			Decision: AccessAllowed,
			Value:    entry.Value,
			Rating:   rating,
		}, nil

	case PolicyDeny, PolicySecretDeny:
		// Always deny
		ap.auditLog.Log(req, AccessDenied, string(policy), fmt.Sprintf("rating=%s", rating), "")
		return &ConfigAccessResponse{
			Decision: AccessDenied,
			Reason:   fmt.Sprintf("access denied: rating=%s", rating),
		}, nil

	case PolicyHITL:
		// Human-in-the-loop approval required
		return ap.requestHITLApproval(ctx, req, entry, rating)
	}

	return &ConfigAccessResponse{
		Decision: AccessDenied,
		Reason:   "unknown policy",
	}, nil
}
