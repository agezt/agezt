// SPDX-License-Identifier: MIT

// access_helpers.go: resolvePolicy / checkRateLimit / requestHITLApproval /
// maskValue / HashValue split off from access.go during the Day 211 god-file
// refactor (#135). Public API unchanged.
package configcenter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/approval"
)


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

	// Create value preview (last 4 chars for secrets)
	valuePreview := ap.maskValue(entry.Value)

	// Submit approval request
	outcome := ap.registry.Submit(ctx, approval.SubmitSpec{
		Capability:    "config.access",
		ToolName:      "config.get",
		Input:         fmt.Sprintf("key=%s rating=%s", req.Key, rating),
		Reason:        req.Reason,
		Actor:         "config_center",
		CorrelationID: req.RunID,
		AutoRec:       autoRec,
		ValuePreview:  valuePreview,
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
