// SPDX-License-Identifier: MIT

package mcp

// Package documentation lives in doc.go.

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/platform/filestore"
	"github.com/agezt/agezt/kernel/ulid"
)

// ErrNotFound is returned for an unknown server id/name.
var ErrNotFound = errors.New("mcp: server not found")

// Server is one registered MCP server.
//
// The row itself lives in the contract layer (kernel/contract/mcpapi): the
// tools and the marketplace that name it are plugins, and a plugin may only
// reach the contract. This package keeps the name, so every existing caller is
// unchanged. The registry — the store that persists these rows — stays here.
type Server = mcpapi.Server

// The caps are the contract's too, so the rule that checks a server's fields
// and the bounds it checks them against live in one place.
const (
	maxArgs      = mcpapi.MaxArgs
	maxEnv       = mcpapi.MaxEnv
	maxHeaders   = mcpapi.MaxHeaders
	maxToolAllow = mcpapi.MaxToolAllow
)

// Validate checks a server's user-supplied fields. The rule travels with the
// row; the module keeps the name.
func Validate(s Server) error { return mcpapi.Validate(s) }

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
	s := &Store{now: time.Now}
	path, err := filestore.LoadFrom(dir, "servers.json", &s.servers)
	if err != nil {
		return nil, fmt.Errorf("mcp: %w", err)
	}
	s.path = path
	return s, nil
}

// Add validates and persists a new server (enabled by default — it will
// auto-attach on the next daemon start; a LIVE attach is a separate, gated
// operation). Caller-supplied ID/timestamps are ignored.
func (s *Store) Add(srv Server) (Server, error) {
	srv.Name = strings.TrimSpace(srv.Name)
	srv.Command = strings.TrimSpace(srv.Command)
	srv.URL = strings.TrimSpace(srv.URL)
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
	return filestore.Save(s.path, s.servers)
}
