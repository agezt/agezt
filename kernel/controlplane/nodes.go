// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/settings"
)

const nodeHealthResponseLimit = 1 << 20

type nodePeer struct {
	Name  string
	URL   string
	Token string
}

func (s *Server) handleNodeRegistry(conn net.Conn, req Request) {
	nodes := []map[string]any{{
		"id":        "local",
		"name":      "local",
		"local":     true,
		"reachable": true,
		"status":    "ok",
		"version":   brand.Version,
		"model":     s.k.Model(),
		"capabilities": []string{
			"controlplane",
			"webui",
			"agent-runtime",
		},
	}}
	if _, ok := s.k.Tools()["remote_run"]; ok {
		nodes[0]["capabilities"] = append(nodes[0]["capabilities"].([]string), "remote-run")
	}

	spec := s.nodePeerSpec()
	peers, err := parseNodePeers(spec)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
			"nodes":      nodes,
			"count":      len(nodes),
			"peer_count": 0,
			"error":      err.Error(),
		}})
		return
	}
	sort.Slice(peers, func(i, j int) bool { return peers[i].Name < peers[j].Name })
	for _, p := range peers {
		nodes = append(nodes, probeNodePeer(p))
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"nodes":                 nodes,
		"count":                 len(nodes),
		"peer_count":            len(peers),
		"remote_run_registered": s.k.Tools()["remote_run"] != nil,
	}})
}

func (s *Server) nodePeerSpec() string {
	if v := os.Getenv(brand.EnvPrefix + "PEERS"); strings.TrimSpace(v) != "" {
		return v
	}
	vault := creds.NewStore(s.baseDir)
	if vault.Load() == nil {
		if v := vault.Get(brand.EnvPrefix + "PEERS"); strings.TrimSpace(v) != "" {
			return v
		}
	}
	store := settings.NewStore(s.baseDir)
	if store.Load() == nil {
		if v, _ := store.Get(brand.EnvPrefix + "PEERS"); strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func parseNodePeers(spec string) ([]nodePeer, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	seen := map[string]bool{}
	var out []nodePeer
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, rest, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("peer %q: expected name=url|token", part)
		}
		name = strings.TrimSpace(name)
		rest = strings.TrimSpace(rest)
		urlStr, token, _ := strings.Cut(rest, "|")
		urlStr = strings.TrimSpace(urlStr)
		token = strings.TrimSpace(token)
		if name == "" || urlStr == "" {
			return nil, fmt.Errorf("peer %q: name and url are required", part)
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate peer %q", name)
		}
		u, err := url.Parse(urlStr)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, fmt.Errorf("peer %q: url must be http(s)", name)
		}
		seen[name] = true
		out = append(out, nodePeer{Name: name, URL: strings.TrimRight(urlStr, "/"), Token: token})
	}
	return out, nil
}

func probeNodePeer(p nodePeer) map[string]any {
	row := map[string]any{
		"id":        "peer:" + p.Name,
		"name":      p.Name,
		"local":     false,
		"url":       p.URL,
		"auth":      "none",
		"reachable": false,
		"status":    "unreachable",
	}
	if p.Token != "" {
		row["auth"] = "token"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/api/v1/health", nil)
	if err != nil {
		row["error"] = err.Error()
		return row
	}
	if p.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		row["error"] = err.Error()
		return row
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		row["error"] = "401 (token rejected)"
		return row
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		row["error"] = fmt.Sprintf("status %d", resp.StatusCode)
		return row
	}
	var body struct {
		Status     string `json:"status"`
		Version    string `json:"version"`
		ModelCount int    `json:"model_count"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, nodeHealthResponseLimit)).Decode(&body); err != nil {
		row["error"] = "bad health response: " + err.Error()
		return row
	}
	row["status"] = body.Status
	row["reachable"] = body.Status == "ok"
	row["version"] = body.Version
	row["model_count"] = body.ModelCount
	if body.Status != "ok" {
		row["error"] = "status=" + body.Status
	}
	return row
}
