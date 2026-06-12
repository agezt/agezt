// SPDX-License-Identifier: MIT

package configcenter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
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

// resolvePolicy determines the effective policy for an entry.
func (ap *AccessPolicy) resolvePolicy(entry *ConfigEntry, rating Rating) Policy {
	// Entry-level override
	if entry.AccessPolicy != "" {
		return entry.AccessPolicy
	}

	// Rating-based default
	if policy, ok := ap.config.AccessPolicies[rating]; ok {
		return policy
	}

	// Fallback
	return PolicyDeny
}

// checkRateLimit checks and updates rate limit for an agent or key.
func (ap *AccessPolicy) checkRateLimit(id string, isAgent bool) bool {
	ap.rateLimits.mu.Lock()
	defer ap.rateLimits.mu.Unlock()

	var tw *timeWindow
	var limits map[string]*timeWindow
	var limit int

	if isAgent {
		limits = ap.rateLimits.agents
		limit = ap.config.RateLimits.PerAgentPerMinute
	} else {
		limits = ap.rateLimits.keys
		limit = ap.config.RateLimits.PerKeyPerMinute
	}

	if tw, ok := limits[id]; ok {
		return tw.Allow()
	}

	// Create new tracker
	tw = newTimeWindow(time.Minute, limit)
	limits[id] = tw
	return tw.Allow()
}

// requestHITLApproval requests operator approval for restricted config access.
func (ap *AccessPolicy) requestHITLApproval(ctx context.Context, req *ConfigAccessRequest, entry *ConfigEntry, rating Rating) (*ConfigAccessResponse, error) {
	// If no registry is configured, deny
	if ap.registry == nil {
		ap.auditLog.Log(req, AccessDenied, "hitl", "approval_not_configured", "")
		return &ConfigAccessResponse{
			Decision: AccessDenied,
			Reason:   "access requires operator approval, but approval system is not available",
		}, nil
	}

	// Determine auto-recommendation for operator (used in approval metadata)
	autoRec := "allow"
	if rating == RatingSecret {
		autoRec = "deny"
	}
	_ = autoRec // TODO: Pass to approval system metadata

	// Create value preview (last 4 chars for secrets)
	valuePreview := ap.maskValue(entry.Value)
	_ = valuePreview // TODO: Include in approval metadata

	// Submit approval request
	outcome := ap.registry.Submit(ctx, approval.SubmitSpec{
		Capability:    "config.access",
		ToolName:      "config.get",
		Input:         fmt.Sprintf("key=%s rating=%s", req.Key, rating),
		Reason:        req.Reason,
		Actor:         "config_center",
		CorrelationID: req.RunID,
	})

	if outcome.Decision == approval.DecisionGrant {
		ap.auditLog.Log(req, AccessAllowed, "hitl", "operator_grant", entry.Value)
		return &ConfigAccessResponse{
			Decision:   AccessAllowed,
			Value:      entry.Value,
			Rating:     rating,
			Approver:   outcome.ResolvedBy,
			ApprovalID: fmt.Sprintf("config:%s:%s", req.AgentID, req.Key),
		}, nil
	}

	// Denied, timeout, or cancelled
	reason := outcome.Reason
	if reason == "" {
		switch outcome.Decision {
		case approval.DecisionTimeout:
			reason = "operator approval timeout"
		case approval.DecisionCancel:
			reason = "request cancelled"
		default:
			reason = "operator denied"
		}
	}

	ap.auditLog.Log(req, AccessDenied, "hitl", string(outcome.Decision), "")
	return &ConfigAccessResponse{
		Decision: AccessDenied,
		Reason:   reason,
	}, nil
}

// maskValue returns a masked preview of a secret value.
func (ap *AccessPolicy) maskValue(value string) string {
	if len(value) <= 4 {
		return strings.Repeat("*", len(value))
	}
	return strings.Repeat("*", len(value)-4) + value[len(value)-4:]
}

// HashValue computes a SHA256 hash of a value.
func HashValue(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
