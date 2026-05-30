// SPDX-License-Identifier: MIT

package governor

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"

	"github.com/agezt/agezt/kernel/agent"
)

// AuthMode summarises how a provider authenticates (DECISIONS C2: routing
// honours subscription-first when available).
type AuthMode string

const (
	AuthSubscription AuthMode = "subscription"
	AuthAPIKey       AuthMode = "api-key"
	AuthLocal        AuthMode = "local"
)

// ProviderInfo describes one registered provider.
type ProviderInfo struct {
	// Name is the registry key; must match Provider.Name() for sanity.
	Name string
	// Provider is the actual implementation.
	Provider agent.Provider
	// AuthMode classifies the provider; routing prefers Subscription /
	// Local over API-key when both can serve a request.
	AuthMode AuthMode
	// IsFallback hints that this provider is suitable as a last-resort
	// floor (typically a local model). True → always tried last.
	IsFallback bool
	// Models is the set of model ids this provider serves (from the catalog
	// entry). Used for per-request model routing: a request naming a model is
	// routed to the provider that serves it. Empty means "unknown" — the
	// provider is never hoisted by model, but still serves as a default/
	// fallback in registration order.
	Models []string
}

// Serves reports whether this provider lists the given model id.
func (p *ProviderInfo) Serves(model string) bool {
	return model != "" && slices.Contains(p.Models, model)
}

// Registry holds all known providers in insertion order. Safe for
// concurrent use.
type Registry struct {
	mu     sync.RWMutex
	byName map[string]*ProviderInfo
	order  []string // insertion order
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{byName: map[string]*ProviderInfo{}}
}

// ErrAlreadyRegistered is returned by Register for a duplicate name.
var ErrAlreadyRegistered = errors.New("governor: provider already registered")

// Register adds a provider to the registry. The Provider's Name() must
// match info.Name for sanity. Re-registering an existing name errors.
func (r *Registry) Register(info *ProviderInfo) error {
	if info == nil || info.Provider == nil {
		return errors.New("governor: nil provider info")
	}
	if info.Name == "" {
		return errors.New("governor: name required")
	}
	if got := info.Provider.Name(); got != info.Name {
		return fmt.Errorf("governor: name mismatch (info=%q provider=%q)", info.Name, got)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byName[info.Name]; dup {
		return fmt.Errorf("%w: %s", ErrAlreadyRegistered, info.Name)
	}
	r.byName[info.Name] = info
	r.order = append(r.order, info.Name)
	return nil
}

// Replace atomically swaps the entry for info.Name. Unlike Register,
// it succeeds whether the name was previously registered or not —
// useful for hot reload paths that re-establish the primary provider
// when catalog/vault state changes. Insertion order is preserved when
// replacing an existing entry; new entries append at the end.
//
// The Provider's Name() must match info.Name (same sanity check
// Register applies).
func (r *Registry) Replace(info *ProviderInfo) error {
	if info == nil || info.Provider == nil {
		return errors.New("governor: nil provider info")
	}
	if info.Name == "" {
		return errors.New("governor: name required")
	}
	if got := info.Provider.Name(); got != info.Name {
		return fmt.Errorf("governor: name mismatch (info=%q provider=%q)", info.Name, got)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, existed := r.byName[info.Name]; !existed {
		r.order = append(r.order, info.Name)
	}
	r.byName[info.Name] = info
	return nil
}

// Remove deletes the named provider from the registry. Returns
// (true, nil) if removed, (false, nil) if absent. Insertion order is
// preserved for the remaining entries.
func (r *Registry) Remove(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byName[name]; !ok {
		return false
	}
	delete(r.byName, name)
	for i, n := range r.order {
		if n == name {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	return true
}

// Get returns the named provider, or (nil, false) if absent.
func (r *Registry) Get(name string) (*ProviderInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.byName[name]
	return info, ok
}

// All returns every registered provider in insertion order.
func (r *Registry) All() []*ProviderInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*ProviderInfo, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.byName[n])
	}
	return out
}

// Names returns just the registered names, alphabetically sorted (useful
// for stable display).
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.order))
	for n := range r.byName {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
