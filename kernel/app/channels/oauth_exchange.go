// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type OAuthHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

func (p *OAuthMemory) Exchange(ctx context.Context, flow OAuthFlow, code string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", flow.RedirectURI)
	form.Set("client_id", flow.ClientID)
	form.Set("client_secret", flow.ClientSecret)

	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, flow.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := p.clientFor(20 * time.Second).Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var parsed struct {
		OK          *bool  `json:"ok"`
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("provider returned non-JSON (status %d)", resp.StatusCode)
	}
	if parsed.OK != nil && !*parsed.OK {
		return "", providerErr(parsed.Error, parsed.ErrorDesc, resp.StatusCode)
	}
	if parsed.AccessToken == "" {
		return "", providerErr(parsed.Error, parsed.ErrorDesc, resp.StatusCode)
	}
	return parsed.AccessToken, nil
}

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
