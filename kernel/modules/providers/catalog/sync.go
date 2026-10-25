// SPDX-License-Identifier: MIT

package catalog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/netguard"
)

// guardedClient returns an HTTP client whose every connection (and redirect
// hop) is screened by netguard. Loopback and private ranges are permitted —
// catalog/Ollama endpoints are legitimately local — but link-local is blocked,
// so a malicious or redirecting catalog URL cannot pivot to the cloud-metadata
// endpoint (169.254.169.254) and exfiltrate instance credentials (SSRF, CWE-918).
func guardedClient(timeout time.Duration) *http.Client {
	return netguard.New(netguard.AllowLoopback(), netguard.AllowPrivate()).HTTPClient(timeout)
}

// DefaultSyncURL is the community-maintained source the catalog syncs
// from. Overridable via AGEZT_CATALOG_URL on the daemon side.
//
// The constant lives in the contract layer (W3.51); this package keeps the
// name.
const DefaultSyncURL = catalogapi.DefaultSyncURL

// DefaultSyncTimeout caps the HTTP fetch. Tight enough that a wedged
// remote can't hang `agt catalog sync` for minutes.
const DefaultSyncTimeout = 30 * time.Second

// MaxSyncBytes caps the response body so a misbehaving source can't
// blow memory. 8 MiB is well above the ~1.5 MiB models.dev currently
// publishes; trip it and we know something's wrong.
const MaxSyncBytes int64 = 8 * 1024 * 1024

// Syncer fetches and parses a remote catalog. Reusable; safe for
// concurrent Sync calls.
type Syncer struct {
	HTTP    *http.Client
	URL     string
	Timeout time.Duration
}

// NewSyncer returns a Syncer with sensible defaults.
func NewSyncer() *Syncer {
	return &Syncer{
		HTTP:    guardedClient(DefaultSyncTimeout),
		URL:     DefaultSyncURL,
		Timeout: DefaultSyncTimeout,
	}
}

// SyncResult summarises one successful Sync for events + reporting.
//
// The row lives in the contract layer (W3.51); this package keeps the name.
type SyncResult = catalogapi.SyncResult

// The syncer answers the contract's port.
var _ catalogapi.Syncer = (*Syncer)(nil)

// Sync fetches the catalog from s.URL, parses + validates it, and
// returns the raw bytes (for direct writing to api.json) plus a
// Catalog (for in-memory use) plus a SyncResult summary.
//
// url and timeout are the CALLER's to choose (W3.51): an empty url means
// the syncer's default source, a timeout of zero or less means the
// syncer's default cap. The port cannot carry mutable state that a caller
// would have to reach through, so they are parameters.
func (s *Syncer) Sync(ctx context.Context, url string, timeout time.Duration) (raw []byte, cat *Catalog, res SyncResult, err error) {
	if url == "" {
		url = s.URL
	}
	if url == "" {
		return nil, nil, res, fmt.Errorf("catalog: empty sync URL")
	}
	start := time.Now()

	to := timeout
	if to <= 0 {
		to = s.Timeout
	}
	if to <= 0 {
		to = DefaultSyncTimeout
	}
	reqCtx, cancel := context.WithTimeout(ctx, to)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, res, fmt.Errorf("catalog: new request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "agezt-catalog-sync")

	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, nil, res, fmt.Errorf("catalog: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, res, fmt.Errorf("catalog: fetch %s: %s", url, resp.Status)
	}

	limited := io.LimitReader(resp.Body, MaxSyncBytes+1)
	raw, err = io.ReadAll(limited)
	if err != nil {
		return nil, nil, res, fmt.Errorf("catalog: read body: %w", err)
	}
	if int64(len(raw)) > MaxSyncBytes {
		return nil, nil, res, fmt.Errorf("catalog: body exceeds %d bytes", MaxSyncBytes)
	}

	cat, err = ParseAPIFile(raw)
	if err != nil {
		return raw, nil, res, err
	}
	// A syntactically valid but empty payload (e.g. `null` or `{}` from a proxy/CDN
	// returning 200) parses without error but carries zero providers. Treat that as a
	// failure so the caller never overwrites a good api.json with an empty catalog —
	// which would leave the Governor with no models to route to (a self-inflicted
	// outage). Fail-safe: the prior catalog is kept (M425).
	if len(cat.Providers) == 0 {
		return raw, nil, res, fmt.Errorf("catalog: sync from %s returned no providers; refusing to overwrite the existing catalog", url)
	}
	res.URL = url
	res.Bytes = len(raw)
	res.ProviderCount = len(cat.Providers)
	for _, p := range cat.Providers {
		res.ModelCount += len(p.Models)
	}
	res.Duration = time.Since(start)
	res.FetchedAt = time.Now().UTC()
	return raw, cat, res, nil
}
