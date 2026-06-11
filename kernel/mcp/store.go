// SPDX-License-Identifier: MIT

// Package mcp is governed runtime self-install for MCP servers (M796): a
// durable registry of Model Context Protocol servers plus a minimal MCP
// client, so an agent (or operator) can ADD a server and ATTACH it while the
// daemon runs — no restart, no separate bridge binary, no env-var surgery.
// An attached server's tools are offered to every run as mcp_<server>_<tool>
// through the same dynamic per-run merge seam forged script tools use.
//
// Governance is the point: registering/attaching is gated by the
// `mcp.install` Edict capability (Ask by default — attaching spawns an
// arbitrary process), every forwarded call exercises `mcp.call`, the child
// gets a SCRUBBED environment (no AGEZT_* / secret-shaped vars), frames are
// size-capped, and every lifecycle transition is journaled (mcp.*) so
// `agt why` can explain how a server came to be attached. Detach is the
// instant kill switch.
//
// Storage mirrors kernel/roster: a single JSON file rewritten atomically.
package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/ulid"
)

// ErrNotFound is returned for an unknown server id/name.
var ErrNotFound = errors.New("mcp: server not found")

// Server is one registered MCP server: a stdio command the daemon can spawn
// and speak MCP to. Name is the immutable handle and the tool-name prefix
// segment (mcp_<name>_<tool>) — no underscores, so the Edict toolmap can
// parse the server back out of a tool name unambiguously.
type Server struct {
	ID string `json:"id"`
	// Name is the server's immutable handle: lowercase letters/digits only
	// (it becomes a tool-name segment, parsed by prefix).
	Name string `json:"name"`
	// Command + Args spawn the server process (stdio transport), e.g.
	// "npx" ["-y","@modelcontextprotocol/server-everything"].
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	// Enabled means "attach automatically when the daemon starts". A live
	// attach/detach is a runtime operation independent of this flag.
	Enabled     bool   `json:"enabled"`
	Description string `json:"description,omitempty"`
	// Env is an OPT-IN set of environment variables injected into THIS server's
	// process on attach (M898) — e.g. {"GITHUB_PERSONAL_ACCESS_TOKEN": "..."}.
	// The base environment stays scrubbed (the daemon's own AGEZT_*/secret vars
	// never leak); only these explicit, operator-supplied entries are added, so a
	// credentialed server (github/brave/slack) can get exactly the key it needs.
	// Stored like Args (plaintext in the registry) — use a dedicated low-scope
	// token. Redacted out of read APIs.
	Env map[string]string `json:"env,omitempty"`
	// ToolAllow is an OPT-IN allowlist of the server's own tool names to expose
	// to runs (M899) — context-efficient MCP management. A chatty server (github
	// exposes ~30 tools) can be trimmed to the few a run actually needs, so its
	// schemas don't bloat every run's context. Empty = expose all the server's
	// tools (the default). Names are the server's bare tool names (not the
	// mcp_<name>_<tool> prefix).
	ToolAllow []string `json:"tool_allow,omitempty"`
	CreatedMS int64    `json:"created_ms"`
	UpdatedMS int64    `json:"updated_ms"`
}

