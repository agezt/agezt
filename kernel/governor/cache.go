// SPDX-License-Identifier: MIT

package governor

// LLM response cache (M888) — OPT-IN, off by default. An identical
// CompletionRequest within the TTL returns the cached response without a
// provider call: no tokens, no spend, no rate-window slot. Identical
// requests are rarer than they look in chat (any new turn changes the
// messages), but they are common in machine-driven paths — a retried
// workflow step, a re-fired schedule that summarizes the same unchanged
// input, parallel sub-agents asking the same focused question, or a
// regenerate-after-crash replay.
//
// Deliberately OFF by default: an LLM is not a pure function, and some
// surfaces — chat "regenerate" explicitly — WANT a fresh sample for the
// same input. The operator enables it (AGEZT_LLM_CACHE_TTL) when their
// workload's repeat-calls are deterministic re-asks, not resamples. The
// cache only ever serves an EXACT match (model + system + messages + tools
// + knobs), so it can never leak across conversations.
//
// Streaming is not served from the cache: CompleteStream's value is the
// live token feed, and replaying a stored response as one blob would lie
// to the UI. A streaming call still POPULATES nothing — only Complete
// reads and writes the cache.

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
)

// DefaultResponseCacheSize bounds the LRU entry count when the cache is
// enabled and Config.ResponseCacheSize is unset. Each entry holds one
// assembled response (typically a few KB); 256 keeps the worst case small.
const DefaultResponseCacheSize = 256

// respCache is a TTL'd LRU keyed by the request fingerprint.
type respCache struct {
	mu    sync.Mutex
	ttl   time.Duration
	max   int
	ll    *list.List // front = most recently used
	items map[string]*list.Element
	now   func() time.Time
}

type cacheEntry struct {
	key  string
	resp agent.CompletionResponse
	at   time.Time
}

func newRespCache(ttl time.Duration, size int, now func() time.Time) *respCache {
	if size <= 0 {
		size = DefaultResponseCacheSize
	}
	if now == nil {
		now = time.Now
	}
	return &respCache{ttl: ttl, max: size, ll: list.New(), items: make(map[string]*list.Element), now: now}
}

// cacheKey fingerprints everything that shapes a completion. Two requests
// with the same key would be served identically by a deterministic provider.
func cacheKey(req agent.CompletionRequest) string {
	h := sha256.New()
	_ = json.NewEncoder(h).Encode(struct {
		Model           string
		System          string
		Messages        []agent.Message
		Tools           []agent.ToolDef
		MaxTokens       int
		JSONMode        bool
		TaskType        string
		Params          agent.Params
		ProviderOptions map[string]json.RawMessage
	}{req.Model, req.System, req.Messages, req.Tools, req.MaxTokens, req.JSONMode, req.TaskType, req.Params, req.ProviderOptions})
	return hex.EncodeToString(h.Sum(nil))
}

// get returns a copy of the cached response for key, expiring stale entries.
func (c *respCache) get(key string) (*agent.CompletionResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	ent := el.Value.(*cacheEntry)
	if c.ttl > 0 && c.now().Sub(ent.at) > c.ttl {
		c.ll.Remove(el)
		delete(c.items, key)
		return nil, false
	}
	c.ll.MoveToFront(el)
	resp := ent.resp // copy; callers treat responses as read-only
	return &resp, true
}

// put stores resp under key, evicting the least-recently-used entry past max.
func (c *respCache) put(key string, resp agent.CompletionResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*cacheEntry).resp = resp
		el.Value.(*cacheEntry).at = c.now()
		c.ll.MoveToFront(el)
		return
	}
	c.items[key] = c.ll.PushFront(&cacheEntry{key: key, resp: resp, at: c.now()})
	for c.ll.Len() > c.max {
		oldest := c.ll.Back()
		c.ll.Remove(oldest)
		delete(c.items, oldest.Value.(*cacheEntry).key)
	}
}
