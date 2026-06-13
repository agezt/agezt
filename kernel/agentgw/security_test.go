// SPDX-License-Identifier: MIT

package agentgw

import (
	"encoding/base64"
	"slices"
	"strings"
	"testing"
	"time"
)

// These tests lock down the security hardening that closed the M939 gateway
// auth-bypass (security-report 2026-06-13, C1/C2 + JWT alg-confusion). They are
// regression guards: each asserts a FORGERY / escalation that must fail.

// mintHelper builds a token manager + a valid token signed by it, returning the
// token string split into its 3 JWT parts for tampering.
func mintHelper(t *testing.T, secret string, claims *TokenClaims) (string, []string) {
	t.Helper()
	tm := NewTokenManager([]byte(secret))
	tok, err := tm.CreateToken(claims)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	return tok, strings.Split(tok, ".")
}

// TestValidateToken_RejectsAlgNone proves a classic "alg":"none" forgery (empty
// signature) is rejected. Defense-in-depth: even though the HS256 signature
// check would independently fail an empty sig, the header pin refuses unknown
// algorithms BEFORE trusting the signature at all.
func TestValidateToken_RejectsAlgNone(t *testing.T) {
	tm := NewTokenManager([]byte("test-secret-key-32-chars-minimum!!"))
	tok, parts := mintHelper(t, "test-secret-key-32-chars-minimum!!", &TokenClaims{
		RunID:     "run_none",
		Caps:      []string{"memory.read"},
		ExpiresAt: time.Now().Add(time.Hour),
	})
	_ = tok
	noneHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	forged := noneHeader + "." + parts[1] + "." // empty signature
	if _, err := tm.ValidateToken(forged); err != ErrInvalidToken {
		t.Errorf("ValidateToken(alg=none): got %v, want ErrInvalidToken", err)
	}
}

// TestValidateToken_RejectsBadTyp isolates the typ pin: a token with the right
// HS256 algorithm but a non-JWT type must be rejected.
func TestValidateToken_RejectsBadTyp(t *testing.T) {
	tm := NewTokenManager([]byte("test-secret-key-32-chars-minimum!!"))
	_, parts := mintHelper(t, "test-secret-key-32-chars-minimum!!", &TokenClaims{
		RunID:     "run_typ",
		Caps:      []string{"memory.read"},
		ExpiresAt: time.Now().Add(time.Hour),
	})
	badTyp := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"oops"}`))
	forged := badTyp + "." + parts[1] + "." + parts[2]
	if _, err := tm.ValidateToken(forged); err != ErrInvalidToken {
		t.Errorf("ValidateToken(typ=oops): got %v, want ErrInvalidToken", err)
	}
}

// TestValidateToken_WrongSecret proves validation depends on the secret: a token
// signed by one key is rejected by a manager built from a different key. Forging
// therefore requires the per-install secret, not knowledge of the algorithm.
func TestValidateToken_WrongSecret(t *testing.T) {
	tok, _ := mintHelper(t, "the-real-per-install-secret-32-bytes!!", &TokenClaims{
		RunID:     "run_cross",
		Caps:      []string{"memory.read"},
		ExpiresAt: time.Now().Add(time.Hour),
	})
	other := NewTokenManager([]byte("a-completely-different-secret-32b!!"))
	if _, err := other.ValidateToken(tok); err != ErrInvalidToken {
		t.Errorf("ValidateToken with wrong secret: got %v, want ErrInvalidToken", err)
	}
}

// TestCreateSubprocessToken_DropsExcessCaps proves a subprocess token can never
// hold a capability its parent lacks — the caps are intersected, not unioned.
func TestCreateSubprocessToken_DropsExcessCaps(t *testing.T) {
	tm := NewTokenManager([]byte("test-secret-key-32-chars-minimum!!"))
	parent := &TokenClaims{
		RunID:     "run_parent",
		Caps:      []string{"memory.read", "memory.write"},
		MaxRate:   60,
		MaxBurst:  10,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	// Request a cap (memory.delete) the parent does NOT have, plus one it does.
	tok, err := tm.CreateSubprocessToken(parent, "sub_1",
		[]string{"memory.read", "memory.delete"}, time.Hour)
	if err != nil {
		t.Fatalf("CreateSubprocessToken: %v", err)
	}
	child, err := tm.ValidateToken(tok)
	if err != nil {
		t.Fatalf("ValidateToken(child): %v", err)
	}
	for _, c := range child.Caps {
		if c == "memory.delete" {
			t.Errorf("child gained a cap the parent lacks: %q (child caps=%v)", c, child.Caps)
		}
	}
	if !slices.Contains(child.Caps, "memory.read") {
		t.Errorf("child lost a legitimately-inherited cap: memory.read (child caps=%v)", child.Caps)
	}
}

// TestCreateSubprocessToken_ExpiryClampedToParent proves a child token can never
// outlive its parent — even when a longer expiry is requested.
func TestCreateSubprocessToken_ExpiryClampedToParent(t *testing.T) {
	tm := NewTokenManager([]byte("test-secret-key-32-chars-minimum!!"))
	parentExpiry := time.Now().Add(1 * time.Minute)
	parent := &TokenClaims{
		RunID:     "run_parent",
		Caps:      []string{"memory.read"},
		MaxRate:   60,
		MaxBurst:  10,
		ExpiresAt: parentExpiry,
	}
	// Request 1 hour — must be clamped down to the parent's 1 minute.
	tok, err := tm.CreateSubprocessToken(parent, "sub_2", []string{"memory.read"}, time.Hour)
	if err != nil {
		t.Fatalf("CreateSubprocessToken: %v", err)
	}
	child, err := tm.ValidateToken(tok)
	if err != nil {
		t.Fatalf("ValidateToken(child): %v", err)
	}
	if child.ExpiresAt.After(parentExpiry.Add(time.Second)) {
		t.Errorf("child outlives parent: child exp=%v, parent exp=%v", child.ExpiresAt, parentExpiry)
	}
}

// TestDefaultGatewayConfig_SecretNotHardcoded is the C1 regression guard at the
// config layer: the default signing secret is random per call and never the old
// public-source literal.
func TestDefaultGatewayConfig_SecretNotHardcoded(t *testing.T) {
	const old = "change-me-in-production"
	a := DefaultGatewayConfig("/tmp/agezt-test-a")
	if len(a.TokenSecret) == 0 {
		t.Fatal("DefaultGatewayConfig returned an empty token secret")
	}
	if string(a.TokenSecret) == old {
		t.Fatalf("default secret is the hardcoded literal %q", old)
	}
	b := DefaultGatewayConfig("/tmp/agezt-test-b")
	if string(a.TokenSecret) == string(b.TokenSecret) {
		t.Error("two DefaultGatewayConfig calls produced the same secret — not random")
	}
}
