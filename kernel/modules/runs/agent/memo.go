// SPDX-License-Identifier: MIT

package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// DefaultToolMemoTTL bounds per-run read-only tool memoization.
const DefaultToolMemoTTL = 5 * time.Minute

// DefaultToolMemoMaxEntries bounds per-run memoized tool results.
const DefaultToolMemoMaxEntries = 256

// ToolMemo caches successful read-only tool results within a bounded scope. Keys
// are SHA-256 digests of tool name + raw input, so sensitive args never become
// cache keys in memory dumps or future diagnostics.
type ToolMemo struct {
	mu      sync.Mutex
	ttl     time.Duration
	max     int
	entries map[string]memoEntry
	order   []string
	now     func() time.Time
}

type memoEntry struct {
	result    Result
	expiresAt time.Time
}

// NewToolMemo creates a bounded memo cache. Non-positive ttl/max values use
// conservative defaults.
func NewToolMemo(ttl time.Duration, max int) *ToolMemo {
	if ttl <= 0 {
		ttl = DefaultToolMemoTTL
	}
	if max <= 0 {
		max = DefaultToolMemoMaxEntries
	}
	return &ToolMemo{
		ttl:     ttl,
		max:     max,
		entries: map[string]memoEntry{},
		now:     time.Now,
	}
}

func (m *ToolMemo) Get(tool string, input json.RawMessage) (Result, bool) {
	if m == nil {
		return Result{}, false
	}
	key := memoKey(tool, input)
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.entries[key]
	if !ok {
		return Result{}, false
	}
	if !entry.expiresAt.After(now) {
		delete(m.entries, key)
		m.removeOrderKey(key)
		return Result{}, false
	}
	return entry.result, true
}

func (m *ToolMemo) Set(tool string, input json.RawMessage, result Result) {
	if m == nil || result.IsError {
		return
	}
	key := memoKey(tool, input)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeOrderKey(key)
	m.order = append(m.order, key)
	m.entries[key] = memoEntry{result: result, expiresAt: m.now().Add(m.ttl)}
	for len(m.entries) > m.max && len(m.order) > 0 {
		oldest := m.order[0]
		m.order = m.order[1:]
		delete(m.entries, oldest)
	}
}

func (m *ToolMemo) removeOrderKey(key string) {
	for i := 0; i < len(m.order); {
		if m.order[i] == key {
			m.order = append(m.order[:i], m.order[i+1:]...)
			continue
		}
		i++
	}
}

func memoKey(tool string, input json.RawMessage) string {
	h := sha256.New()
	_, _ = h.Write([]byte(tool))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(input)
	return hex.EncodeToString(h.Sum(nil))
}
