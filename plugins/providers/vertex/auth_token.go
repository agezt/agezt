// SPDX-License-Identifier: MIT
//
// plugins/providers/vertex token lifecycle (TokenSource.Token, TokenSource.exchange).
// Extracted from auth.go during Day 211 god-file refactor (#89).
// Public API unchanged.
package vertex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agezt/agezt/plugins/providers/internal/httpread"
)

func (ts *TokenSource) Token(ctx context.Context) (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.cached != "" && ts.now().Add(TokenSkew).Before(ts.expiresAt) {
		return ts.cached, nil
	}
	tok, expiresIn, err := ts.exchange(ctx)
	if err != nil {
		return "", err
	}
	ts.cached = tok
	ts.expiresAt = ts.now().Add(time.Duration(expiresIn) * time.Second)
	return tok, nil
}
func (ts *TokenSource) exchange(ctx context.Context) (token string, expiresIn int, err error) {
	jwt, err := signJWT(ts.sa, ts.key, ts.scope, ts.sa.TokenURI, ts.now())
	if err != nil {
		return "", 0, err
	}
	body := url.Values{
		"grant_type": {JWTBearerGrantType},
		"assertion":  {jwt},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.sa.TokenURI, strings.NewReader(body))
	if err != nil {
		return "", 0, fmt.Errorf("vertex: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := ts.http.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("vertex: token exchange: %w", err)
	}
	defer resp.Body.Close()
	raw, err := httpread.All(resp.Body, httpread.DefaultMaxResponseBytes)
	if err != nil {
		return "", 0, fmt.Errorf("vertex: read token response: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return "", 0, fmt.Errorf("vertex: token exchange status %d: %s", resp.StatusCode, string(raw))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(raw, &tr); err != nil {
		return "", 0, fmt.Errorf("vertex: parse token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", 0, errors.New("vertex: token response missing access_token")
	}
	if tr.ExpiresIn <= 0 {
		tr.ExpiresIn = 3600 // default 1h per Google's spec
	}
	return tr.AccessToken, tr.ExpiresIn, nil
}