// nameRe: lowercase letter first, then letters/digits. Deliberately NO
// underscore/dash — the name is parsed out of mcp_<name>_<tool> by the
// Edict toolmap, so it must never contain the separator.
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9]{0,15}$`)

// envKeyRe: a POSIX-ish environment variable name (letters/digits/underscore,
// not starting with a digit). Keeps a malformed key from reaching exec.
var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const maxArgs = 32

// maxEnv caps how many env vars one server may carry (a sanity bound, not a
// security boundary).
const maxEnv = 32

// maxToolAllow caps the per-server tool allowlist length.
const maxToolAllow = 128

// Validate checks a server's user-supplied fields.
func Validate(s Server) error {
	if !nameRe.MatchString(s.Name) {
		return fmt.Errorf("mcp: name must match %s (it becomes the mcp_<name>_* tool prefix)", nameRe)
	}
	if strings.TrimSpace(s.Command) == "" {
		return errors.New("mcp: command is required")
	}
	if len(s.Args) > maxArgs {
		return fmt.Errorf("mcp: at most %d args", maxArgs)
	}
	for _, a := range s.Args {
		if strings.TrimSpace(a) == "" {
			return errors.New("mcp: empty arg")
		}
	}
	if len(s.Env) > maxEnv {
		return fmt.Errorf("mcp: at most %d env vars", maxEnv)
	}
	for k := range s.Env {
		if !envKeyRe.MatchString(k) {
			return fmt.Errorf("mcp: invalid env var name %q", k)
		}
	}
	if len(s.ToolAllow) > maxToolAllow {
		return fmt.Errorf("mcp: at most %d allowed tools", maxToolAllow)
	}
	for _, t := range s.ToolAllow {
		if strings.TrimSpace(t) == "" {
			return errors.New("mcp: empty tool name in allowlist")
		}
	}
	return nil
}

// Store is the persistent MCP-server registry, a single JSON file rewritten
// atomically on change. Safe for concurrent use. Mirrors kernel/roster.Store.
type Store struct {
	path    string
	mu      sync.Mutex
	now     func() time.Time
	servers []*Server
}

// OpenStore opens (or creates) the registry under dir.
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mcp: mkdir %s: %w", dir, err)
	}
	s := &Store{path: filepath.Join(dir, "servers.json"), now: time.Now}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("mcp: read %s: %w", s.path, err)
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.servers); err != nil {
			return nil, fmt.Errorf("mcp: parse %s: %w", s.path, err)
		}
	}
	return s, nil
}

// Add validates and persists a new server (enabled by default — it will
// auto-attach on the next daemon start; a LIVE attach is a separate, gated
// operation). Caller-supplied ID/timestamps are ignored.
func (s *Store) Add(srv Server) (Server, error) {
	srv.Name = strings.TrimSpace(srv.Name)
	srv.Command = strings.TrimSpace(srv.Command)
	if err := Validate(srv); err != nil {
		return Server{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ex := range s.servers {
		if ex.Name == srv.Name {
			return Server{}, fmt.Errorf("mcp: name %q already exists", srv.Name)
		}
	}
	now := s.now().UnixMilli()
	srv.ID = ulid.New()
	srv.Enabled = true
	srv.CreatedMS = now
	srv.UpdatedMS = now
	cp := srv
	s.servers = append(s.servers, &cp)
	if err := s.save(); err != nil {
		s.servers = s.servers[:len(s.servers)-1]
		return Server{}, err
	}
	return cp, nil
}

// SetEnabled flips auto-attach-at-start for a server by id or name.
func (s *Store) SetEnabled(ref string, enabled bool) (Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	srv := s.find(ref)
	if srv == nil {
		return Server{}, ErrNotFound
	}
	prevEnabled, prevUpdated := srv.Enabled, srv.UpdatedMS
	srv.Enabled = enabled
	srv.UpdatedMS = s.now().UnixMilli()
	if err := s.save(); err != nil {
		srv.Enabled, srv.UpdatedMS = prevEnabled, prevUpdated
		return Server{}, err
	}
	return *srv, nil
}

// Remove deletes a server by id or name. Returns whether it existed.
func (s *Store) Remove(ref string) (Server, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, srv := range s.servers {
		if srv.ID == ref || srv.Name == ref {
			removed := s.servers
			gone := *srv
			s.servers = append(append([]*Server{}, s.servers[:i]...), s.servers[i+1:]...)
			if err := s.save(); err != nil {
				s.servers = removed
				return Server{}, false, err
			}
			return gone, true, nil
		}
	}
	return Server{}, false, nil
}

// Get returns one server by id or name.
func (s *Store) Get(ref string) (Server, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if srv := s.find(ref); srv != nil {
		return *srv, true
	}
	return Server{}, false
}

// find returns the live pointer for an id or name. Caller holds s.mu.
func (s *Store) find(ref string) *Server {
	for _, srv := range s.servers {
		if srv.ID == ref || srv.Name == ref {
			return srv
		}
	}
	return nil
}

// List returns all servers, sorted by creation time then id.
func (s *Store) List() []Server {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Server, 0, len(s.servers))
	for _, srv := range s.servers {
		out = append(out, *srv)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedMS != out[j].CreatedMS {
			return out[i].CreatedMS < out[j].CreatedMS
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Count returns the number of registered servers.
func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.servers)
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.servers, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
