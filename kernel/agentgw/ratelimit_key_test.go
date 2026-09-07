package agentgw

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// withAuth must key its rate-limit buckets by the token's unique TokenID.
// Keying by SubprocessID couples every token sharing an ID — top-level tokens
// all carry the zero SubprocessID (the `agt token create` mint never sets
// one) — and the bucket keeps its creator's limits: a later, tighter-limited
// token rides the loose bucket (limit bypass), or the loose token is
// throttled on the tight one's budget.
//
// RateLimit semantics: a token's per-window budget is max + burst.
func TestWithAuth_RateLimitsPerTokenIdentity(t *testing.T) {
	g := NewGateway(GatewayConfig{TokenSecret: []byte("test-secret-key-32-chars-minimum!!")})
	handler := g.withAuth(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	mint := func(rate, burst int) string {
		tok, err := g.tokenMgr.CreateToken(&TokenClaims{
			RunID:     "run-rl",
			Caps:      []string{"memory.read"},
			MaxRate:   rate,
			MaxBurst:  burst,
			ExpiresAt: time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("mint token: %v", err)
		}
		return tok
	}
	call := func(tok string) int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/memory", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		handler(rec, req)
		return rec.Code
	}

	loose := mint(1000, 1000) // budget 2000/window
	tight := mint(1, 1)       // budget 2/window

	if code := call(loose); code != http.StatusOK {
		t.Fatalf("loose token's first request: got %d, want 200", code)
	}
	if code := call(tight); code != http.StatusOK {
		t.Fatalf("tight token's first request: got %d, want 200", code)
	}
	// The tight token's second request is still within its own 2/window budget.
	if code := call(tight); code != http.StatusOK {
		t.Fatalf("tight token's second request: got %d, want 200", code)
	}
	// The tight token's own budget (2/window) is now spent: its third request
	// must be 429 even though a bucket shared with the loose token would still
	// hold the 2000-request budget.
	if code := call(tight); code != http.StatusTooManyRequests {
		t.Fatalf("tight token's third request: got %d, want 429 — it must not ride the loose token's 2000/window bucket", code)
	}
	// The loose token keeps its own budget.
	if code := call(loose); code != http.StatusOK {
		t.Fatalf("loose token throttled by the tight token's contract: got %d, want 200", code)
	}
}

// Two subprocess tokens that re-use the same SubprocessID across runs are
// still distinct tokens with distinct contracts: neither may inherit the
// other's limits.
func TestWithAuth_SameSubID_DistinctTokens(t *testing.T) {
	g := NewGateway(GatewayConfig{TokenSecret: []byte("test-secret-key-32-chars-minimum!!")})
	handler := g.withAuth(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	mint := func(sub string, rate, burst int) string {
		tok, err := g.tokenMgr.CreateToken(&TokenClaims{
			RunID:        "run-rl",
			SubprocessID: sub,
			Caps:         []string{"memory.read"},
			MaxRate:      rate,
			MaxBurst:     burst,
			ExpiresAt:    time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("mint token: %v", err)
		}
		return tok
	}
	call := func(tok string) int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/memory", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		handler(rec, req)
		return rec.Code
	}

	first := mint("worker", 1, 1)        // budget 2/window
	second := mint("worker", 1000, 1000) // budget 2000/window

	if code := call(first); code != http.StatusOK {
		t.Fatalf("first token's first request: got %d, want 200", code)
	}
	if code := call(second); code != http.StatusOK {
		t.Fatalf("second token's first request: got %d, want 200", code)
	}
	// The second token's contract is 2000/window: its second request must be
	// admitted regardless of the first token's 2/window budget.
	if code := call(second); code != http.StatusOK {
		t.Fatalf("second token's second request: got %d, want 200 — it must not inherit the first token's 2/window bucket", code)
	}
	if code := call(first); code != http.StatusOK {
		t.Fatalf("first token's second request: got %d, want 200 — its own budget still has room", code)
	}
	// The first token's own 2/window budget is spent either way.
	if code := call(first); code != http.StatusTooManyRequests {
		t.Fatalf("first token's third request: got %d, want 429 — its own contract was spent", code)
	}
}
