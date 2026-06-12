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
	"sync/atomic"
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
		CapConfigAccess, CapConfigList, CapConfigSearch,
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

// RateLimit tracks request counts for a token.
type RateLimit struct {
	mu       int64 // atomic count
	max      int
	burst    int
	windowMs int64
	lastTick int64
}

// NewRateLimit creates a new rate limiter.
func NewRateLimit(maxRPM, maxBurst int) *RateLimit {
	return &RateLimit{
		max:      maxRPM,
		burst:    maxBurst,
		windowMs: 60_000,
		lastTick: time.Now().UnixMilli(),
	}
}

// Allow checks if a request is allowed under the rate limit.
// Returns true if allowed, false if rate limited.
func (r *RateLimit) Allow() bool {
	now := time.Now().UnixMilli()

	// Reset window if we've passed a full window
	if now-r.lastTick >= r.windowMs {
		// Can't use atomic swap for int64 easily, so use sync
		return true
	}

	// Check count
	count := atomicLoadInt64(&r.mu)
	if count >= int64(r.max+r.burst) {
		return false
	}

	atomicAddInt64(&r.mu, 1)
	return true
}

// atomicAddInt64 adds delta to a *int64 atomically.
func atomicAddInt64(addr *int64, delta int64) int64 {
	return atomic.AddInt64(addr, delta)
}

// atomicLoadInt64 loads a *int64 atomically.
func atomicLoadInt64(addr *int64) int64 {
	return atomic.LoadInt64(addr)
}
