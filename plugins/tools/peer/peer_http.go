// SPDX-License-Identifier: MIT

// Peer tool: HTTP request helpers + peer-spec parsers.
// Code extracted from peer.go during the Day-99 god-file split.
// Public API unchanged.
package peer


import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/meshctx"
)

func httpPost(ctx context.Context, endpoint, token string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	// Forward the delegation hop count +1 so the peer (and the chain beyond it) is
	// bounded against federation loops (M209).
	req.Header.Set(meshctx.HopHeader, strconv.Itoa(meshctx.Hop(ctx)+1))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, rb, nil
}

// httpListModels is the default lister: GET {url}/api/v1/models, returning the
// peer's routable model ids. The response is bounded-read (1 MiB) so a hostile peer
// can't exhaust memory during auto-routing discovery.
func httpListModels(ctx context.Context, p Peer) ([]string, error) {
	endpoint := strings.TrimRight(p.URL, "/") + "/api/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if p.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var body struct {
		Models []string `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, err
	}
	return body.Models, nil
}

// ParsePeers parses the AGEZT_PEERS spec: a comma-separated list of peers, each
// "name=url|token" (token optional). Whitespace is trimmed; the URL must be
// http(s). A malformed entry is a hard error so a misconfigured mesh is caught
// at startup.
func ParsePeers(spec string) (map[string]Peer, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	peers := map[string]Peer{}
	for _, raw := range strings.Split(spec, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		name, rest, ok := strings.Cut(entry, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("peer: entry %q must be name=url[|token]", entry)
		}
		urlStr, token, _ := strings.Cut(rest, "|")
		urlStr = strings.TrimSpace(urlStr)
		token = strings.TrimSpace(token)
		u, err := url.Parse(urlStr)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("peer %q: invalid URL %q (need http(s)://host…)", name, urlStr)
		}
		// A duplicate name would silently overwrite the earlier entry — you'd think
		// you had N peers but the mesh would only know N-1, and `remote_run` to the
		// shadowed name would hit the wrong node. Reject it so the misconfig is caught
		// at startup, like other malformed specs (M215).
		if _, dup := peers[name]; dup {
			return nil, fmt.Errorf("peer %q is defined more than once", name)
		}
		peers[name] = Peer{Name: name, URL: urlStr, Token: token}
	}
	return peers, nil
}

// ParseTenantPeers decodes the AGEZT_TENANT_PEERS spec (M219): a JSON object mapping a
// tenant id to that tenant's own AGEZT_PEERS-style spec, e.g.
//
//	{"alpha":"nodeA=http://a:8800|tokA","beta":"nodeB=https://b:8800"}
//
// Each value is parsed with ParsePeers, so the same validation (URL scheme, duplicate
// name) applies per tenant. A JSON object rather than per-tenant env vars keeps every
// tenant id expressible (including ones with characters an env-var name can't hold).
// Empty / whitespace-only spec → nil, no error. A tenant whose value parses to no peers
// is dropped (it would just fall back to the global set anyway).
func ParseTenantPeers(spec string) (map[string]map[string]Peer, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	var raw map[string]string
	if err := json.Unmarshal([]byte(spec), &raw); err != nil {
		return nil, fmt.Errorf("tenant peers: invalid JSON: %w", err)
	}
	out := map[string]map[string]Peer{}
	for tenant, peerSpec := range raw {
		tenant = strings.TrimSpace(tenant)
		if tenant == "" {
			return nil, fmt.Errorf("tenant peers: empty tenant id")
		}
		peers, err := ParsePeers(peerSpec)
		if err != nil {
			return nil, fmt.Errorf("tenant %q peers: %w", tenant, err)
		}
		if len(peers) > 0 {
			out[tenant] = peers
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// Describe renders a one-line banner summary of the peers (tokens redacted).
func Describe(peers map[string]Peer) string {
	if len(peers) == 0 {
		return ""
	}
	names := make([]string, 0, len(peers))
	for n := range peers {
		names = append(names, n)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(peers))
	for _, n := range names {
		p := peers[n]
		auth := ""
		if p.Token != "" {
			auth = " (token)"
		}
		parts = append(parts, fmt.Sprintf("%s→%s%s", n, p.URL, auth))
	}
	return fmt.Sprintf("%d peer(s): %s", len(peers), strings.Join(parts, ", "))
}
