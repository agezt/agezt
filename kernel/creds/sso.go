// SPDX-License-Identifier: MIT

package creds

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/creds/sigv4"
)


// SSOParams describes one SSO profile worth of inputs. Most fields
// are read out of `~/.aws/config`; AWSSSOLookup composes them by
// profile name so callers don't have to thread the values manually.
type SSOParams struct {
	StartURL  string
	Region    string
	AccountID string
	RoleName  string

	// Test seams. Empty / nil in production.
	Endpoint string // override SSO portal endpoint
	CacheDir string // override ~/.aws/sso/cache
	HTTP     interface {
		Do(*http.Request) (*http.Response, error)
	}
	Now func() time.Time
}

// ssoCachedToken mirrors the JSON shape `aws sso login` writes
// into `~/.aws/sso/cache/<sha1>.json`. We read only the fields we
// need; AWS may add others over time without breaking us.
type ssoCachedToken struct {
	StartURL    string    `json:"startUrl"`
	Region      string    `json:"region"`
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// ssoCacheFilename is the AWS SDK convention: sha1-hex of the
// start URL, with `.json` suffix. Lower-case hex.
func ssoCacheFilename(startURL string) string {
	sum := sha1.Sum([]byte(startURL))
	return hex.EncodeToString(sum[:]) + ".json"
}

// ssoCacheDir defaults to ~/.aws/sso/cache. Empty when the user
// has no home dir (shouldn't happen on supported OSes; defensive).
func ssoCacheDir(override string) string {
	if override != "" {
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".aws", "sso", "cache")
}

// readSSOCachedToken locates and parses the cached token for
// startURL. Returns errors for missing-file and parse-failure
// separately so AWSSSOLookup can distinguish "operator never
// logged in" from "cache is malformed". An expired token is
// returned successfully with err nil; the caller checks expiry.
func readSSOCachedToken(cacheDir, startURL string) (*ssoCachedToken, error) {
	dir := ssoCacheDir(cacheDir)
	if dir == "" {
		return nil, errors.New("sso: no home directory for cache lookup")
	}
	path := filepath.Join(dir, ssoCacheFilename(startURL))
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tok ssoCachedToken
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, fmt.Errorf("sso: parse cache %s: %w", path, err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("sso: cache %s missing accessToken", path)
	}
	if tok.ExpiresAt.IsZero() {
		// The AWS CLI always writes expiresAt (its own refresh depends on it);
		// a cache without one is truncated or corrupt. Treating it as
		// never-expiring sent an ancient bearer token to the portal, surfacing
		// as an opaque 401 instead of this actionable error. Fail closed.
		return nil, fmt.Errorf("sso: cache %s missing expiresAt (re-run `aws sso login` to rewrite the cache)", path)
	}
	return &tok, nil
}

// ssoPortalEndpoint returns the regional SSO portal URL. Operators
// can override for testing or VPC endpoints.
func ssoPortalEndpoint(region, override string) string {
	if override != "" {
		return override
	}
	if region == "" {
		region = "us-east-1"
	}
	return "https://portal.sso." + region + ".amazonaws.com"
}

// ssoRoleCredentialsResponse mirrors the GetRoleCredentials JSON
// envelope. Note `expiration` is **Unix milliseconds** (not RFC3339
// — AWS chose a different format for this API than for STS).
type ssoRoleCredentialsResponse struct {
	RoleCredentials struct {
		AccessKeyID     string `json:"accessKeyId"`
		SecretAccessKey string `json:"secretAccessKey"`
		SessionToken    string `json:"sessionToken"`
		Expiration      int64  `json:"expiration"`
	} `json:"roleCredentials"`
}

// GetSSORoleCredentials calls the SSO portal API for one role and
// returns the resulting short-lived IAM credentials. Caller is
// responsible for caching the result; see AWSSSOLookup for the
// cached-and-wired version.
func GetSSORoleCredentials(ctx context.Context, p SSOParams) (*AssumedCreds, error) {
	if p.AccountID == "" || p.RoleName == "" || p.StartURL == "" {
		return nil, errors.New("sso: AccountID, RoleName, and StartURL all required")
	}
	tok, err := readSSOCachedToken(p.CacheDir, p.StartURL)
	if err != nil {
		return nil, fmt.Errorf("sso: read cached token: %w", err)
	}
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	if !tok.ExpiresAt.IsZero() && !now().Before(tok.ExpiresAt) {
		return nil, fmt.Errorf("sso: cached token expired at %s (re-run `aws sso login`)", tok.ExpiresAt.Format(time.RFC3339))
	}

	reqURL := ssoPortalEndpoint(p.Region, p.Endpoint) + "/federation/credentials"
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("sso: build request: %w", err)
	}
	// Escape the query params: IAM role names legitimately contain characters that
	// are special in a URL query (`+` decodes to space, `&`/`#`/`=` corrupt it), so
	// raw concatenation would send a wrong role_name/account_id for those operators
	// (STS already builds its form with url.Values). (M466)
	q := url.Values{}
	q.Set("account_id", p.AccountID)
	q.Set("role_name", p.RoleName)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("x-amz-sso_bearer_token", tok.AccessToken)

	client := &http.Client{Timeout: credentialHTTPTimeout}
	if p.HTTP != nil {
		if c, ok := p.HTTP.(*http.Client); ok {
			client = c
		} else {
			resp, err := p.HTTP.Do(req)
			if err != nil {
				return nil, fmt.Errorf("sso: http: %w", err)
			}
			return parseSSORoleCredentials(resp)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sso: http: %w", err)
	}
	return parseSSORoleCredentials(resp)
}

func parseSSORoleCredentials(resp *http.Response) (*AssumedCreds, error) {
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("sso: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		excerpt := string(raw)
		if len(excerpt) > 512 {
			excerpt = strutil.Ellipsis(excerpt, 512, "...")
		}
		return nil, fmt.Errorf("sso: %s: %s", resp.Status, excerpt)
	}
	var wire ssoRoleCredentialsResponse
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("sso: parse JSON: %w", err)
	}
	rc := wire.RoleCredentials
	// sessionToken is required: GetRoleCredentials returns temporary STS
	// credentials that always carry one. Accepting a token-less response
	// would let the chain pair SSO's AKID/SECRET with a session token from
	// another source — credentials that cannot sign together.
	if rc.AccessKeyID == "" || rc.SecretAccessKey == "" || rc.SessionToken == "" {
		return nil, fmt.Errorf("sso: response missing credentials: %s", string(raw))
	}
	// Unix-milliseconds → time.Time. The SSO API picked a different
	// encoding from every other AWS API; nothing we can do.
	exp := time.UnixMilli(rc.Expiration).UTC()
	return &AssumedCreds{
		Creds: sigv4.Creds{
			AccessKeyID:     rc.AccessKeyID,
			SecretAccessKey: rc.SecretAccessKey,
			SessionToken:    rc.SessionToken,
		},
		Expiration: exp,
	}, nil
}

// ssoCache mirrors assumeRoleCache from sts.go but for SSO. Kept
// separate because the params shape differs; sharing a struct
// would force the more abstract one to thread two unrelated
// parameter shapes through one cache.
