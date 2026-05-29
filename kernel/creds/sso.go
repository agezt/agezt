// SPDX-License-Identifier: MIT

package creds

// AWS IAM Identity Center (formerly AWS SSO) credential reader
// (M1.rr). Operator runs `aws sso login` interactively (browser
// device-auth flow), which lands an OIDC access token in
// `~/.aws/sso/cache/<sha1(start_url)>.json`. From that token we
// exchange GetRoleCredentials against the regional SSO portal and
// receive temporary IAM creds — same shape as STS AssumeRole.
//
// **Why this is not built on the SigV4 extraction.** The SSO
// portal API (`portal.sso.{region}.amazonaws.com`) authenticates
// with a *bearer token* in the `x-amz-sso_bearer_token` header
// rather than a SigV4 signature. So M1.rr ships in parallel to
// M1.SigV4 rather than depending on it. Mentioned because the
// batch-2 report listed SSO as "deferred pending SigV4
// extraction" — that was wrong; the dep is the other way.
//
// Scope:
//   - Read access token from cache; no interactive login flow.
//     If the cache is missing or expired the lookup returns empty
//     (chain falls through to base creds), so `aws sso login`
//     becomes the operator's recovery path.
//   - GetRoleCredentials only. RegisterClient, StartDeviceAuthorization,
//     CreateToken (the login flow itself) are out of scope — every
//     SSO operator already has the AWS CLI for that.
//   - Result cached in-process until 60s before SSO-reported expiry.

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ersinkoc/agezt/kernel/creds/sigv4"
)

// SSOParams describes one SSO profile worth of inputs. Most fields
// are read out of `~/.aws/config`; AWSSSOLookup composes them by
// profile name so callers don't have to thread the values manually.
type SSOParams struct {
	StartURL    string
	Region      string
	AccountID   string
	RoleName    string

	// Test seams. Empty / nil in production.
	Endpoint  string // override SSO portal endpoint
	CacheDir  string // override ~/.aws/sso/cache
	HTTP      interface{ Do(*http.Request) (*http.Response, error) }
	Now       func() time.Time
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

	url := ssoPortalEndpoint(p.Region, p.Endpoint) +
		"/federation/credentials?account_id=" + p.AccountID +
		"&role_name=" + p.RoleName
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("sso: build request: %w", err)
	}
	req.Header.Set("x-amz-sso_bearer_token", tok.AccessToken)

	client := http.DefaultClient
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
			excerpt = excerpt[:512] + "..."
		}
		return nil, fmt.Errorf("sso: %s: %s", resp.Status, excerpt)
	}
	var wire ssoRoleCredentialsResponse
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("sso: parse JSON: %w", err)
	}
	rc := wire.RoleCredentials
	if rc.AccessKeyID == "" || rc.SecretAccessKey == "" {
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
type ssoCache struct {
	mu     sync.Mutex
	creds  *AssumedCreds
	params SSOParams
}

func (c *ssoCache) get(ctx context.Context, now time.Time) (*AssumedCreds, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.creds != nil && now.Before(c.creds.Expiration.Add(-refreshLeadTime)) {
		return c.creds, nil
	}
	fresh, err := GetSSORoleCredentials(ctx, c.params)
	if err != nil {
		return nil, err
	}
	c.creds = fresh
	return fresh, nil
}

// AWSSSOLookup returns a ChainLookup-compatible function that
// resolves AWS credentials from an SSO profile. Other names fall
// through. Errors are swallowed at lookup time (returning empty
// strings) so the chain falls through to base creds; the operator
// sees them via the banner-print site in cmd/agezt.
func AWSSSOLookup(params SSOParams) func(name string) string {
	cache := &ssoCache{params: params}
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

// LoadSSOParamsFromProfile reads SSO config out of ~/.aws/config
// for `profile`. Returns ok=false (with no error) when the profile
// has no SSO fields, so the operator-facing wiring can quietly
// skip SSO when not configured.
//
// Recognises both the old (profile-inline) and new (sso-session)
// AWS-CLI layouts. The old layout puts everything in one section:
//
//	[profile foo]
//	sso_start_url = ...
//	sso_region = ...
//	sso_account_id = ...
//	sso_role_name = ...
//
// The new layout references a separate `[sso-session NAME]`
// section for the URL/region:
//
//	[profile foo]
//	sso_session = NAME
//	sso_account_id = ...
//	sso_role_name = ...
//	[sso-session NAME]
//	sso_start_url = ...
//	sso_region = ...
func LoadSSOParamsFromProfile(profile string) (SSOParams, bool) {
	if profile == "" {
		profile = "default"
	}
	cfgPath := awsConfigFilePath("AWS_CONFIG_FILE", "config")
	if cfgPath == "" {
		return SSOParams{}, false
	}
	cfgSection := profile
	if profile != "default" {
		cfgSection = "profile " + profile
	}
	section, err := readINISection(cfgPath, cfgSection)
	if err != nil {
		return SSOParams{}, false
	}
	p := SSOParams{
		StartURL:  section["sso_start_url"],
		Region:    section["sso_region"],
		AccountID: section["sso_account_id"],
		RoleName:  section["sso_role_name"],
	}
	// New layout: dereference the sso-session section for URL + region
	// when missing from the profile.
	if sess := section["sso_session"]; sess != "" {
		if ssoSec, err := readINISection(cfgPath, "sso-session "+sess); err == nil {
			if p.StartURL == "" {
				p.StartURL = ssoSec["sso_start_url"]
			}
			if p.Region == "" {
				p.Region = ssoSec["sso_region"]
			}
		}
	}
	if p.StartURL == "" || p.AccountID == "" || p.RoleName == "" {
		return SSOParams{}, false
	}
	return p, true
}
