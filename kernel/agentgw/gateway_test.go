// SPDX-License-Identifier: MIT

package agentgw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestTokenManager tests token creation and validation.
func TestTokenManager(t *testing.T) {
	tm := NewTokenManager([]byte("test-secret-key-32-chars-minimum!!"))

	claims := &TokenClaims{
		RunID:     "run_abc123",
		Caps:      []string{"memory.write", "memory.read"},
		MaxRate:   60,
		MaxBurst:  10,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	// Create token
	token, err := tm.CreateToken(claims)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}

	// Validate token
	validated, err := tm.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	if validated.RunID != claims.RunID {
		t.Errorf("RunID: got %q, want %q", validated.RunID, claims.RunID)
	}
	if len(validated.Caps) != len(claims.Caps) {
		t.Errorf("Caps length: got %d, want %d", len(validated.Caps), len(claims.Caps))
	}
	if validated.MaxRate != claims.MaxRate {
		t.Errorf("MaxRate: got %d, want %d", validated.MaxRate, claims.MaxRate)
	}
}

// TestTokenManager_Expired tests that expired tokens are rejected.
func TestTokenManager_Expired(t *testing.T) {
	tm := NewTokenManager([]byte("test-secret-key-32-chars-minimum!!"))

	claims := &TokenClaims{
		RunID:     "run_abc123",
		Caps:      []string{"memory.write"},
		MaxRate:   60,
		MaxBurst:  10,
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
	}

	token, err := tm.CreateToken(claims)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	_, err = tm.ValidateToken(token)
	if err != ErrTokenExpired {
		t.Errorf("ValidateToken: got %v, want ErrTokenExpired", err)
	}
}

// TestTokenManager_Invalid tests that invalid tokens are rejected.
func TestTokenManager_Invalid(t *testing.T) {
	tm := NewTokenManager([]byte("test-secret-key-32-chars-minimum!!"))

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"random", "not.a.valid.token"},
		{"wrong sig", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJydW5faWQiOiJydW5fYWJjMTIzIn0.wrongsignature"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tm.ValidateToken(tt.token)
			if err != ErrInvalidToken {
				t.Errorf("ValidateToken(%q): got %v, want ErrInvalidToken", tt.name, err)
			}
		})
	}
}

// TestTokenClaims_HasCap tests capability checking.
func TestTokenClaims_HasCap(t *testing.T) {
	claims := &TokenClaims{
		Caps: []string{"memory.write", "eventbus.publish"},
	}

	if !claims.HasCap("memory.write") {
		t.Error("HasCap(memory.write): got false, want true")
	}
	if !claims.HasCap("eventbus.publish") {
		t.Error("HasCap(eventbus.publish): got false, want true")
	}
	if claims.HasCap("memory.read") {
		t.Error("HasCap(memory.read): got true, want false")
	}
}

// TestCapabilityChecker tests capability validation.
func TestCapabilityChecker(t *testing.T) {
	cc := NewCapabilityChecker()

	claims := &TokenClaims{
		Caps: []string{"memory.write"},
	}

	// Should pass
	if err := cc.Check(claims, CapMemoryWrite); err != nil {
		t.Errorf("Check(memory.write): got %v, want nil", err)
	}

	// Should fail
	if err := cc.Check(claims, CapMemoryRead); err == nil {
		t.Error("Check(memory.read): got nil, want error")
	}
}

// TestCapabilityChecker_CheckAny tests checking multiple capabilities.
func TestCapabilityChecker_CheckAny(t *testing.T) {
	cc := NewCapabilityChecker()

	claims := &TokenClaims{
		Caps: []string{"memory.write"},
	}

	// Should pass - has at least one
	if err := cc.CheckAny(claims, CapMemoryRead, CapMemoryWrite); err != nil {
		t.Errorf("CheckAny(memory.read, memory.write): got %v, want nil", err)
	}

	// Should fail - has none
	if err := cc.CheckAny(claims, CapMemoryRead, CapAgentList); err == nil {
		t.Error("CheckAny(memory.read, agent.list): got nil, want error")
	}
}

// TestParseCapability tests capability string parsing.
func TestParseCapability(t *testing.T) {
	tests := []struct {
		input    string
		expected AgentCapability
		wantErr  bool
	}{
		{"memory.write", CapMemoryWrite, false},
		{"MEMORY.WRITE", CapMemoryWrite, false}, // Case insensitive
		{" eventbus.publish ", CapEventbusPublish, false},
		{"invalid.cap", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseCapability(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseCapability(%q): got %v, want error", tt.input, got)
				}
			} else {
				if err != nil {
					t.Errorf("ParseCapability(%q): got error %v", tt.input, err)
				}
				if got != tt.expected {
					t.Errorf("ParseCapability(%q): got %v, want %v", tt.input, got, tt.expected)
				}
			}
		})
	}
}

