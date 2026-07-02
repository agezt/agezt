// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	verb := "list"
	var pos []string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s peers [list | models [<name>] | route <model> | run <peer> <corr> | artifacts <peer> <corr> | artifact-get <peer> <artifact_id> <out_file>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "  list    configured peers (AGEZT_PEERS) + each one's REST /api/v1/health\n")
			fmt.Fprintf(stdout, "  models  the models each peer can route (GET /api/v1/models); <name> filters to one\n")
			fmt.Fprintf(stdout, "  route   which peer remote_run would auto-route a <model> to, and the fallback order\n")
			fmt.Fprintf(stdout, "  run     metadata-only remote run event arc (GET /api/v1/runs/<corr>); payloads are not printed\n")
			fmt.Fprintf(stdout, "  artifacts metadata-only remote artifact index entries (GET /api/v1/artifacts?corr=<corr>); bytes are not printed\n")
			fmt.Fprintf(stdout, "  artifact-get policy-gated remote artifact bytes (GET /api/v1/artifacts/<id>/bytes); writes <out_file>\n")
			return 0
		case a == "list" || a == "models" || a == "route" || a == "run" || a == "artifacts" || a == "artifact-get":
			verb = a
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s peers: unknown flag %q\n", brand.CLI, a)
			return 2
		default:
			pos = append(pos, a)
		}
	}
	switch verb {
	case "list":
		if len(pos) != 0 {
			fmt.Fprintf(stderr, "%s peers list: unexpected argument %q\n", brand.CLI, pos[0])
			return 2
		}
	case "models":
		if len(pos) > 1 {
			fmt.Fprintf(stderr, "%s peers models: unexpected argument %q\n", brand.CLI, pos[1])
			return 2
		}
	case "route":
		if len(pos) != 1 {
			fmt.Fprintf(stderr, "%s peers route: a <model> is required\n", brand.CLI)
			return 2
		}
	case "run":
		if len(pos) != 2 {
			fmt.Fprintf(stderr, "%s peers run: <peer> and <correlation_id> are required\n", brand.CLI)
			return 2
		}
	case "artifacts":
		if len(pos) != 2 {
			fmt.Fprintf(stderr, "%s peers artifacts: <peer> and <correlation_id> are required\n", brand.CLI)
			return 2
		}
	case "artifact-get":
		if len(pos) != 3 {
			fmt.Fprintf(stderr, "%s peers artifact-get: <peer>, <artifact_id>, and <out_file> are required\n", brand.CLI)
			return 2
		}
	default:
		fmt.Fprintf(stderr, "%s peers: unknown subcommand %q\n", brand.CLI, verb)
		return 2
	}

	peers, err := peer.ParsePeers(os.Getenv(brand.EnvPrefix + "PEERS"))
	if err != nil {
		fmt.Fprintf(stderr, "%s peers: %v\n", brand.CLI, err)
		return 1
	}
	if len(peers) == 0 {
		if verb == "run" || verb == "models" || verb == "route" || verb == "artifacts" || verb == "artifact-get" {
			fmt.Fprintf(stderr, "%s peers %s: no peers configured (set AGEZT_PEERS=\"name=url|token,…\")\n", brand.CLI, verb)
			return 1
		}
		if asJSON {
			fmt.Fprintln(stdout, "[]")
		} else {
			fmt.Fprintf(stdout, "No peers configured. Set AGEZT_PEERS=\"name=url|token,…\".\n")
		}
		return 0
	}

	if verb == "models" {
		name := ""
		if len(pos) == 1 {
			name = pos[0]
		}
		return peersModels(peers, name, asJSON, stdout, stderr)
	}
	if verb == "route" {
		return peersRoute(peers, pos[0], asJSON, stdout, stderr)
	}
	if verb == "run" {
		return peersRun(peers, pos[0], pos[1], asJSON, stdout, stderr)
	}
	if verb == "artifacts" {
		return peersArtifacts(peers, pos[0], pos[1], asJSON, stdout, stderr)
	}
	if verb == "artifact-get" {
		return peersArtifactGet(peers, pos[0], pos[1], pos[2], asJSON, stdout, stderr)
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

// maxPeerResponseBytes caps a peer's REST response (health / models). A legitimate
// reply is a few bytes; the cap matches the remote_run tool's 1 MiB peer-response
// limit so a hostile/misconfigured peer can't exhaust the CLI's memory (M200/M201).
const maxPeerResponseBytes = 1 << 20
const maxPeerRunResponseBytes = 4 << 20
const maxPeerArtifactResponseBytes = 1 << 20
const maxPeerArtifactBytesResponseBytes = 96 << 20
const maxPeerArtifactEntries = 200

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
func peersRoute(peers map[string]peer.Peer, model string, asJSON bool, stdout, stderr io.Writer) int {
	names := make([]string, 0, len(peers))
	for n := range peers {
		names = append(names, n)
	}
	sort.Strings(names)

	results := make([]peerRoute, 0, len(names))
	chosen := ""
	for _, n := range names {
		pm := fetchPeerModels(peers[n])
		e := peerRoute{Name: n, URL: peers[n].URL, Reachable: pm.Reachable, Error: pm.Error}
		if pm.Reachable {
			for _, m := range pm.Models {
				if m == model {
					e.Serves = true
					break
				}
			}
			if e.Serves && chosen == "" {
				e.Chosen = true
				chosen = n
			}
		}
		results = append(results, e)
	}

	if asJSON {
		b, _ := json.MarshalIndent(results, "", "  ")
		fmt.Fprintln(stdout, string(b))
		if chosen == "" {
			return 1
		}
		return 0
	}

	if chosen != "" {
		fmt.Fprintf(stdout, "model %q — would route to: %s\n", model, chosen)
	} else {
		fmt.Fprintf(stdout, "model %q — no reachable peer serves it\n", model)
	}
	for _, e := range results {
		switch {
		case !e.Reachable:
			fmt.Fprintf(stdout, "  %-14s %s  UNREACHABLE: %s\n", e.Name, e.URL, e.Error)
		case e.Chosen:
			fmt.Fprintf(stdout, "  %-14s %s  serves (chosen)\n", e.Name, e.URL)
		case e.Serves:
			fmt.Fprintf(stdout, "  %-14s %s  serves (fallback)\n", e.Name, e.URL)
		default:
			fmt.Fprintf(stdout, "  %-14s %s  does not serve\n", e.Name, e.URL)
		}
	}
	if chosen == "" {
		return 1
	}
	return 0
}

type peerRunEvent struct {
	ID            string `json:"id"`
	Seq           int64  `json:"seq"`
	TSUnixMS      int64  `json:"ts_unix_ms"`
	Subject       string `json:"subject"`
	Actor         string `json:"actor"`
	Kind          string `json:"kind"`
	CorrelationID string `json:"correlation_id"`
	Hash          string `json:"hash,omitempty"`
}

type peerRunArc struct {
	Peer          string         `json:"peer"`
	URL           string         `json:"url"`
	CorrelationID string         `json:"correlation_id"`
	Count         int            `json:"count"`
	Events        []peerRunEvent `json:"events"`
	Error         string         `json:"error,omitempty"`
}

func peersRun(peers map[string]peer.Peer, name, corr string, asJSON bool, stdout, stderr io.Writer) int {
	p, ok := peers[name]
	if !ok {
		fmt.Fprintf(stderr, "%s peers run: unknown peer %q\n", brand.CLI, name)
		return 1
	}
	corr = strings.TrimSpace(corr)
	if corr == "" {
		fmt.Fprintf(stderr, "%s peers run: correlation id is required\n", brand.CLI)
		return 2
	}
	arc := fetchPeerRun(p, corr)
	if asJSON {
		b, _ := json.MarshalIndent(arc, "", "  ")
		fmt.Fprintln(stdout, string(b))
		if arc.Error != "" {
			return 1
		}
		return 0
	}
	if arc.Error != "" {
		fmt.Fprintf(stdout, "  %-14s %s  UNREACHABLE: %s\n", arc.Peer, arc.URL, arc.Error)
		return 1
	}
	fmt.Fprintf(stdout, "peer %s run %s: %d event(s)\n", arc.Peer, arc.CorrelationID, arc.Count)
	for _, ev := range arc.Events {
		hash := ev.Hash
		if len(hash) > 12 {
			hash = hash[:12]
		}
		fmt.Fprintf(stdout, "  #%d %-22s %-28s %s\n", ev.Seq, ev.Kind, ev.Subject, hash)
	}
	return 0
}

func fetchPeerRun(p peer.Peer, corr string) peerRunArc {
	out := peerRunArc{Peer: p.Name, URL: p.URL, CorrelationID: corr}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	endpoint := strings.TrimRight(p.URL, "/") + "/api/v1/runs/" + url.PathEscape(corr)
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
		CorrelationID string         `json:"correlation_id"`
		Count         int            `json:"count"`
		Events        []peerRunEvent `json:"events"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxPeerRunResponseBytes)).Decode(&body); err != nil {
		out.Error = "bad run response: " + err.Error()
		return out
	}
	if body.CorrelationID != "" {
		out.CorrelationID = body.CorrelationID
	}
	out.Count = body.Count
	if out.Count == 0 {
		out.Count = len(body.Events)
	}
	out.Events = body.Events
	return out
}

type peerArtifactEntry struct {
	ID        string `json:"id"`
	Ref       string `json:"ref"`
	Name      string `json:"name,omitempty"`
	Mime      string `json:"mime,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Source    string `json:"source,omitempty"`
	Sender    string `json:"sender,omitempty"`
	Corr      string `json:"corr,omitempty"`
	Size      int64  `json:"size"`
	CreatedMs int64  `json:"created_ms"`
	Caption   string `json:"caption,omitempty"`
}

type peerArtifactList struct {
	Peer          string              `json:"peer"`
	URL           string              `json:"url"`
	CorrelationID string              `json:"correlation_id"`
	Count         int                 `json:"count"`
	TotalCount    int                 `json:"total_count,omitempty"`
	Truncated     bool                `json:"truncated,omitempty"`
	Entries       []peerArtifactEntry `json:"entries"`
	Error         string              `json:"error,omitempty"`
}

func peersArtifacts(peers map[string]peer.Peer, name, corr string, asJSON bool, stdout, stderr io.Writer) int {
	p, ok := peers[name]
	if !ok {
		fmt.Fprintf(stderr, "%s peers artifacts: unknown peer %q\n", brand.CLI, name)
		return 1
	}
	corr = strings.TrimSpace(corr)
	if corr == "" {
		fmt.Fprintf(stderr, "%s peers artifacts: correlation id is required\n", brand.CLI)
		return 2
	}
	list := fetchPeerArtifacts(p, corr)
	if asJSON {
		b, _ := json.MarshalIndent(list, "", "  ")
		fmt.Fprintln(stdout, string(b))
		if list.Error != "" {
			return 1
		}
		return 0
	}
	if list.Error != "" {
		fmt.Fprintf(stdout, "  %-14s %s  UNREACHABLE: %s\n", list.Peer, list.URL, list.Error)
		return 1
	}
	fmt.Fprintf(stdout, "peer %s artifacts for %s: %d artifact(s)", list.Peer, list.CorrelationID, list.Count)
	if list.Truncated {
		fmt.Fprintf(stdout, " (truncated from %d)", list.TotalCount)
	}
	fmt.Fprintln(stdout)
	for _, a := range list.Entries {
		ref := a.Ref
		if len(ref) > 12 {
			ref = ref[:12]
		}
		name := a.Name
		if name == "" {
			name = "(unnamed)"
		}
		kind := a.Kind
		if kind == "" {
			kind = "-"
		}
		fmt.Fprintf(stdout, "  %-22s %-12s %8d  %-24s %s\n", a.ID, kind, a.Size, name, ref)
	}
	return 0
}

func fetchPeerArtifacts(p peer.Peer, corr string) peerArtifactList {
	out := peerArtifactList{Peer: p.Name, URL: p.URL, CorrelationID: corr}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	endpoint := strings.TrimRight(p.URL, "/") + "/api/v1/artifacts"
	q := url.Values{}
	q.Set("corr", corr)
	q.Set("limit", fmt.Sprintf("%d", maxPeerArtifactEntries))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
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
		CorrelationID string              `json:"correlation_id"`
		Count         int                 `json:"count"`
		TotalCount    int                 `json:"total_count"`
		Truncated     bool                `json:"truncated"`
		Entries       []peerArtifactEntry `json:"entries"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxPeerArtifactResponseBytes)).Decode(&body); err != nil {
		out.Error = "bad artifacts response: " + err.Error()
		return out
	}
	out.Count = body.Count
	if out.Count == 0 {
		out.Count = len(body.Entries)
	}
	out.TotalCount = body.TotalCount
	if out.TotalCount == 0 {
		out.TotalCount = out.Count
	}
	out.Truncated = body.Truncated
	out.Entries = body.Entries
	return out
}

type peerArtifactBytes struct {
	Peer  string            `json:"peer"`
	URL   string            `json:"url"`
	ID    string            `json:"id"`
	Entry peerArtifactEntry `json:"entry,omitempty"`
	Size  int               `json:"size,omitempty"`
	Error string            `json:"error,omitempty"`
}

func peersArtifactGet(peers map[string]peer.Peer, name, id, outPath string, asJSON bool, stdout, stderr io.Writer) int {
	p, ok := peers[name]
	if !ok {
		fmt.Fprintf(stderr, "%s peers artifact-get: unknown peer %q\n", brand.CLI, name)
		return 1
	}
	id = strings.TrimSpace(id)
	outPath = strings.TrimSpace(outPath)
	if id == "" || outPath == "" {
		fmt.Fprintf(stderr, "%s peers artifact-get: artifact id and out_file are required\n", brand.CLI)
		return 2
	}
	meta, data := fetchPeerArtifactBytes(p, id)
	if meta.Error == "" {
		if err := os.WriteFile(outPath, data, 0o600); err != nil {
			meta.Error = "write " + outPath + ": " + err.Error()
		}
	}
	if asJSON {
		b, _ := json.MarshalIndent(meta, "", "  ")
		fmt.Fprintln(stdout, string(b))
		if meta.Error != "" {
			return 1
		}
		return 0
	}
	if meta.Error != "" {
		fmt.Fprintf(stdout, "  %-14s %s  UNREACHABLE: %s\n", meta.Peer, meta.URL, meta.Error)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %d byte(s) from peer %s artifact %s to %s\n", meta.Size, meta.Peer, meta.ID, outPath)
	return 0
}

func fetchPeerArtifactBytes(p peer.Peer, id string) (peerArtifactBytes, []byte) {
	out := peerArtifactBytes{Peer: p.Name, URL: p.URL, ID: id}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	endpoint := strings.TrimRight(p.URL, "/") + "/api/v1/artifacts/" + url.PathEscape(id) + "/bytes"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		out.Error = err.Error()
		return out, nil
	}
	if p.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		out.Error = err.Error()
		return out, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		out.Error = "401 (token rejected)"
		return out, nil
	}
	if resp.StatusCode == http.StatusForbidden {
		out.Error = "403 (artifact bytes disabled on peer)"
		return out, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out.Error = fmt.Sprintf("status %d", resp.StatusCode)
		return out, nil
	}
	var body struct {
		Entry peerArtifactEntry `json:"entry"`
		Size  int               `json:"size"`
		Data  string            `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxPeerArtifactBytesResponseBytes)).Decode(&body); err != nil {
		out.Error = "bad artifact bytes response: " + err.Error()
		return out, nil
	}
	data, err := base64.StdEncoding.DecodeString(body.Data)
	if err != nil {
		out.Error = "bad artifact bytes response: invalid base64: " + err.Error()
		return out, nil
	}
	out.Entry = body.Entry
	out.Size = len(data)
	if body.Size > 0 && body.Size != len(data) {
		out.Error = fmt.Sprintf("bad artifact bytes response: size %d does not match decoded %d", body.Size, len(data))
		return out, nil
	}
	return out, data
}
