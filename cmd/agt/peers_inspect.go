// SPDX-License-Identifier: MIT

package main

// agt peers inspect helpers: checkPeer + peersModels + fetchPeerModels.
// Carved out of peers.go during the Day 188 god-file split so the
// main file can stay focused on cmdPeers dispatch + the remote file
// can stay focused on peersRoute + peersRun.
// Public API unchanged.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/plugins/tools/peer"
)

func checkPeer(p peer.Peer) peerHealth {
	out := peerHealth{Name: p.Name, URL: p.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	endpoint := strings.TrimRight(p.URL, "/") + "/api/v1/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	if p.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		out.Error = "401 (token rejected)"
		return out
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out.Error = fmt.Sprintf("status %d", resp.StatusCode)
		return out
	}
	var body struct {
		Status     string `json:"status"`
		Version    string `json:"version"`
		ModelCount int    `json:"model_count"`
	}
	// Bound the response: a health doc is a few bytes, but a hostile or
	// misconfigured peer could stream an unbounded body and exhaust the operator's
	// CLI. Cap it like the remote_run tool does its own peer responses (M200).
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxPeerResponseBytes)).Decode(&body); err != nil {
		out.Error = "bad health response: " + err.Error()
		return out
	}
	out.Reachable = body.Status == "ok"
	out.Version = body.Version
	out.ModelCount = body.ModelCount
	if !out.Reachable {
		out.Error = "status=" + body.Status
	}
	return out
}

// peerModels is one peer's model inventory as reported by GET /api/v1/models.
type peerModels struct {
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Reachable bool     `json:"reachable"`
	Default   string   `json:"default,omitempty"`
	Models    []string `json:"models,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// peersModels runs `agt peers models [<name>]`: it fetches each peer's routable
// model set so an operator can decide where to dispatch a remote_run. With a name
// it queries just that peer; otherwise all configured peers (sorted). Exits 1 if
// any queried peer is unreachable. Tokens are never printed.
func peersModels(peers map[string]peer.Peer, name string, asJSON bool, stdout, stderr io.Writer) int {
	var names []string
	if name != "" {
		if _, ok := peers[name]; !ok {
			fmt.Fprintf(stderr, "%s peers models: unknown peer %q\n", brand.CLI, name)
			return 1
		}
		names = []string{name}
	} else {
		for n := range peers {
			names = append(names, n)
		}
		sort.Strings(names)
	}

	results := make([]peerModels, 0, len(names))
	for _, n := range names {
		results = append(results, fetchPeerModels(peers[n]))
	}

	if asJSON {
		b, _ := json.MarshalIndent(results, "", "  ")
		fmt.Fprintln(stdout, string(b))
		return 0
	}

	allOK := true
	for _, r := range results {
		if r.Reachable {
			list := strings.Join(r.Models, ", ")
			if list == "" {
				list = "(none)"
			}
			fmt.Fprintf(stdout, "  %-14s %s  default=%s  models: %s\n", r.Name, r.URL, r.Default, list)
		} else {
			allOK = false
			fmt.Fprintf(stdout, "  %-14s %s  UNREACHABLE: %s\n", r.Name, r.URL, r.Error)
		}
	}
	if !allOK {
		return 1
	}
	return 0
}

// fetchPeerModels queries one peer's GET /api/v1/models. Mirrors checkPeer: a 5s
// timeout, bearer auth, status handling, and a bounded-read decode (M201).
func fetchPeerModels(p peer.Peer) peerModels {
	out := peerModels{Name: p.Name, URL: p.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	endpoint := strings.TrimRight(p.URL, "/") + "/api/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	if p.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		out.Error = "401 (token rejected)"
		return out
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out.Error = fmt.Sprintf("status %d", resp.StatusCode)
		return out
	}
	var body struct {
		Default string   `json:"default"`
		Models  []string `json:"models"`
	}
	// Bounded read, same cap and rationale as checkPeer (M200/M201).
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxPeerResponseBytes)).Decode(&body); err != nil {
		out.Error = "bad models response: " + err.Error()
		return out
	}
	out.Reachable = true
	out.Default = body.Default
	out.Models = body.Models
	return out
}

// peerRoute is one peer's standing for a routing query: whether it serves the
// requested model, and whether it is the one remote_run would auto-route to.
type peerRoute struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	Reachable bool   `json:"reachable"`
	Serves    bool   `json:"serves"`
	Chosen    bool   `json:"chosen,omitempty"`
	Error     string `json:"error,omitempty"`
}

// peersRoute runs `agt peers route <model>`: it shows which peer remote_run would
// auto-route a task for <model> to, and the fallback order. It mirrors the tool's
// selection exactly — peers are queried in name order and the first reachable one
// that serves the model is "chosen" (M203). Exits 1 if no reachable peer serves it,
// so it composes in scripts. Tokens are never printed.
