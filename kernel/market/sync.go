// SPDX-License-Identifier: MIT

package market

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/agezt/agezt/kernel/netguard"
)

// DefaultSyncTimeout caps each HTTP fetch during a sync.
const DefaultSyncTimeout = 30 * time.Second

// MaxSyncBytes caps any single response body so a misbehaving source can't blow
// memory (mirrors catalog.MaxSyncBytes). Applies to the index and each pack.
const MaxSyncBytes int64 = 8 * 1024 * 1024

// Syncer fetches a remote marketplace.json and every pack it lists, validating
// and (optionally) verifying each, then caches the lot via Store.SaveMarketplace.
// Network access is screened by netguard (SSRF/CWE-918): a crafted or redirecting
// source URL cannot pivot to the cloud-metadata endpoint. Reusable + concurrency-safe.
type Syncer struct {
	HTTP    *http.Client
	Timeout time.Duration
}

// NewSyncer returns a Syncer with a netguard-screened client. Loopback/private
// are allowed (a self-hosted marketplace is legitimately local); link-local is
// blocked.
func NewSyncer() *Syncer {
	return &Syncer{
		HTTP:    netguard.New(netguard.AllowLoopback(), netguard.AllowPrivate()).HTTPClient(DefaultSyncTimeout),
		Timeout: DefaultSyncTimeout,
	}
}

// SyncResult summarizes one successful sync.
type SyncResult struct {
	Source    string `json:"source"`
	URL       string `json:"url"`
	Packs     int    `json:"packs"`
	Bytes     int    `json:"bytes"`
	FetchedMS int64  `json:"fetched_ms"`
}

// Sync fetches src's marketplace index + every pack, validates and verifies them,
// and caches the result. It is keep-last-good: nothing is written to the cache
// until the entire fetch+validate succeeds, so a failed sync leaves the prior
// catalogue intact. nowMS stamps the result; store receives the cached data.
func (s *Syncer) Sync(ctx context.Context, store *Store, src Source, nowMS int64) (SyncResult, error) {
	var res SyncResult
	res.Source = src.Name
	res.URL = src.URL
	if !isHTTPURL(src.URL) {
		return res, fmt.Errorf("market: source %q url must be http(s)", src.Name)
	}
	base, err := url.Parse(src.URL)
	if err != nil {
		return res, fmt.Errorf("market: parse source url: %w", err)
	}

	raw, err := s.fetch(ctx, src.URL)
	if err != nil {
		return res, err
	}
	res.Bytes += len(raw)
	var mp Marketplace
	if err := json.Unmarshal(raw, &mp); err != nil {
		return res, fmt.Errorf("market: parse marketplace.json from %s: %w", src.URL, err)
	}
	if len(mp.Packs) == 0 {
		return res, fmt.Errorf("market: %s lists no packs; refusing to overwrite the cached catalogue", src.URL)
	}
	// The cache is keyed by the LOCAL source name, not the remote's self-reported
	// name — so two sources can't collide and a remote can't claim "official".
	mp.Name = src.Name
	mp.Source = src.URL
	mp.Builtin = false

	packs := make([]Pack, 0, len(mp.Packs))
	for i := range mp.Packs {
		e := &mp.Packs[i]
		if e.Source == "" {
			return res, fmt.Errorf("market: entry %q has no source path", e.Name)
		}
		packURL, err := resolveRef(base, e.Source)
		if err != nil {
			return res, err
		}
		pb, err := s.fetch(ctx, packURL)
		if err != nil {
			return res, fmt.Errorf("market: fetch pack %q: %w", e.Name, err)
		}
		res.Bytes += len(pb)
		var p Pack
		if err := json.Unmarshal(pb, &p); err != nil {
			return res, fmt.Errorf("market: parse pack %q: %w", e.Name, err)
		}
		if p.Name != e.Name {
			return res, fmt.Errorf("market: pack body name %q != index entry %q", p.Name, e.Name)
		}
		if err := p.Validate(); err != nil {
			return res, err
		}
		// Content integrity: if the index pins a hash, the bytes must match.
		if e.SHA256 != "" {
			got, _ := p.ContentHash()
			if got != e.SHA256 {
				return res, fmt.Errorf("market: pack %q content hash mismatch (index %s, got %s)", e.Name, e.SHA256, got)
			}
		}
		// Trust: a source with a pinned key requires every pack to be signed by it.
		signed, verr := VerifyPack(p, src.PubKey)
		if verr != nil {
			return res, fmt.Errorf("market: verify pack %q: %w", e.Name, verr)
		}
		e.Signed = signed
		// Refresh the index entry's counts from the authoritative pack body.
		e.SkillCount, e.MCPCount, e.ToolCount = p.Counts()
		packs = append(packs, p)
	}

	mp.FormatVersion = FormatVersion
	mp.GeneratedUnixMS = nowMS
	if err := store.SaveMarketplace(src.Name, mp, packs); err != nil {
		return res, err
	}
	res.Packs = len(packs)
	res.FetchedMS = nowMS
	return res, nil
}

// resolveRef resolves a (possibly relative) pack source against the index URL,
// refusing to cross to a different host — a crafted relative path must not point
// the fetcher at an unrelated server.
func resolveRef(base *url.URL, ref string) (string, error) {
	r, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("market: bad pack source %q: %w", ref, err)
	}
	abs := base.ResolveReference(r)
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return "", fmt.Errorf("market: pack source %q resolves to a non-http scheme", ref)
	}
	if abs.Host != base.Host {
		return "", fmt.Errorf("market: pack source %q points off the marketplace host %q", ref, base.Host)
	}
	return abs.String(), nil
}

// deriveSourceName builds a kebab-case local handle from a marketplace URL's
// host (e.g. https://packs.example.com/m.json → "packs-example-com"), so `agt
// market add <url>` needs no explicit name.
func deriveSourceName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return "remote"
	}
	var b []rune
	for _, r := range u.Hostname() {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b = append(b, r)
		case r >= 'A' && r <= 'Z':
			b = append(b, r+('a'-'A'))
		default:
			b = append(b, '-')
		}
	}
	// Trim to nameRe: must start with a letter; collapse leading non-letters.
	for len(b) > 0 && !(b[0] >= 'a' && b[0] <= 'z') {
		b = b[1:]
	}
	if len(b) == 0 {
		return "remote"
	}
	if len(b) > 64 {
		b = b[:64]
	}
	return string(b)
}

func (s *Syncer) fetch(ctx context.Context, u string) ([]byte, error) {
	to := s.Timeout
	if to <= 0 {
		to = DefaultSyncTimeout
	}
	reqCtx, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("market: new request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "agezt-market-sync")
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("market: fetch %s: %w", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("market: fetch %s: %s", u, resp.Status)
	}
	limited := io.LimitReader(resp.Body, MaxSyncBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("market: read body: %w", err)
	}
	if int64(len(body)) > MaxSyncBytes {
		return nil, fmt.Errorf("market: body from %s exceeds %d bytes", u, MaxSyncBytes)
	}
	return body, nil
}
