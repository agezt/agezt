// SPDX-License-Identifier: MIT

// channel_oauth_helpers.go: pruneOAuthLocked + newOAuthState + providerErr +
// normalizeInstanceURL + isHTTPSURL split off from channel_oauth.go during the
// Day 211 god-file refactor (#145). Public API unchanged.
package controlplane

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func (s *Server) pruneOAuthLocked(now time.Time) {
	for k, f := range s.oauthPending {
		if now.Sub(f.created) > oauthFlowTTL {
			delete(s.oauthPending, k)
		}
	}
}

func newOAuthState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// handleChannelOAuthStart begins an OAuth flow: it records the client credentials
// + target account and returns the provider's authorize URL for the browser to
// open. args: kind, label, client_id, client_secret, redirect_uri, instance_url.
func providerErr(code, desc string, status int) error {
	switch {
	case desc != "":
		return fmt.Errorf("%s", desc)
	case code != "":
		return fmt.Errorf("%s", code)
	default:
		return fmt.Errorf("no access_token in response (status %d)", status)
	}
}

// normalizeInstanceURL validates an operator-supplied instance URL (Mastodon)
// and returns its scheme://host[:port] base. SSRF to internal hosts is blocked
// at dial time by the netguard client during the token exchange; here we just
// enforce a well-formed https URL.
func normalizeInstanceURL(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("instance URL required for this provider")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid instance URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("instance URL must be http(s)")
	}
	return u.Scheme + "://" + u.Host, nil
}

func isHTTPSURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
