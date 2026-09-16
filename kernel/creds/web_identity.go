// SPDX-License-Identifier: MIT
//
// kernel/creds AssumeRoleWithWebIdentity + AWSWebIdentityLookup + types
// (WebIdentityParams, webIdentityCache).
// Extracted from web_identity.go during Day 211 god-file refactor (#93).
// Public API unchanged.
package creds

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// WebIdentityParams configures a single AssumeRoleWithWebIdentity call.
// Unlike AssumeRoleParams there are no BaseCreds: the OIDC token is the
// authentication, so the request is unsigned.
type WebIdentityParams struct {
	Region          string
	RoleArn         string
	RoleSessionName string
	TokenFile       string // path to the projected OIDC token (re-read each refresh)
	DurationSeconds int    // 900..43200; 0 → AWS default of 3600

	// Test seams. Production callers leave these nil/empty.
	Endpoint string // override STS endpoint; default https://sts.{region}.amazonaws.com/
	HTTP     interface {
		Do(*http.Request) (*http.Response, error)
	}
	Now func() time.Time
}

// AssumeRoleWithWebIdentity performs a single unsigned
// sts:AssumeRoleWithWebIdentity call, reading the OIDC token from
// p.TokenFile. Caller caches + refreshes; see AWSWebIdentityLookup for the
// wired-and-cached version.
func AssumeRoleWithWebIdentity(ctx context.Context, p WebIdentityParams) (*AssumedCreds, error) {
	if p.RoleArn == "" {
		return nil, errors.New("sts web-identity: RoleArn required")
	}
	if p.TokenFile == "" {
		return nil, errors.New("sts web-identity: TokenFile required")
	}
	tokenBytes, err := os.ReadFile(p.TokenFile)
	if err != nil {
		return nil, fmt.Errorf("sts web-identity: read token file %q: %w", p.TokenFile, err)
	}
	token := strings.TrimSpace(string(tokenBytes))
	if token == "" {
		return nil, fmt.Errorf("sts web-identity: token file %q is empty", p.TokenFile)
	}

	sessionName := p.RoleSessionName
	if sessionName == "" {
		sessionName = defaultSessionName()
	}
	duration := p.DurationSeconds
	if duration == 0 {
		duration = 3600
	}

	form := url.Values{}
	form.Set("Action", "AssumeRoleWithWebIdentity")
	form.Set("Version", "2011-06-15")
	form.Set("RoleArn", p.RoleArn)
	form.Set("RoleSessionName", sessionName)
	form.Set("WebIdentityToken", token)
	form.Set("DurationSeconds", strconv.Itoa(duration))
	body := []byte(form.Encode())

	endpoint := stsAssumeRoleEndpoint(p.Region, p.Endpoint)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("sts web-identity: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Force Content-Length (STS rejects chunked encoding).
	req.ContentLength = int64(len(body))
	// NOTE: deliberately NOT SigV4-signed — the WebIdentityToken is the
	// credential. This is the keyless property of IRSA.

	client := &http.Client{Timeout: credentialHTTPTimeout}
	if p.HTTP != nil {
		if c, ok := p.HTTP.(*http.Client); ok {
			client = c
		} else {
			resp, err := p.HTTP.Do(req)
			if err != nil {
				return nil, fmt.Errorf("sts web-identity: http: %w", err)
			}
			return parseWebIdentityResponse(resp)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sts web-identity: http: %w", err)
	}
	return parseWebIdentityResponse(resp)
}
func AWSWebIdentityLookup(params WebIdentityParams) func(name string) string {
	cache := &webIdentityCache{params: params}
	now := time.Now
	if params.Now != nil {
		now = params.Now
	}
	return func(name string) string {
		switch name {
		case "AWS_ACCESS_KEY_ID":
			c, err := cache.get(context.Background(), now())
			if err != nil {
				return ""
			}
			return c.Creds.AccessKeyID
		case "AWS_SECRET_ACCESS_KEY":
			c, err := cache.get(context.Background(), now())
			if err != nil {
				return ""
			}
			return c.Creds.SecretAccessKey
		case "AWS_SESSION_TOKEN":
			c, err := cache.get(context.Background(), now())
			if err != nil {
				return ""
			}
			return c.Creds.SessionToken
		case "AWS_REGION":
			return params.Region
		}
		return ""
	}
}
