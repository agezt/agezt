// SPDX-License-Identifier: MIT

// Package seatapi is the execution-seat catalog's contract: the row a seat
// is, and the port the host reads and edits it through.
//
// A seat is a task-facing preset — the isolation surface, the model chain and
// the optional tool allowlist a workboard task is dispatched under. It is pure
// data, so the row belongs to the contract layer and the module keeps its name
// as an alias of it: there is one seat and one spelling of it.
//
// The port is what the host holds. The kernel used to hold the module's
// concrete store and open it itself; the store is now the composition root's
// business (the daemon owns the file), and the host holds this port — so a
// host that was given nothing holds a nil port and must say "not available"
// rather than call into nothing.
package seatapi

// Seat is a named execution preset. Empty fields mean "inherit from the
// assigned agent / defaults" — a seat only overrides the axes it sets.
type Seat struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// ExecutionProfile selects the isolation surface (warden-family id:
	// local|warden|container). "" = inherit tool defaults.
	ExecutionProfile string `json:"execution_profile,omitempty"`
	// ModelChain, when set, forces this ordered model fallback chain for the run.
	ModelChain []string `json:"model_chain,omitempty"`
	// Tools is the allowlist applied only when RestrictTools is true (an empty
	// allowlist with RestrictTools = a pure-reasoning run).
	Tools         []string `json:"tools,omitempty"`
	RestrictTools bool     `json:"restrict_tools,omitempty"`
	Icon          string   `json:"icon,omitempty"`
	// Builtin marks a seeded, non-deletable seat. Custom operator seats are false.
	Builtin bool `json:"builtin,omitempty"`
}

// Store is the seat catalog the host holds.
//
// The five verbs are the ones the host actually uses: the seat list and the
// single-seat read (the dispatch path asks whether a seat is valid before it
// runs a task under it), the two edits (an operator creates and deletes a
// custom seat) and the validity check.
type Store interface {
	// List returns every seat in the catalog.
	List() []Seat
	// Get returns one seat by id; the bool is false when there is none.
	Get(id string) (Seat, bool)
	// Valid reports whether id names a seat that may be dispatched under. An
	// empty id means "no seat at all" and is valid — a task that asks for no
	// seat is not asking for an unknown one.
	Valid(id string) bool
	// Create adds a custom seat; the store validates and normalises it.
	Create(spec Seat) (Seat, error)
	// Delete removes a custom seat; the store refuses built-ins.
	Delete(id string) error
}
