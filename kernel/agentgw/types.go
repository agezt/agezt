// SPDX-License-Identifier: MIT

// Package agentgw provides a secure gateway for AI agent subprocess code
// to communicate with the AGEZT kernel. It exposes scoped HTTP/WebSocket
// endpoints validated by JWT capability tokens.
//
// Capability namespaces exposed to agent code:
//   - eventbus: publish, subscribe
//   - channel: send, read, list
//   - memory: read, write, delete, search, list
//   - log: read, write
//   - agent: list, query
//   - db: query, read, write
package agentgw

import (
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/edict"
)

// TokenClaims represents the JWT claims for an agent subprocess token.
type TokenClaims struct {
	// RunID is the correlation ID of the parent agent run.
	RunID string `json:"run_id"`
	// Caps is the list of capabilities granted to this token.
	Caps []string `json:"caps"`
	// MaxRate is the maximum requests per minute allowed.
	MaxRate int `json:"max_rpm"`
	// MaxBurst allows burst requests beyond MaxRate.
	MaxBurst int `json:"max_burst"`
	// ExpiresAt is when the token expires.
	ExpiresAt time.Time `json:"exp"`
	// Issuer ("iss") identifies the minter; pinned to TokenIssuer and verified
	// on validation so a token minted in another context can't be replayed here.
	Issuer string `json:"iss,omitempty"`
	// Audience ("aud") identifies the intended consumer; pinned to TokenAudience
	// and verified on validation.
	Audience string `json:"aud,omitempty"`
	// TokenID is the unique identifier for this token (set by CreateToken).
	TokenID string `json:"tid,omitempty"`
	// ParentTokenID is the ID of the parent token (if this is a subprocess token).
	ParentTokenID string `json:"parent_tid,omitempty"`
	// SubprocessID is the ID of the subprocess this token is for.
	SubprocessID string `json:"sub_id,omitempty"`
}

// HasCap reports whether the token has the given capability.
func (c *TokenClaims) HasCap(cap edict.Capability) bool {
	for _, p := range c.Caps {
		if edict.Capability(p) == cap {
			return true
		}
	}
	return false
}

// HasAnyCap reports whether the token has any of the given capabilities.
func (c *TokenClaims) HasAnyCap(caps ...edict.Capability) bool {
	for _, cap := range caps {
		if c.HasCap(cap) {
			return true
		}
	}
	return false
}

// AgentCapability is a capability namespace exposed to agent subprocess code.
type AgentCapability string

const (
	// CapEventbusPublish allows publishing events to the bus.
	CapEventbusPublish AgentCapability = "eventbus.publish"
	// CapEventbusSubscribe allows subscribing to bus events.
	CapEventbusSubscribe AgentCapability = "eventbus.subscribe"
	// CapChannelSend allows sending on channels.
	CapChannelSend AgentCapability = "channel.send"
	// CapChannelRead allows reading from channels.
	CapChannelRead AgentCapability = "channel.read"
	// CapChannelList allows listing channels.
	CapChannelList AgentCapability = "channel.list"
	// CapMemoryRead allows reading from memory.
	CapMemoryRead AgentCapability = "memory.read"
	// CapMemoryWrite allows writing to memory.
	CapMemoryWrite AgentCapability = "memory.write"
	// CapMemoryDelete allows deleting from memory.
	CapMemoryDelete AgentCapability = "memory.delete"
	// CapMemorySearch allows searching memory.
	CapMemorySearch AgentCapability = "memory.search"
	// CapMemoryList allows listing memory records.
	CapMemoryList AgentCapability = "memory.list"
	// CapLogRead allows reading logs.
	CapLogRead AgentCapability = "log.read"
	// CapLogWrite allows writing logs.
	CapLogWrite AgentCapability = "log.write"
	// CapAgentList allows listing agents.
	CapAgentList AgentCapability = "agent.list"
	// CapAgentQuery allows querying agent status.
	CapAgentQuery AgentCapability = "agent.query"
	// CapDBQuery allows querying the database.
	CapDBQuery AgentCapability = "db.query"
	// CapDBRead allows reading from the database.
	CapDBRead AgentCapability = "db.read"
	// CapDBWrite allows writing to the database.
	CapDBWrite AgentCapability = "db.write"

	// Config capabilities.
	CapConfigAccess AgentCapability = "config.access" // Get config values (rating-based)
	CapConfigList   AgentCapability = "config.list"   // List accessible config keys
	CapConfigSearch AgentCapability = "config.search" // Search config keys
	CapConfigWrite  AgentCapability = "config.write"  // Set/modify config values (privileged)
)

// AllAgentCaps returns all available agent capabilities.
func AllAgentCaps() []AgentCapability {
	return []AgentCapability{
		CapEventbusPublish, CapEventbusSubscribe,
		CapChannelSend, CapChannelRead, CapChannelList,
		CapMemoryRead, CapMemoryWrite, CapMemoryDelete, CapMemorySearch, CapMemoryList,
		CapLogRead, CapLogWrite,
		CapAgentList, CapAgentQuery,
		CapDBQuery, CapDBRead, CapDBWrite,
		CapConfigAccess, CapConfigList, CapConfigSearch, CapConfigWrite,
	}
}

// AuditEntry records one capability access event.
type AuditEntry struct {
	Timestamp  time.Time `json:"ts"`
	TokenID    string    `json:"tid"`
	RunID      string    `json:"run_id"`
	Subprocess string    `json:"sub_id,omitempty"`
	Capability string    `json:"cap"`
	Operation  string    `json:"op"`
	Path       string    `json:"path,omitempty"`
	Success    bool      `json:"ok"`
	Error      string    `json:"err,omitempty"`
	DurationMs int64     `json:"dur_ms"`
	ClientIP   string    `json:"ip,omitempty"`
}

// RateLimit tracks request counts for a token within a sliding fixed window.
type RateLimit struct {
	mu        sync.Mutex
	count     int   // requests in the current window
	max       int   // sustained requests per window
	burst     int   // extra requests allowed on top of max
	windowMs  int64 // window length
	windowEnd int64 // unix-ms when the current window rolls over
	lastSeen  int64 // unix-ms of the most recent Allow() (for idle eviction)
}

// NewRateLimit creates a new rate limiter.
func NewRateLimit(maxRPM, maxBurst int) *RateLimit {
	now := time.Now().UnixMilli()
	return &RateLimit{
		max:       maxRPM,
		burst:     maxBurst,
		windowMs:  60_000,
		windowEnd: now + 60_000,
		lastSeen:  now,
	}
}

// Allow reports whether a request is permitted, counting it when so. On window
// rollover the counter resets — unlike the previous implementation, which
// returned true unconditionally after the first window and silently disabled
// the limiter (CWE-770).
func (r *RateLimit) Allow() bool {
	now := time.Now().UnixMilli()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastSeen = now
	if now >= r.windowEnd {
		r.count = 0
		r.windowEnd = now + r.windowMs
	}
	if r.count >= r.max+r.burst {
		return false
	}
	r.count++
	return true
}

// LastSeen returns the unix-ms timestamp of the most recent Allow() call, used
// to evict idle buckets.
func (r *RateLimit) LastSeen() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastSeen
}
