// SPDX-License-Identifier: MIT

package creds

// AWS STS AssumeRole support (M1.vv). Calls sts:AssumeRole against
// the regional STS endpoint, parses the XML response, returns
// temporary credentials (AccessKeyId / SecretAccessKey /
// SessionToken / Expiration).
//
// Why it's its own file (not part of aws.go): STS sits one level
// above the base credential chain. The chain produces *long-lived*
// credentials (env / shared file / IMDS / credential_process);
// STS uses those as the *signing* credentials for an HTTP request
// that yields *temporary* credentials with a different scope. So
// the data flow is `base creds → SigV4-signed STS request →
// temporary creds`, and tangling the two would obscure that.
//
// Wiring: `AWSAssumeRoleLookup` returns a `func(name string) string`
// in the same shape as ChainLookup expects, so operators bolt it
// onto the chain by prepending it:
//
//	chain := creds.ChainLookup(
//	    vault.Lookup,
//	    creds.AWSAssumeRoleLookup(params), // ← if AWS_ASSUME_ROLE_ARN set
//	    creds.AWSDefaultChain(),
//	)
//
// (See cmd/agezt/main.go for the env-var driven wire-up.)
//
// Scope:
//   - Caches the temporary creds in-process until `Expiration -
//     refreshLeadTime`. No on-disk caching — daemons are long-lived
//     and operators who want cross-process caching should issue STS
//     once and write to the vault.
//   - Only the standard AssumeRole parameters are wired: RoleArn,
//     RoleSessionName, DurationSeconds, ExternalId. SAML and OIDC
//     federation paths are out of scope (operators already have
//     CLI tooling that lands the result in env or in ~/.aws).

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

	"github.com/ersinkoc/agezt/kernel/creds/sigv4"
)

// AssumeRoleParams configures a single AssumeRole call. Region is
// the AWS region whose STS endpoint to call (also goes into the
// SigV4 credential scope). BaseCreds are the long-lived signing
// credentials — usually obtained by reading the rest of the chain
// before assembling AssumeRoleParams.
type AssumeRoleParams struct {
	Region          string
	BaseCreds       sigv4.Creds
	RoleArn         string
	RoleSessionName string
	DurationSeconds int    // 900 (15 min) to 43200 (12 hr); 0 → AWS default of 3600
	ExternalID      string // optional; only when the role's trust policy demands one

	// Test seams. Production callers leave these nil/empty.
	Endpoint string                                  // override STS endpoint; default https://sts.{region}.amazonaws.com/
	HTTP     interface{ Do(*http.Request) (*http.Response, error) }
	Now      func() time.Time
}

// AssumedCreds is the result of a successful AssumeRole call. It
// includes the expiration so the caller can decide when to refresh.
type AssumedCreds struct {
	Creds      sigv4.Creds
	Expiration time.Time
}

// stsAssumeRoleEndpoint returns the regional STS endpoint URL.
// Empty region defaults to us-east-1 (the AWS legacy default;
// matches what the chain does when AWS_REGION is missing).
func stsAssumeRoleEndpoint(region, override string) string {
	if override != "" {
		return override
	}
	if region == "" {
		region = "us-east-1"
	}
	return "https://sts." + region + ".amazonaws.com/"
}

