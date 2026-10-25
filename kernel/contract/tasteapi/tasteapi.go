// SPDX-License-Identifier: MIT

// Package tasteapi is the contract layer's home for the taste rows: the
// curated "what good looks like" exemplar, the input that authors one, the
// filter that reads them back, and the port through which the exemplar
// library is read and curated.
//
// The rows live here because more than one layer has to speak them and none
// of them may name the module's implementation: the kernel selects the
// exemplars that apply to a run, the app layer curates them, and the daemon
// owns the file they live in. The implementation — the file-backed store —
// stays in the module (kernel/modules/knowledge/taste); the module keeps the
// names its own callers use as aliases, so there is one row and one spelling
// of it.
//
// The rules of the road:
//
//   - Rows, constants and pure functions only. No store, no file, no clock.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package tasteapi

import "errors"

// ErrNotFound is returned when an operation names an exemplar that does not
// exist.
//
// It lives in the contract layer because more than one layer has to NAME it:
// the store that answers it and the app layer that reports it back to the
// caller. A module may not be named by the layers above it, so the error the
// layers above it compare against must not live in the module (W3.52).
var ErrNotFound = errors.New("taste: not found")

// Exemplar is one "what good looks like" anchor. Body is the example itself
// (a sample answer, snippet, or artifact); Scope narrows where it applies: an
// empty Scope means every run, otherwise it matches a run's agent slug (or a
// free-form tag the operator dispatches under).
type Exemplar struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Scope     string   `json:"scope,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	CreatedMS int64    `json:"created_ms"`
	UpdatedMS int64    `json:"updated_ms"`
}

// CreateSpec is the input to Create.
type CreateSpec struct {
	Title string
	Body  string
	Scope string
	Tags  []string
}

// Filter narrows a listing. An empty Scope or Tag matches everything; Limit
// caps the result (zero means no cap).
type Filter struct {
	Scope string
	Tag   string
	Limit int
}

// Store is the port the daemon's exemplar library implements: the curated set
// of exemplars, one JSON file rewritten atomically on change.
//
// The port lives in the contract layer because three layers have to speak it
// and none of them may name the module's implementation:
//
//   - the kernel holds it (cfg.TasteStore) and selects the exemplars that
//     apply to a run before the model acts;
//   - the app layer's service holds it and curates it (list/create/delete);
//   - the control plane reaches it through the kernel's accessor.
//
// The daemon opens the file and hands the kernel this port; a host that was
// given none holds a nil port and injects no exemplars rather than calling
// into nothing.
//
// The port is the module's whole public surface — five verbs, no more. A
// wider surface would let a caller reach past the library's own rules (the
// store validates, assigns ids and timestamps); a narrower one would force a
// second shape to be kept in step with this one.
type Store interface {
	// Create validates and persists a new exemplar, assigning its id and
	// timestamps. The caller supplies the clock (nowMS) so the port stays
	// free of one.
	Create(spec CreateSpec, nowMS int64) (Exemplar, error)
	// Get returns one exemplar by id; the bool says whether it was there.
	Get(id string) (Exemplar, bool)
	// List returns the exemplars matching the filter, newest first.
	List(f Filter) []Exemplar
	// ForScope returns the exemplars that apply to a run with the given
	// scope: every global exemplar (empty Scope) plus those whose Scope
	// matches, scoped ones first, newest first, capped at limit.
	ForScope(scope string, limit int) []Exemplar
	// Delete removes an exemplar by id. ErrNotFound for an unknown id.
	Delete(id string) error
}
