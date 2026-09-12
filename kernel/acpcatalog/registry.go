// SPDX-License-Identifier: MIT

// Registry client: NewRegistryClient + Fetch + fetch.
// Code extracted from registry.go during the Day-70 god-file split. Public API unchanged.
package acpcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)






// OfficialRegistryURL is the stable v1 index published by the ACP project.
const OfficialRegistryURL = "https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json"

const (
	registryMaxBytes  = 2 << 20
	registryMaxAgents = 512
	registryCacheTTL  = 15 * time.Minute
)

var registryIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var registryEnvPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// PackageDistribution is an npx/uvx launch recipe from the official registry.
type PackageDistribution struct {
	Package string            `json:"package"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// BinaryTarget describes one downloadable binary for a concrete OS/architecture.
type BinaryTarget struct {
	Archive string            `json:"archive"`
	Cmd     string            `json:"cmd"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// Distribution mirrors the v1 ACP registry distribution union.
type Distribution struct {
	Binary map[string]BinaryTarget `json:"binary,omitempty"`
	NPX    *PackageDistribution    `json:"npx,omitempty"`
	UVX    *PackageDistribution    `json:"uvx,omitempty"`
}

// RegistryAgent is one official ACP registry manifest.
type RegistryAgent struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Description  string       `json:"description"`
	Repository   string       `json:"repository,omitempty"`
	Website      string       `json:"website,omitempty"`
	Authors      []string     `json:"authors,omitempty"`
	License      string       `json:"license,omitempty"`
	Icon         string       `json:"icon,omitempty"`
	Distribution Distribution `json:"distribution"`
}

// Registry is the official aggregated v1 document.
type Registry struct {
	Version string          `json:"version"`
	Agents  []RegistryAgent `json:"agents"`
}

type registrySnapshot struct {
	registry  Registry
	fetchedAt time.Time
}

// RegistryClient fetches and validates the official index with a bounded body
// and an in-memory TTL cache. A stale snapshot remains usable when refresh
// fails, so a transient CDN outage never erases the UI catalog.
type RegistryClient struct {
	URL  string
	HTTP *http.Client
	TTL  time.Duration
	Now  func() time.Time

	mu       sync.Mutex
	snapshot *registrySnapshot
}

func NewRegistryClient(url string) *RegistryClient {
	return &RegistryClient{
		URL:  url,
		HTTP: &http.Client{Timeout: 6 * time.Second},
		TTL:  registryCacheTTL,
		Now:  time.Now,
	}
}

// DefaultRegistry is shared by the control-plane inventory and the acp_agent
// selector, so a UI refresh also warms delegation lookups.
var DefaultRegistry = NewRegistryClient(OfficialRegistryURL)

// Fetch returns the current validated registry. cached reports whether the
// returned data came from memory. When a forced refresh fails after a previous
// success, the stale snapshot is returned together with the refresh error.
func (c *RegistryClient) Fetch(ctx context.Context, force bool) (reg Registry, fetchedAt time.Time, cached bool, err error) {
	if c == nil {
		return Registry{}, time.Time{}, false, errors.New("ACP registry client is nil")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	ttl := c.TTL
	if ttl <= 0 {
		ttl = registryCacheTTL
	}
	if !force && c.snapshot != nil && now.Sub(c.snapshot.fetchedAt) < ttl {
		return c.snapshot.registry, c.snapshot.fetchedAt, true, nil
	}

	reg, fetchErr := c.fetch(ctx)
	if fetchErr != nil {
		if c.snapshot != nil {
			return c.snapshot.registry, c.snapshot.fetchedAt, true, fetchErr
		}
		return Registry{}, time.Time{}, false, fetchErr
	}
	c.snapshot = &registrySnapshot{registry: reg, fetchedAt: now.UTC()}
	return reg, c.snapshot.fetchedAt, false, nil
}

func (c *RegistryClient) fetch(ctx context.Context) (Registry, error) {
	url := strings.TrimSpace(c.URL)
	if url == "" {
		return Registry{}, errors.New("ACP registry URL is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Registry{}, fmt.Errorf("build ACP registry request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "agezt-acp-registry/1")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 6 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Registry{}, fmt.Errorf("fetch ACP registry: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Registry{}, fmt.Errorf("fetch ACP registry: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > registryMaxBytes {
		return Registry{}, fmt.Errorf("ACP registry exceeds %d bytes", registryMaxBytes)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, registryMaxBytes+1))
	if err != nil {
		return Registry{}, fmt.Errorf("read ACP registry: %w", err)
	}
	if len(body) > registryMaxBytes {
		return Registry{}, fmt.Errorf("ACP registry exceeds %d bytes", registryMaxBytes)
	}
	var reg Registry
	if err := json.Unmarshal(body, &reg); err != nil {
		return Registry{}, fmt.Errorf("decode ACP registry: %w", err)
	}
	if err := validateRegistry(reg); err != nil {
		return Registry{}, err
	}
	return reg, nil
}
