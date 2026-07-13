// SPDX-License-Identifier: MIT

package acpcatalog

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// OfficialClientsURL is the source-of-truth Clients page in the official ACP
// protocol repository. Unlike the agent Registry, ACP does not currently
// publish a separate clients.json index, so this bounded parser consumes the
// maintained MDX list directly from the repository.
const OfficialClientsURL = "https://raw.githubusercontent.com/agentclientprotocol/agent-client-protocol/main/docs/get-started/clients.mdx"

const clientsMaxBytes = 256 << 10

var (
	clientsHeadingPattern = regexp.MustCompile(`^##\s+(.+?)\s*$`)
	clientsBulletPattern  = regexp.MustCompile(`^\s*-\s+(.+?)\s*$`)
	clientsLinkPattern    = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^)\s]+)\)`)
)

// ClientEntry is one item maintained on the official ACP Clients page. The
// page intentionally includes clients, frameworks and connectors; Category
// preserves that official distinction instead of claiming every entry is an IDE.
type ClientEntry struct {
	Name        string `json:"name"`
	URL         string `json:"url,omitempty"`
	Category    string `json:"category"`
	Description string `json:"description,omitempty"`
}

type clientsSnapshot struct {
	entries   []ClientEntry
	revision  string
	fetchedAt time.Time
}

// ClientsClient fetches and caches the official repository page. It mirrors
// RegistryClient's keep-last-good behavior so a GitHub outage does not erase a
// previously validated list.
type ClientsClient struct {
	URL  string
	HTTP *http.Client
	TTL  time.Duration
	Now  func() time.Time

	mu       sync.Mutex
	snapshot *clientsSnapshot
}

func NewClientsClient(url string) *ClientsClient {
	return &ClientsClient{
		URL:  url,
		HTTP: &http.Client{Timeout: 6 * time.Second},
		TTL:  registryCacheTTL,
		Now:  time.Now,
	}
}

var DefaultClients = NewClientsClient(OfficialClientsURL)

func (c *ClientsClient) Fetch(ctx context.Context, force bool) (entries []ClientEntry, revision string, fetchedAt time.Time, cached bool, err error) {
	if c == nil {
		return nil, "", time.Time{}, false, errors.New("ACP clients source is nil")
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
		return cloneClientEntries(c.snapshot.entries), c.snapshot.revision, c.snapshot.fetchedAt, true, nil
	}

	entries, revision, fetchErr := c.fetch(ctx)
	if fetchErr != nil {
		if c.snapshot != nil {
			return cloneClientEntries(c.snapshot.entries), c.snapshot.revision, c.snapshot.fetchedAt, true, fetchErr
		}
		return nil, "", time.Time{}, false, fetchErr
	}
	c.snapshot = &clientsSnapshot{entries: cloneClientEntries(entries), revision: revision, fetchedAt: now.UTC()}
	return entries, revision, c.snapshot.fetchedAt, false, nil
}

func (c *ClientsClient) fetch(ctx context.Context) ([]ClientEntry, string, error) {
	url := strings.TrimSpace(c.URL)
	if url == "" {
		return nil, "", errors.New("ACP clients source URL is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build ACP clients request: %w", err)
	}
	req.Header.Set("Accept", "text/plain, text/markdown")
	req.Header.Set("User-Agent", "agezt-acp-clients/1")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 6 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch ACP clients: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("fetch ACP clients: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > clientsMaxBytes {
		return nil, "", fmt.Errorf("ACP clients source exceeds %d bytes", clientsMaxBytes)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, clientsMaxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read ACP clients: %w", err)
	}
	if len(body) > clientsMaxBytes {
		return nil, "", fmt.Errorf("ACP clients source exceeds %d bytes", clientsMaxBytes)
	}
	entries, err := ParseClientsMDX(string(body))
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(body)
	return entries, fmt.Sprintf("%x", digest[:6]), nil
}

// ParseClientsMDX extracts every list item under a level-two category heading.
// It accepts both linked and plain parent items and keeps nested integrations as
// their own entries—the official page treats those plugins/connectors as usable
// ACP client surfaces too.
func ParseClientsMDX(src string) ([]ClientEntry, error) {
	category := ""
	entries := make([]ClientEntry, 0, 96)
	seen := map[string]struct{}{}
	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimRight(raw, "\r")
		if match := clientsHeadingPattern.FindStringSubmatch(line); match != nil {
			category = strings.TrimSpace(match[1])
			continue
		}
		match := clientsBulletPattern.FindStringSubmatch(line)
		if match == nil || category == "" {
			continue
		}
		item := strings.TrimSpace(match[1])
		name, url := clientNameAndURL(item)
		name = cleanMarkdown(name)
		if name == "" {
			continue
		}
		description := cleanMarkdown(clientsLinkPattern.ReplaceAllString(item, "$1"))
		if strings.EqualFold(description, name) {
			description = ""
		}
		key := strings.ToLower(category + "\x00" + name + "\x00" + url)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		entries = append(entries, ClientEntry{Name: name, URL: url, Category: category, Description: description})
	}
	if len(entries) == 0 {
		return nil, errors.New("ACP clients source contained no categorized list entries")
	}
	return entries, nil
}

func clientNameAndURL(item string) (string, string) {
	match := clientsLinkPattern.FindStringSubmatchIndex(item)
	if match == nil {
		return trimClientPrefix(item), ""
	}
	linkText := item[match[2]:match[3]]
	url := item[match[4]:match[5]]
	prefix := strings.TrimSpace(item[:match[0]])
	prefix = strings.TrimRight(prefix, "—–-: ")
	if prefix != "" && !strings.HasPrefix(strings.ToLower(prefix), "through") && !strings.HasPrefix(strings.ToLower(prefix), "powered") {
		return trimClientPrefix(prefix), url
	}
	return linkText, url
}

func trimClientPrefix(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, " via")
	s = strings.TrimSuffix(s, " through the")
	s = strings.TrimRight(strings.TrimSpace(s), "—–-: ")
	for _, separator := range []string{" — ", " – ", " - ", " via ", ": "} {
		if before, _, ok := strings.Cut(s, separator); ok && strings.TrimSpace(before) != "" {
			return strings.TrimSpace(before)
		}
	}
	return s
}

func cleanMarkdown(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	return strings.Join(strings.Fields(s), " ")
}

func cloneClientEntries(in []ClientEntry) []ClientEntry {
	return append([]ClientEntry(nil), in...)
}

func attachClientsResult(inv Inventory, source string, entries []ClientEntry, revision string, fetchedAt time.Time, cached bool, err error) Inventory {
	inv.ClientsSource = source
	inv.Clients = entries
	inv.ClientCount = len(entries)
	inv.ClientsRevision = revision
	inv.ClientsCached = cached
	if !fetchedAt.IsZero() {
		inv.ClientsFetchedAt = fetchedAt.UTC().Format(time.RFC3339)
	}
	if err != nil {
		inv.ClientsError = err.Error()
	}
	return inv
}
