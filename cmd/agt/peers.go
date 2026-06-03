// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/plugins/tools/peer"
)

// cmdPeers implements `agt peers` (M8 mesh): list the peer Agezt nodes
// configured via AGEZT_PEERS and check each one's health over its native REST
// surface (GET /api/v1/health). It is a self-contained client command — it reads
// the same AGEZT_PEERS spec the daemon's remote_run tool uses and pings the
// peers directly, so an operator can verify the mesh wiring without a local
// daemon. Tokens are never printed.
func cmdPeers(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "list", "": // default verb
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s peers [list] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "  list configured peers (AGEZT_PEERS) and check each one's REST /api/v1/health\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s peers: unknown argument %q\n", brand.CLI, a)
			return 2
		}
	}

	peers, err := peer.ParsePeers(os.Getenv(brand.EnvPrefix + "PEERS"))
	if err != nil {
		fmt.Fprintf(stderr, "%s peers: %v\n", brand.CLI, err)
		return 1
	}
	if len(peers) == 0 {
		if asJSON {
			fmt.Fprintln(stdout, "[]")
		} else {
			fmt.Fprintf(stdout, "No peers configured. Set AGEZT_PEERS=\"name=url|token,…\".\n")
		}
		return 0
	}

	names := make([]string, 0, len(peers))
	for n := range peers {
		names = append(names, n)
	}
	sort.Strings(names)

	results := make([]peerHealth, 0, len(names))
	for _, n := range names {
		results = append(results, checkPeer(peers[n]))
	}

	if asJSON {
		b, _ := json.MarshalIndent(results, "", "  ")
		fmt.Fprintln(stdout, string(b))
		return 0
	}

	allOK := true
	for _, r := range results {
		if r.Reachable {
			fmt.Fprintf(stdout, "  %-14s %s  OK (version %s, %d model(s))\n", r.Name, r.URL, r.Version, r.ModelCount)
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

// maxPeerHealthBytes caps a peer's /api/v1/health response. A legitimate health
// doc is a few bytes; the cap matches the remote_run tool's 1 MiB peer-response
// limit so a hostile/misconfigured peer can't exhaust the CLI's memory (M200).
const maxPeerHealthBytes = 1 << 20

type peerHealth struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Reachable  bool   `json:"reachable"`
	Version    string `json:"version,omitempty"`
	ModelCount int    `json:"model_count,omitempty"`
	Error      string `json:"error,omitempty"`
}

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
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxPeerHealthBytes)).Decode(&body); err != nil {
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
