// SPDX-License-Identifier: MIT

// Package nodes owns the node registry: the local daemon plus the configured
// AGEZT_PEERS mesh nodes, each probed for token-redacted reachability. The peer
// list's parser is shared with the remote run mirror.
package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

const healthResponseLimit = 1 << 20

// Peer is one configured mesh node. Its token authenticates the probe and is
// never reported.
type Peer struct {
	Name  string
	URL   string
	Token string
}

// ParsePeers reads a comma-separated `name=url|token` list. Blank entries are
// skipped; names must be unique and URLs http(s) with a host, kept without a
// trailing slash.
func ParsePeers(spec string) ([]Peer, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	seen := map[string]bool{}
	var out []Peer
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
		out = append(out, Peer{Name: name, URL: strings.TrimRight(urlStr, "/"), Token: token})
	}
	return out, nil
}

// Ports reads the kernel's model, whether a remote_run tool is present and
// non-nil, and the configured peer list, and gives the HTTP client the probes
// use.
type Ports struct {
	Model     func() string
	RemoteRun func() (present, registered bool)
	PeerSpec  func() string
	Client    func() *http.Client
}

type Service struct{ ports Ports }

func New(ports Ports) *Service { return &Service{ports: ports} }

type RegistryRequest struct{}

// Node is one registry row. The local row carries the model and capabilities;
// a peer row carries its URL and auth mode, and its version and model count
// once its health answered.
type Node struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Local        bool     `json:"local"`
	URL          string   `json:"url,omitempty"`
	Auth         string   `json:"auth,omitempty"`
	Reachable    bool     `json:"reachable"`
	Status       string   `json:"status"`
	Version      *string  `json:"version,omitempty"`
	Model        *string  `json:"model,omitempty"`
	ModelCount   *int     `json:"model_count,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Error        string   `json:"error,omitempty"`
}

// RegistryOutput lists the nodes. An unparsable peer list reports its error
// with only the local node and no remote_run_registered flag.
type RegistryOutput struct {
	Nodes               []Node `json:"nodes"`
	Count               int    `json:"count"`
	PeerCount           int    `json:"peer_count"`
	RemoteRunRegistered *bool  `json:"remote_run_registered,omitempty"`
	Error               string `json:"error,omitempty"`
}

// Registry lists the local daemon, then each configured peer in name order,
// probed one at a time.
func (s *Service) Registry(_ context.Context, _ RegistryRequest) (RegistryOutput, error) {
	version, model := brand.Version, s.ports.Model()
	local := Node{ID: "local", Name: "local", Local: true, Reachable: true, Status: "ok", Version: &version, Model: &model, Capabilities: []string{
		"controlplane",
		"webui",
		"agent-runtime",
	}}
	if present, _ := s.ports.RemoteRun(); present {
		local.Capabilities = append(local.Capabilities, "remote-run")
	}
	nodes := []Node{local}
	peers, err := ParsePeers(s.ports.PeerSpec())
	if err != nil {
		return RegistryOutput{Nodes: nodes, Count: len(nodes), PeerCount: 0, Error: err.Error()}, nil
	}
	sort.Slice(peers, func(i, j int) bool { return peers[i].Name < peers[j].Name })
	for _, p := range peers {
		nodes = append(nodes, probe(s.ports.Client(), p))
	}
	_, registered := s.ports.RemoteRun()
	return RegistryOutput{Nodes: nodes, Count: len(nodes), PeerCount: len(peers), RemoteRunRegistered: &registered}, nil
}

// probe asks a peer's /api/v1/health within three seconds, independent of the
// request.
func probe(client *http.Client, p Peer) Node {
	row := Node{ID: "peer:" + p.Name, Name: p.Name, Local: false, URL: p.URL, Auth: "none", Reachable: false, Status: "unreachable"}
	if p.Token != "" {
		row.Auth = "token"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/api/v1/health", nil)
	if err != nil {
		row.Error = err.Error()
		return row
	}
	if p.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	resp, err := client.Do(req)
	if err != nil {
		row.Error = err.Error()
		return row
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		row.Error = "401 (token rejected)"
		return row
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		row.Error = fmt.Sprintf("status %d", resp.StatusCode)
		return row
	}
	var body struct {
		Status     string `json:"status"`
		Version    string `json:"version"`
		ModelCount int    `json:"model_count"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, healthResponseLimit)).Decode(&body); err != nil {
		row.Error = "bad health response: " + err.Error()
		return row
	}
	row.Status = body.Status
	row.Reachable = body.Status == "ok"
	row.Version = &body.Version
	row.ModelCount = &body.ModelCount
	if body.Status != "ok" {
		row.Error = "status=" + body.Status
	}
	return row
}

// Operations declares the read-only, operator-only registry on its Web UI
// route.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("nodes provider required")
	}
	out, err := schema.FromType(reflect.TypeFor[RegistryOutput](), false)
	if err != nil {
		return nil, err
	}
	op, err := app.NewOperation(opapi.Spec{Name: "node_registry", ReadOnly: true, OutputSchema: out, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "GET", Path: "/api/nodes"}}, func(ctx context.Context, in RegistryRequest) (RegistryOutput, error) {
		return provider(ctx).Registry(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
