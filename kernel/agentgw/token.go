// SPDX-License-Identifier: MIT

package agentgw

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/ulid"
)

// ErrInvalidToken is returned when token validation fails.
var ErrInvalidToken = errors.New("agentgw: invalid token")

// ErrTokenExpired is returned when the token has expired.
var ErrTokenExpired = errors.New("agentgw: token expired")

// DefaultTokenSecret is the default HMAC secret for signing tokens.
const DefaultTokenSecret = "change-me-in-production"

// TokenManager creates and validates JWT-like tokens for agent subprocess access.
type TokenManager struct {
	secret []byte
}

// NewTokenManager creates a new token manager with the given secret.
func NewTokenManager(secret []byte) *TokenManager {
	if len(secret) < 32 {
		// Use a hash of the secret if it's too short
		h := sha256.Sum256(secret)
		secret = h[:]
	}
	return &TokenManager{secret: secret}
}

// CreateToken creates a new agent subprocess token with the given claims.
func (tm *TokenManager) CreateToken(claims *TokenClaims) (string, error) {
	if claims.RunID == "" {
		return "", errors.New("agentgw: RunID required")
	}
	if len(claims.Caps) == 0 {
		return "", errors.New("agentgw: at least one capability required")
	}
	if claims.ExpiresAt.IsZero() {
		claims.ExpiresAt = time.Now().Add(1 * time.Hour)
	}
	if claims.MaxRate == 0 {
		claims.MaxRate = 60 // default 60 RPM
	}
	if claims.MaxBurst == 0 {
		claims.MaxBurst = 10 // default burst of 10
	}

	// Generate a unique token ID (we don't need to store it, just create the token)
	_ = ulid.New()

	// Create header
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

	// Create payload with tid
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("agentgw: marshal: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)

	// Create signature
	sigInput := header + "." + payload
	sig := tm.sign(sigInput)
	signature := base64.RawURLEncoding.EncodeToString(sig)

	return sigInput + "." + signature, nil
}

// ValidateToken validates a token and returns its claims.
func (tm *TokenManager) ValidateToken(token string) (*TokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	// Verify signature
	expectedSig := tm.sign(parts[0] + "." + parts[1])
	actualSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}

	if !hmac.Equal(expectedSig, actualSig) {
		return nil, ErrInvalidToken
	}

	// Decode payload
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims TokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	// Check expiration
	if !claims.ExpiresAt.IsZero() && time.Now().After(claims.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	return &claims, nil
}

// sign creates an HMAC-SHA256 signature.
func (tm *TokenManager) sign(input string) []byte {
	h := hmac.New(sha256.New, tm.secret)
	h.Write([]byte(input))
	return h.Sum(nil)
}

// CreateSubprocessToken creates a scoped token for a subprocess.
// It inherits capabilities from the parent but can have a subset.
func (tm *TokenManager) CreateSubprocessToken(parent *TokenClaims, subID string, caps []string, expiry time.Duration) (string, error) {
	if expiry == 0 {
		expiry = 10 * time.Minute // default 10 minutes for subprocess
	}

	claims := &TokenClaims{
		RunID:         parent.RunID,
		Caps:          caps,
		MaxRate:       parent.MaxRate,
		MaxBurst:      parent.MaxBurst / 2, // subprocess gets half the burst
		ExpiresAt:     time.Now().Add(expiry),
		ParentTokenID: parent.RunID, // TODO: store actual parent tid
		SubprocessID:  subID,
	}

	return tm.CreateToken(claims)
}
