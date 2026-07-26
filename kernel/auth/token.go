// SPDX-License-Identifier: MIT

// Package auth owns transport-independent credential verification and
// authority tiers. HTTP surfaces adapt request headers, cookies, and query
// parameters into the presented credential; this package decides whether that
// credential has enough authority.
package auth

import "crypto/subtle"

// Verifier is the credential boundary shared by the HTTP and non-HTTP
// transports. Implementations must fail closed for blank credentials and
// unknown tiers.
type Verifier interface {
	Authorize(presented string, required Tier) bool
}

// StaticVerifier verifies one daemon-admin credential and zero or more
// user-scoped credentials. It is suitable for the daemon's long-lived listen
// tokens and ephemeral, user-scoped credentials such as the WebUI SSE token.
type StaticVerifier struct {
	adminToken string
	userTokens []string
}

// NewStaticVerifier builds a verifier from the daemon admin token and optional
// user-scoped tokens. Blank tokens are ignored so an omitted configuration can
// never turn into a valid credential.
func NewStaticVerifier(adminToken string, userTokens ...string) *StaticVerifier {
	filtered := make([]string, 0, len(userTokens))
	for _, token := range userTokens {
		if token != "" {
			filtered = append(filtered, token)
		}
	}
	return &StaticVerifier{adminToken: adminToken, userTokens: filtered}
}

// Authorize reports whether presented satisfies required. Comparisons are
// constant-time for each configured token. All configured user tokens are
// checked even after a match so their position is not exposed through an early
// return.
func (v *StaticVerifier) Authorize(presented string, required Tier) bool {
	if !required.Valid() {
		return false
	}
	if required == TierPublic {
		return true
	}
	if v == nil || presented == "" {
		return false
	}

	adminMatch := tokenEqual(presented, v.adminToken)
	if required == TierAdmin {
		return adminMatch
	}

	matched := adminMatch
	for _, token := range v.userTokens {
		if tokenEqual(presented, token) {
			matched = true
		}
	}
	return matched
}

func tokenEqual(presented, configured string) bool {
	if configured == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(configured)) == 1
}
