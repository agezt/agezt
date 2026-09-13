// SPDX-License-Identifier: MIT

package webhook

// Webhook channel helpers: sign + writeJSON + dedup + newDedup +
// seenBefore. Carved out of webhook.go during the Day 192 god-file
// split so the main file can stay focused on types + lifecycle +
// inbound handleInbound and the ops file can stay focused on
// outbound + signature verification.
// Public API unchanged.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// dedup is a small bounded set of recently-seen message ids (replay guard). It
// keeps two generations (live + previous) so eviction never forgets every id in
// one shot: a key is dropped only after it ages out of BOTH, bounding memory at
// 2×cap while roughly doubling the replay window's coverage.
type dedup struct {
	mu   sync.Mutex
	seen map[string]struct{}
	prev map[string]struct{}
	cap  int
}

func newDedup(capacity int) *dedup {
	return &dedup{seen: make(map[string]struct{}, capacity), cap: capacity}
}

// seenBefore records key and reports whether it had been seen already (in either
// generation). When the live set fills it rotates to become the previous
// generation and a fresh live set starts — unlike a wholesale clear, which would
// forget every recently-seen id at once and let a captured signed body replay
// (the freshness window only guards replays when the client sends ts_ms; with no
// timestamp this set is the sole replay guard, so it must not flush so coarsely).
func (d *dedup) seenBefore(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.seen[key]; ok {
		return true
	}
	if _, ok := d.prev[key]; ok {
		return true
	}
	if len(d.seen) >= d.cap {
		d.prev = d.seen
		d.seen = make(map[string]struct{}, d.cap)
	}
	d.seen[key] = struct{}{}
	return false
}