// TestNormalizeCaps tests capability normalization and deduplication.
func TestNormalizeCaps(t *testing.T) {
	_, err := NormalizeCaps([]string{"memory.write", "memory.read", "MEMORY.WRITE", "invalid.cap"})
	if err == nil {
		t.Error("NormalizeCaps: got nil error, want error for invalid.cap")
	}

	caps, err := NormalizeCaps([]string{"memory.write", "memory.read", "MEMORY.WRITE"})
	if err != nil {
		t.Fatalf("NormalizeCaps: %v", err)
	}

	// Should be deduplicated
	if len(caps) != 2 {
		t.Errorf("NormalizeCaps: got %d caps, want 2 (deduplicated)", len(caps))
	}
}

// TestNewRateLimit tests rate limiter creation.
func TestNewRateLimit(t *testing.T) {
	rl := NewRateLimit(60, 10)
	if rl.max != 60 {
		t.Errorf("max: got %d, want 60", rl.max)
	}
	if rl.burst != 10 {
		t.Errorf("burst: got %d, want 10", rl.burst)
	}
}

// TestRateLimit_Allow tests rate limiting logic.
func TestRateLimit_Allow(t *testing.T) {
	rl := NewRateLimit(5, 2)

	// First 5 should pass (max RPM)
	for i := 0; i < 5; i++ {
		if !rl.Allow() {
			t.Errorf("Allow() iteration %d: got false, want true", i)
		}
	}

	// Burst allows 2 more
	if !rl.Allow() {
		t.Error("Allow() burst: got false, want true")
	}
	if !rl.Allow() {
		t.Error("Allow() burst: got false, want true")
	}

	// Now should be limited
	if rl.Allow() {
		t.Error("Allow() after burst: got true, want false")
	}
}

// TestRateLimit_WindowReset proves the limiter re-arms after a window rolls
// over (regression for the bug where Allow() returned true unconditionally
// once the first window elapsed, silently disabling the limit).
func TestRateLimit_WindowReset(t *testing.T) {
	rl := NewRateLimit(2, 1) // 3 allowed per window
	// Exhaust the current window.
	for i := 0; i < 3; i++ {
		if !rl.Allow() {
			t.Fatalf("Allow() %d: got false, want true", i)
		}
	}
	if rl.Allow() {
		t.Fatal("Allow() over limit: got true, want false")
	}
	// Force the window to have elapsed, then confirm the limit re-applies
	// (NOT that every call is suddenly allowed forever).
	rl.mu.Lock()
	rl.windowEnd = time.Now().UnixMilli() - 1
	rl.mu.Unlock()
	for i := 0; i < 3; i++ {
		if !rl.Allow() {
			t.Fatalf("Allow() post-reset %d: got false, want true", i)
		}
	}
	if rl.Allow() {
		t.Error("Allow() over limit after reset: got true, want false (limiter must re-arm, not disable)")
	}
}

