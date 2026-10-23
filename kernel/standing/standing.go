// SPDX-License-Identifier: MIT

package standing

// Provenance: Package standing implements the standing-order model and store
//             (SPEC-16 §4). This file holds the declarations: TriggerType / Trigger
//             / InitiativeMode / Initiative / Order / Store types + consts +
//             ErrNotFound + Validate. Split from standing.go during Day 211 god-file
//             refactor (#47). Public API unchanged.
//
// The ROWS live in the contract layer (kernel/contract/standingapi): the
// introspector and the guardians are PLUGINS, and a plugin may reach the
// contract and the platform layers and nothing else — so a row a plugin has to
// name cannot live in this package. The names below stay as aliases, so every
// existing caller is unchanged.

import (
	"errors"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/contract/standingapi"
)

// The standing-order rows and their constants — the contract's, kept here as
// aliases so the module's public names do not move.
type (
	TriggerType    = standingapi.TriggerType
	Trigger        = standingapi.Trigger
	InitiativeMode = standingapi.InitiativeMode
	Initiative     = standingapi.Initiative
	Order          = standingapi.Order
)

const (
	TriggerCron  = standingapi.TriggerCron
	TriggerEvent = standingapi.TriggerEvent

	InitiativeInformOnly = standingapi.InitiativeInformOnly
	InitiativeAsk        = standingapi.InitiativeAsk
	InitiativeActOrAsk   = standingapi.InitiativeActOrAsk
)

// Validate checks an order is well-formed enough to persist (SPEC-16 §4). Pure,
// so the CLI/control plane and tests share one definition of "valid".
func Validate(o Order) error { return standingapi.Validate(o) }

// ErrNotFound is returned for operations on an unknown order id.
var ErrNotFound = errors.New("standing: order not found")

// Store is the persistent set of standing orders, a single JSON file rewritten
// atomically on change. Safe for concurrent use. Mirrors kernel/cadence.Store.
type Store struct {
	path   string
	mu     sync.Mutex
	now    func() time.Time
	orders []*Order
}

// Open opens (or creates) the standing-order store under dir.
