// SPDX-License-Identifier: MIT

// ChatGPT auth: token-exchange HTTP surface (tokenResp + postToken).
// Code extracted from chatgptauth.go during the Day-131 god-file split.
// Public API unchanged.
package chatgptauth


import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
)

type tokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func postToken(ctx context.Context, form url.Values) (*tokenResp, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, tokenEP, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClientFor(20 * time.Second).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out tokenResp
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("chatgptauth: token endpoint returned non-JSON (status %d)", resp.StatusCode)
	}
	if out.AccessToken == "" {
		if out.ErrorDesc != "" {
			return nil, fmt.Errorf("chatgptauth: %s", out.ErrorDesc)
		}
		if out.Error != "" {
			return nil, fmt.Errorf("chatgptauth: %s", out.Error)
		}
		return nil, fmt.Errorf("chatgptauth: no access_token (status %d)", resp.StatusCode)
	}
	return &out, nil
}

// jwtPayload base64url-decodes a JWT's claims (no signature check — it's our own
// token, used only to read exp / account id / email).
func jwtPayload(token string) (map[string]any, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil, false
	}
	return m, true
}

func jwtExp(token string) (time.Time, bool) {
	m, ok := jwtPayload(token)
	if !ok {
		return time.Time{}, false
	}
	exp, ok := m["exp"].(float64)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(int64(exp), 0), true
}

func jwtEmail(token string) string {
	m, ok := jwtPayload(token)
	if !ok {
		return ""
	}
	if e, ok := m["email"].(string); ok {
		return e
	}
	return ""
}

// accountIDFromIDToken extracts chatgpt_account_id from the id_token's custom
// claim namespace.
func accountIDFromIDToken(token string) string {
	m, ok := jwtPayload(token)
	if !ok {
		return ""
	}
	auth, ok := m["https://api.openai.com/auth"].(map[string]any)
	if !ok {
		return ""
	}
	if id, ok := auth["chatgpt_account_id"].(string); ok {
		return id
	}
	return ""
}
