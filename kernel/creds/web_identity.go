// SPDX-License-Identifier: MIT

package creds

// AWS STS AssumeRoleWithWebIdentity support — IRSA / EKS Pod Identity
// (M1.ww). The AWS counterpart of the GCP/GKE metadata token source: a
// workload running on EKS (or any OIDC-federated environment) is handed a
// short-lived OIDC token on disk plus a role ARN, and exchanges them at STS
// for temporary AWS credentials. No long-lived access key ever exists.
//
// What EKS injects automatically (the IAM-Roles-for-Service-Accounts
// webhook, or EKS Pod Identity):
//
//	AWS_WEB_IDENTITY_TOKEN_FILE — path to a projected ServiceAccount OIDC
//	                              token; the kubelet rotates the file, so we
//	                              re-read it on every refresh.
//	AWS_ROLE_ARN                — the role to assume.
//	AWS_ROLE_SESSION_NAME        — optional session name (we synthesise one).
//
// Why it's distinct from AssumeRole (sts.go): AssumeRole needs *base*
// credentials and SigV4-signs the STS request with them. Web identity needs
// NO base credentials — the OIDC token itself is the proof of identity, so
// the STS call is unsigned. That's exactly what makes IRSA keyless.
//
// Wiring: `AWSWebIdentityLookup` returns a `func(name string) string` in the
// ChainLookup shape. cmd/agezt auto-activates it when the standard
// AWS_WEB_IDENTITY_TOKEN_FILE + AWS_ROLE_ARN env vars are present, placed
// ahead of the IMDS/default chain so a pod gets its OWN role rather than the
// node's instance-profile role.

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/creds/sigv4"
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

	client := http.DefaultClient
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

func parseWebIdentityResponse(resp *http.Response) (*AssumedCreds, error) {
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("sts web-identity: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		excerpt := string(raw)
		if len(excerpt) > 512 {
			excerpt = strutil.Ellipsis(excerpt, 512, "...")
		}
		return nil, fmt.Errorf("sts web-identity: %s: %s", resp.Status, excerpt)
	}

	var env struct {
		XMLName xml.Name `xml:"AssumeRoleWithWebIdentityResponse"`
		Result  struct {
			Credentials struct {
				AccessKeyID     string `xml:"AccessKeyId"`
				SecretAccessKey string `xml:"SecretAccessKey"`
				SessionToken    string `xml:"SessionToken"`
				Expiration      string `xml:"Expiration"`
			} `xml:"Credentials"`
		} `xml:"AssumeRoleWithWebIdentityResult"`
	}
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("sts web-identity: parse XML: %w", err)
	}
	c := env.Result.Credentials
	if c.AccessKeyID == "" || c.SecretAccessKey == "" || c.SessionToken == "" {
		return nil, fmt.Errorf("sts web-identity: response missing credential fields: %s", string(raw))
	}
	exp, err := time.Parse(time.RFC3339, c.Expiration)
	if err != nil {
		return nil, fmt.Errorf("sts web-identity: parse Expiration %q: %w", c.Expiration, err)
	}
	return &AssumedCreds{
		Creds: sigv4.Creds{
			AccessKeyID:     c.AccessKeyID,
			SecretAccessKey: c.SecretAccessKey,
			SessionToken:    c.SessionToken,
		},
		Expiration: exp,
	}, nil
}

// webIdentityCache holds the most recent successful web-identity result.
// Mirrors assumeRoleCache; refreshes when within refreshLeadTime of expiry.
type webIdentityCache struct {
	mu     sync.Mutex
	creds  *AssumedCreds
	params WebIdentityParams
}

func (c *webIdentityCache) get(ctx context.Context, now time.Time) (*AssumedCreds, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.creds != nil && now.Before(c.creds.Expiration.Add(-refreshLeadTime)) {
		return c.creds, nil
	}
	fresh, err := AssumeRoleWithWebIdentity(ctx, c.params)
	if err != nil {
		return nil, err
	}
	c.creds = fresh
	return fresh, nil
}

// AWSWebIdentityLookup returns a ChainLookup-compatible function that maps
// the canonical AWS_* credential names to a cached web-identity result.
// Non-credential names fall through (empty). Construct once at daemon start
// so the cache persists; on any exchange failure the credential names return
// empty so the chain falls through to the next source.
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