// TestGateway_HealthEndpoint tests the health check endpoint.
func TestGateway_HealthEndpoint(t *testing.T) {
	g := NewGateway(GatewayConfig{
		SocketPath:  "@test/agentgw/health.sock",
		TokenSecret: []byte("test-secret-key-32-chars-minimum!!"),
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", g.handleHealth)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// TestGateway_TokenCreateEndpoint tests the (now authenticated) subprocess
// token-mint endpoint: it requires a parent token in context and clamps the
// child to the parent's capabilities.
func TestGateway_TokenCreateEndpoint(t *testing.T) {
	g := NewGateway(GatewayConfig{
		SocketPath:  "@test/agentgw/token.sock",
		TokenSecret: []byte("test-secret-key-32-chars-minimum!!"),
	})

	// Unauthenticated (no claims in context) must be rejected.
	t.Run("unauthenticated", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/token/create", strings.NewReader(`{"caps":["memory.read"]}`))
		rr := httptest.NewRecorder()
		g.handleTokenCreate(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Status: got %d, want %d. Body: %s", rr.Code, http.StatusUnauthorized, rr.Body.String())
		}
	})

	parent := &TokenClaims{
		RunID:     "run_abc123",
		Caps:      []string{"memory.write", "memory.read"},
		MaxRate:   60,
		MaxBurst:  10,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	// A subset request from an authenticated parent succeeds.
	t.Run("subset_ok", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/token/create", strings.NewReader(`{
			"sub_id": "sub_1",
			"caps": ["memory.read"],
			"expiry_ms": 600000
		}`))
		req = req.WithContext(context.WithValue(req.Context(), claimsKey{}, parent))
		rr := httptest.NewRecorder()
		g.handleTokenCreate(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("Status: got %d, want %d. Body: %s", rr.Code, http.StatusCreated, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), `"token"`) {
			t.Errorf("Body: got %s, want token field", rr.Body.String())
		}
	})

	// Requesting a capability the parent lacks must be rejected (no escalation).
	t.Run("escalation_rejected", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/token/create", strings.NewReader(`{
			"caps": ["memory.delete"]
		}`))
		req = req.WithContext(context.WithValue(req.Context(), claimsKey{}, parent))
		rr := httptest.NewRecorder()
		g.handleTokenCreate(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Status: got %d, want %d. Body: %s", rr.Code, http.StatusForbidden, rr.Body.String())
		}
	})
}

// TestGateway_TokenValidationEndpoint tests the token validation endpoint.
func TestGateway_TokenValidationEndpoint(t *testing.T) {
	g := NewGateway(GatewayConfig{
		SocketPath:  "@test/agentgw/validate.sock",
		TokenSecret: []byte("test-secret-key-32-chars-minimum!!"),
	})

	// First create a valid token
	tm := g.tokenMgr
	token, err := tm.CreateToken(&TokenClaims{
		RunID:     "run_test",
		Caps:      []string{"memory.write"},
		MaxRate:   60,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	// Validate via handler (we can't easily test the actual endpoint without socket)
	claims, err := tm.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.RunID != "run_test" {
		t.Errorf("RunID: got %q, want %q", claims.RunID, "run_test")
	}
}

// TestResponseError tests the error response helper.
func TestResponseError(t *testing.T) {
	rr := httptest.NewRecorder()
	responseError(rr, http.StatusBadRequest, "TEST_ERROR", "test message")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Code: got %d, want %d", rr.Code, http.StatusBadRequest)
	}

	if !strings.Contains(rr.Body.String(), "TEST_ERROR") {
		t.Errorf("Body: got %s, want TEST_ERROR", rr.Body.String())
	}
}

// TestResponseJSON tests the JSON response helper.
func TestResponseJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	responseJSON(rr, http.StatusOK, map[string]string{"key": "value"})

	if rr.Code != http.StatusOK {
		t.Errorf("Code: got %d, want %d", rr.Code, http.StatusOK)
	}

	if !strings.Contains(rr.Body.String(), "value") {
		t.Errorf("Body: got %s, want value", rr.Body.String())
	}
}

// TestExtractBearerToken tests the Authorization header extraction.
func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"Bearer abc123", "abc123"},
		{"Basic abc", ""}, // Wrong scheme
		{"", ""},          // Empty
		{"Bearer ", ""},   // Empty token
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}

			got := extractBearerToken(req)
			if got != tt.want {
				t.Errorf("extractBearerToken(%q): got %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

// TestAuditEntry tests the audit entry structure.
func TestAuditEntry(t *testing.T) {
	entry := AuditEntry{
		Timestamp:  time.Now(),
		TokenID:    "token_abc",
		RunID:      "run_xyz",
		Capability: "memory.write",
		Operation:  "POST",
		Path:       "/v1/memory/write",
		Success:    true,
		DurationMs: 15,
	}

	if entry.TokenID != "token_abc" {
		t.Errorf("TokenID: got %q, want %q", entry.TokenID, "token_abc")
	}
	if entry.Capability != "memory.write" {
		t.Errorf("Capability: got %q, want %q", entry.Capability, "memory.write")
	}
	if entry.DurationMs != 15 {
		t.Errorf("DurationMs: got %d, want %d", entry.DurationMs, 15)
	}
}

// TestGatewayConfig_Defaults tests default configuration.
func TestGatewayConfig_Defaults(t *testing.T) {
	cfg := DefaultGatewayConfig("/tmp/agezt")
	if !strings.HasPrefix(cfg.SocketPath, "@agezt/agentgw-") || !strings.HasSuffix(cfg.SocketPath, ".sock") {
		t.Errorf("SocketPath: got %q, want prefix @agezt/agentgw- and suffix .sock", cfg.SocketPath)
	}
	if cfg.ReadTimeout != 30*time.Second {
		t.Errorf("ReadTimeout: got %v, want %v", cfg.ReadTimeout, 30*time.Second)
	}
	if cfg.WriteTimeout != 30*time.Second {
		t.Errorf("WriteTimeout: got %v, want %v", cfg.WriteTimeout, 30*time.Second)
	}
}

// BenchmarkTokenCreation benchmarks token creation.
func BenchmarkTokenCreation(b *testing.B) {
	tm := NewTokenManager([]byte("test-secret-key-32-chars-minimum!!"))
	claims := &TokenClaims{
		RunID:     "run_bench",
		Caps:      []string{"memory.write", "memory.read", "eventbus.publish"},
		MaxRate:   60,
		MaxBurst:  10,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	for i := 0; i < b.N; i++ {
		_, _ = tm.CreateToken(claims)
	}
}

// BenchmarkTokenValidation benchmarks token validation.
func BenchmarkTokenValidation(b *testing.B) {
	tm := NewTokenManager([]byte("test-secret-key-32-chars-minimum!!"))
	claims := &TokenClaims{
		RunID:     "run_bench",
		Caps:      []string{"memory.write", "memory.read", "eventbus.publish"},
		MaxRate:   60,
		MaxBurst:  10,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	token, _ := tm.CreateToken(claims)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tm.ValidateToken(token)
	}
}
