// SPDX-License-Identifier: MIT

// sts_helpers.go: STS endpoint, response parser, default session name, cache
// type + getter split off from sts.go during the Day 211 god-file refactor (#132).
// Public API unchanged.
package creds

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/creds/sigv4"
)


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
			excerpt = strutil.Ellipsis(excerpt, 512, "...")
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

// errCredFetchSuppressed is returned by the credential caches when a fetch
// failed recently and retries are suppressed for negCacheTTL. The lookups
// map every error to empty strings, so the observable chain behavior is
// unchanged — the suppressed retry just never hits the network.
var errCredFetchSuppressed = errors.New("creds: recent credential fetch failed; suppressing retries")

// assumeRoleCache holds the most recent successful AssumeRole result
// for a given params instance. Concurrency: a single sync.Mutex —
// AssumeRole calls are infrequent (once per ~hour), so contention is
// not a concern.
type assumeRoleCache struct {
	mu       sync.Mutex
	creds    *AssumedCreds
	params   AssumeRoleParams
	negCache time.Time // last failed fetch; retries suppressed for negCacheTTL
}

func (c *assumeRoleCache) get(ctx context.Context, now time.Time) (*AssumedCreds, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.creds != nil && now.Before(c.creds.Expiration.Add(-refreshLeadTime)) {
		return c.creds, nil
	}
	// A failure seconds ago means the fetch is doomed (revoked creds, a
	// refused or black-holed endpoint): re-attempting it for every credential
	// name in one chain resolution — and for every resolution until the
	// window lapses — just amplifies a slow failure. Mirrors imdsCache.
	if !c.negCache.IsZero() && now.Sub(c.negCache) < negCacheTTL {
		return nil, errCredFetchSuppressed
	}
	fresh, err := AssumeRole(ctx, c.params)
	if err != nil {
		c.negCache = now
		return nil, err
	}
	c.creds = fresh
	c.negCache = time.Time{}
	return fresh, nil
}
