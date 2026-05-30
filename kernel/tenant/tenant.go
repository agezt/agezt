// SPDX-License-Identifier: MIT

// Package tenant is the multi-tenant isolation foundation (ROADMAP P6-MULTI).
// A Registry manages a set of independent tenants under one root directory:
// each tenant gets its own base dir (and therefore its own journal, state,
// memory, world model, skills, vault, and schedules) and its own lazily-opened
// kernel. Tenant ids are validated as single, safe path segments so one
// tenant's state can never escape into or collide with another's — isolation by
// construction, not by convention.
//
// This package is intentionally decoupled from kernel/runtime: it opens tenants
// through an injected OpenFunc returning an io.Closer, so the registry's
// lifecycle logic is unit-testable without a provider, and the daemon supplies a
// real runtime.Open-backed factory. Wiring the control plane and APIs to route
// requests per tenant is a later phase; this is the storage/lifecycle core they
// build on.
package tenant

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// idPattern restricts a tenant id to a single safe path segment: lowercase
// alphanumerics, dashes, and underscores, 1–64 chars, starting alphanumeric. No
// dots or separators, so an id can neither traverse out of the root
// (".."/"a/b") nor collide with a sibling.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// ValidID reports whether id is a legal tenant identifier.
func ValidID(id string) bool { return idPattern.MatchString(id) }

// OpenFunc opens a tenant's kernel rooted at baseDir. The Registry calls it
// lazily the first time a tenant is acquired. Returning an io.Closer keeps this
// package independent of kernel/runtime (and easy to fake in tests); the daemon
// passes a func that calls runtime.Open and returns the *Kernel.
type OpenFunc func(id, baseDir string) (io.Closer, error)

// Tenant is one isolated tenant with its base dir and (once acquired) its open
// kernel.
type Tenant struct {
	ID          string
	BaseDir     string
	Kernel      io.Closer
	CreatedUnix int64
}

// Info is a listing snapshot: a tenant that exists on disk, and whether its
// kernel is currently loaded.
type Info struct {
	ID      string `json:"id"`
	BaseDir string `json:"base_dir"`
	Open    bool   `json:"open"`
}

// Registry manages isolated tenants under a shared root. It is safe for
// concurrent use.
type Registry struct {
	root string
	open OpenFunc
	mu   sync.Mutex
	live map[string]*Tenant // id -> acquired (kernel-open) tenant
}

// New creates a Registry rooted at root (created if missing). open is the
// factory used to lazily bring a tenant's kernel online.
func New(root string, open OpenFunc) (*Registry, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("tenant: root is required")
	}
	if open == nil {
		return nil, fmt.Errorf("tenant: open func is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("tenant: mkdir root %s: %w", root, err)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("tenant: resolve root: %w", err)
	}
	return &Registry{root: abs, open: open, live: map[string]*Tenant{}}, nil
}

// baseDir returns the validated, contained base dir for id. It double-checks
// (beyond the id regex) that the cleaned path is still directly under root.
func (r *Registry) baseDir(id string) (string, error) {
	if !ValidID(id) {
		return "", fmt.Errorf("tenant: invalid id %q (use [a-z0-9_-], 1-64 chars)", id)
	}
	dir := filepath.Join(r.root, id)
	if filepath.Dir(dir) != r.root {
		return "", fmt.Errorf("tenant: id %q escapes the tenant root", id)
	}
	return dir, nil
}

// Acquire returns the tenant with id, opening its kernel on first use (lazily)
// and creating its base dir if needed. It is idempotent: a second Acquire of the
// same id returns the already-open tenant without reopening.
func (r *Registry) Acquire(id string, now time.Time) (*Tenant, error) {
	dir, err := r.baseDir(id)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.live[id]; ok {
		return t, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("tenant: mkdir %s: %w", dir, err)
	}
	k, err := r.open(id, dir)
	if err != nil {
		return nil, fmt.Errorf("tenant %q: open: %w", id, err)
	}
	t := &Tenant{ID: id, BaseDir: dir, Kernel: k, CreatedUnix: now.Unix()}
	r.live[id] = t
	return t, nil
}

// Get returns the tenant with id if its kernel is currently loaded.
func (r *Registry) Get(id string) (*Tenant, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.live[id]
	return t, ok
}

// Exists reports whether a tenant directory exists on disk (whether or not its
// kernel is loaded).
func (r *Registry) Exists(id string) bool {
	dir, err := r.baseDir(id)
	if err != nil {
		return false
	}
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// List returns every tenant that exists on disk under root, marking which are
// currently open, sorted by id.
func (r *Registry) List() ([]Info, error) {
	entries, err := os.ReadDir(r.root)
	if err != nil {
		return nil, fmt.Errorf("tenant: read root: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Info
	for _, e := range entries {
		if !e.IsDir() || !ValidID(e.Name()) {
			continue
		}
		_, open := r.live[e.Name()]
		out = append(out, Info{ID: e.Name(), BaseDir: filepath.Join(r.root, e.Name()), Open: open})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Release closes a tenant's kernel and forgets it, leaving its on-disk state
// intact (a later Acquire reopens it). Returns whether it was loaded.
func (r *Registry) Release(id string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.live[id]
	if !ok {
		return false, nil
	}
	delete(r.live, id)
	if err := t.Kernel.Close(); err != nil {
		return true, fmt.Errorf("tenant %q: close: %w", id, err)
	}
	return true, nil
}

// Remove closes a tenant's kernel (if open) and deletes its base dir entirely.
// Destructive and irreversible — the tenant's journal, state, and vault are
// gone. Returns whether anything was removed.
func (r *Registry) Remove(id string) (bool, error) {
	dir, err := r.baseDir(id)
	if err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.live[id]; ok {
		delete(r.live, id)
		_ = t.Kernel.Close()
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return false, nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return false, fmt.Errorf("tenant %q: remove %s: %w", id, dir, err)
	}
	return true, nil
}

// CloseAll closes every loaded tenant kernel (leaving state on disk). It returns
// the first close error, after attempting all of them.
func (r *Registry) CloseAll() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var first error
	for id, t := range r.live {
		if err := t.Kernel.Close(); err != nil && first == nil {
			first = fmt.Errorf("tenant %q: close: %w", id, err)
		}
		delete(r.live, id)
	}
	return first
}

// Count returns the number of currently-open tenants.
func (r *Registry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.live)
}

// Root returns the registry's root directory.
func (r *Registry) Root() string { return r.root }
