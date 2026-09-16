// SPDX-License-Identifier: MIT
//
// kernel/creds webIdentityCache.get (the per-process OIDC token reuse cache).
// Extracted from web_identity.go during Day 211 god-file refactor (#93).
// Public API unchanged.
package creds

import (
	"context"
	"sync"
	"time"
)

// Mirrors assumeRoleCache; refreshes when within refreshLeadTime of expiry.
type webIdentityCache struct {
	mu       sync.Mutex
	creds    *AssumedCreds
	params   WebIdentityParams
	negCache time.Time // last failed fetch; retries suppressed for negCacheTTL
}

func (c *webIdentityCache) get(ctx context.Context, now time.Time) (*AssumedCreds, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.creds != nil && now.Before(c.creds.Expiration.Add(-refreshLeadTime)) {
		return c.creds, nil
	}
	// Negative cache, mirroring assumeRoleCache/imdsCache: a just-failed
	// fetch is doomed; don't re-run it for every credential name in one
	// chain resolution.
	if !c.negCache.IsZero() && now.Sub(c.negCache) < negCacheTTL {
		return nil, errCredFetchSuppressed
	}
	fresh, err := AssumeRoleWithWebIdentity(ctx, c.params)
	if err != nil {
		c.negCache = now
		return nil, err
	}
	c.creds = fresh
	c.negCache = time.Time{}
	return fresh, nil
}