// AssumeRole performs a single sts:AssumeRole call. Caller is
// responsible for caching the result and refreshing before
// Expiration; see AWSAssumeRoleLookup for the cached-and-wired
// version most operators want.
func AssumeRole(ctx context.Context, p AssumeRoleParams) (*AssumedCreds, error) {
	if p.RoleArn == "" {
		return nil, errors.New("sts assume-role: RoleArn required")
	}
	if p.BaseCreds.AccessKeyID == "" || p.BaseCreds.SecretAccessKey == "" {
		return nil, errors.New("sts assume-role: BaseCreds (AccessKeyID/SecretAccessKey) required")
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
	form.Set("Action", "AssumeRole")
	form.Set("Version", "2011-06-15")
	form.Set("RoleArn", p.RoleArn)
	form.Set("RoleSessionName", sessionName)
	form.Set("DurationSeconds", strconv.Itoa(duration))
	if p.ExternalID != "" {
		form.Set("ExternalId", p.ExternalID)
	}
	body := []byte(form.Encode())

	endpoint := stsAssumeRoleEndpoint(p.Region, p.Endpoint)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("sts assume-role: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Force net/http to populate Content-Length so AWS doesn't
	// reject the request as chunked-encoding (STS doesn't allow it).
	req.ContentLength = int64(len(body))

	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	region := p.Region
	if region == "" {
		region = "us-east-1"
	}
	if err := sigv4.SignRequest(req, "sts", region, body, p.BaseCreds, now()); err != nil {
		return nil, fmt.Errorf("sts assume-role: sign: %w", err)
	}

	client := http.DefaultClient
	if p.HTTP != nil {
		if c, ok := p.HTTP.(*http.Client); ok {
			client = c
		} else {
			resp, err := p.HTTP.Do(req)
			if err != nil {
				return nil, fmt.Errorf("sts assume-role: http: %w", err)
			}
			return parseAssumeRoleResponse(resp)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sts assume-role: http: %w", err)
	}
	return parseAssumeRoleResponse(resp)
}

func parseAssumeRoleResponse(resp *http.Response) (*AssumedCreds, error) {
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("sts assume-role: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// STS error envelope is XML too, but we don't parse it
		// structurally — the status line + body excerpt is enough
		// for an operator to debug (typically "AccessDenied" or
		// "InvalidClientTokenId").
		excerpt := string(raw)
		if len(excerpt) > 512 {
			excerpt = excerpt[:512] + "..."
		}
		return nil, fmt.Errorf("sts assume-role: %s: %s", resp.Status, excerpt)
	}

	var env struct {
		XMLName xml.Name `xml:"AssumeRoleResponse"`
		Result  struct {
			Credentials struct {
				AccessKeyID     string `xml:"AccessKeyId"`
				SecretAccessKey string `xml:"SecretAccessKey"`
				SessionToken    string `xml:"SessionToken"`
				Expiration      string `xml:"Expiration"`
			} `xml:"Credentials"`
		} `xml:"AssumeRoleResult"`
	}
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("sts assume-role: parse XML: %w", err)
	}
	c := env.Result.Credentials
	if c.AccessKeyID == "" || c.SecretAccessKey == "" || c.SessionToken == "" {
		return nil, fmt.Errorf("sts assume-role: response missing credential fields: %s", string(raw))
	}
	exp, err := time.Parse(time.RFC3339, c.Expiration)
	if err != nil {
		return nil, fmt.Errorf("sts assume-role: parse Expiration %q: %w", c.Expiration, err)
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

// defaultSessionName builds a session name that an operator looking
// at CloudTrail can correlate back to this agezt instance: pid +
// unix-second timestamp, prefixed `agezt-`. Session names have a
// 2–64 char limit; this fits.
func defaultSessionName() string {
	return fmt.Sprintf("agezt-%d-%d", os.Getpid(), time.Now().Unix())
}

// refreshLeadTime is how long before expiry we proactively refresh
// cached assume-role credentials. 60s gives in-flight signed
// requests time to land before AWS rotates them out.
const refreshLeadTime = 60 * time.Second

// assumeRoleCache holds the most recent successful AssumeRole result
// for a given params instance. Concurrency: a single sync.Mutex —
// AssumeRole calls are infrequent (once per ~hour), so contention is
// not a concern.
type assumeRoleCache struct {
	mu     sync.Mutex
	creds  *AssumedCreds
	params AssumeRoleParams
}

func (c *assumeRoleCache) get(ctx context.Context, now time.Time) (*AssumedCreds, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.creds != nil && now.Before(c.creds.Expiration.Add(-refreshLeadTime)) {
		return c.creds, nil
	}
	fresh, err := AssumeRole(ctx, c.params)
	if err != nil {
		return nil, err
	}
	c.creds = fresh
	return fresh, nil
}

// AWSAssumeRoleLookup returns a lookup function compatible with
// ChainLookup that translates the canonical `AWS_ACCESS_KEY_ID` /
// `AWS_SECRET_ACCESS_KEY` / `AWS_SESSION_TOKEN` / `AWS_REGION`
// names to the cached AssumeRole result. Other names fall through
// (empty return — ChainLookup will try the next source).
//
// The cache is shared per lookup-function instance. Construct once
// at daemon start; re-using preserves the cache across calls.
//
// On AssumeRole failure the lookup returns empty for credential
// names so the chain falls through to base creds; the failure is
// silent here because the operator-facing wiring in cmd/agezt
// logs assume-role errors at the banner-print site.
func AWSAssumeRoleLookup(params AssumeRoleParams) func(name string) string {
	cache := &assumeRoleCache{params: params}
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
			// Propagate the region the operator configured so the
			// chain's region resolver doesn't have to re-read it.
			return params.Region
		}
		return ""
	}
}
