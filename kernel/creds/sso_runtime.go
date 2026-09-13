// SPDX-License-Identifier: MIT

package creds

// AWS SSO runtime: ssoCache type + ssoCache.get + AWSSSOLookup +
// LoadSSOParamsFromProfile. Carved out of sso.go during the Day
// 201 god-file split so the main file can stay focused on the
// cache-file helpers + the GetRoleCredentials exchange + parsing.
// Public API unchanged.

import (
	"context"
	"sync"
	"time"
)

type ssoCache struct {
	mu       sync.Mutex
	creds    *AssumedCreds
	params   SSOParams
	negCache time.Time // last failed fetch; retries suppressed for negCacheTTL
}

func (c *ssoCache) get(ctx context.Context, now time.Time) (*AssumedCreds, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.creds != nil && now.Before(c.creds.Expiration.Add(-refreshLeadTime)) {
		return c.creds, nil
	}
	// Negative cache, mirroring assumeRoleCache/imdsCache: a just-failed
	// fetch (expired bearer token, refused portal) is doomed; don't re-run
	// it for every credential name in one chain resolution.
	if !c.negCache.IsZero() && now.Sub(c.negCache) < negCacheTTL {
		return nil, errCredFetchSuppressed
	}
	fresh, err := GetSSORoleCredentials(ctx, c.params)
	if err != nil {
		c.negCache = now
		return nil, err
	}
	c.creds = fresh
	c.negCache = time.Time{}
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

