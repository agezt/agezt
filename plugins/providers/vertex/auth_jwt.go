// SPDX-License-Identifier: MIT
//
// plugins/providers/vertex JWT helpers (b64url, signJWT).
// Extracted from auth.go during Day 211 god-file refactor (#89).
// Public API unchanged.
package vertex

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
func signJWT(sa *ServiceAccountKey, key *rsa.PrivateKey, scope, aud string, now time.Time) (string, error) {
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	if sa.PrivateKeyID != "" {
		header["kid"] = sa.PrivateKeyID
	}
	hdrJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claims := map[string]any{
		"iss":   sa.ClientEmail,
		"scope": scope,
		"aud":   aud,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := b64url(hdrJSON) + "." + b64url(claimsJSON)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("vertex: sign JWT: %w", err)
	}
	return signingInput + "." + b64url(sig), nil
}
