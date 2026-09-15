// SPDX-License-Identifier: MIT

// Package standing implements the standing-order model and store
// (SPEC-16 §4). This file holds the declarations: TriggerType /
// Trigger / InitiativeMode / Initiative / Order / Store types + consts
// + ErrNotFound + Validate. Split from standing.go during Day 211
// god-file refactor (#47). Public API unchanged.
package standing

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)


// TriggerType enumerates what can activate a standing order's evaluation.
type TriggerType string

const (
	// TriggerCron fires on a schedule (Schedule is a cron/interval spec).
	TriggerCron TriggerType = "cron"
	// TriggerEvent fires when a journal event matching Subject is published.
	TriggerEvent TriggerType = "event"
)

// Trigger is one activation condition. Exactly one of Schedule (cron) or Subject
// (event) is meaningful, per Type.
type Trigger struct {
	Type     TriggerType `json:"type"`
	Schedule string      `json:"schedule,omitempty"` // for cron
	Subject  string      `json:"subject,omitempty"`  // for event (subject glob)
}

// InitiativeMode is how autonomous the order may be (SPEC-03 §9 / SPEC-16 §4).
type InitiativeMode string

const (
	InitiativeInformOnly InitiativeMode = "inform_only"
	InitiativeAsk        InitiativeMode = "ask"
	InitiativeActOrAsk   InitiativeMode = "act_or_ask"
)


// Initiative bounds autonomous action within an order.
type Initiative struct {
	Mode           InitiativeMode `json:"mode"`
	MaxTrust       string         `json:"max_trust,omitempty"`         // L0..L4 ceiling
	BudgetPerRunMc int64          `json:"budget_per_run_mc,omitempty"` // per-run cost cap (microcents)
}

// Order is one standing order (SPEC-16 §4).
type Order struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Enabled       bool       `json:"enabled"`
	Triggers      []Trigger  `json:"triggers"`
	Observers     []string   `json:"observers,omitempty"`
	ScopeEntities []string   `json:"scope_entities,omitempty"`
	Initiative    Initiative `json:"initiative"`
	BriefingMin   string     `json:"briefing_disposition_min,omitempty"` // drop|digest|notify|alert
	BriefingChan  string     `json:"briefing_channel,omitempty"`
	Plan          string     `json:"plan,omitempty"` // optional explicit plan/intent template
	// Agent, when set, makes each firing run AS that named roster agent
	// (M790): its soul, model fallback chain, and memory scope apply, and its
	// per-run cost ceiling is the default when the order sets none. Resolved
	// at fire time — an unknown or paused agent journals a standing.error
	// instead of silently running as the default identity.
	Agent string `json:"agent,omitempty"`
	// Assure, when > 0, makes each firing "do-it-for-sure": the order's plan runs,
	// a verifier checks it was actually accomplished, and it retries the gap up to
	// this many attempts (M655). 0 = a single pass (the default). The fire path
	// (cmd/agezt) reads this to choose RunAssured vs RunWith — symmetric with the
	// schedule tool's assure budget for the time axis.
	Assure int `json:"assure,omitempty"`
	// CooldownSec, when > 0, overrides the event-runner default cooldown for this
	// order. This keeps noisy event-triggered orders from spawning LLM work on
	// every event burst while still allowing quiet orders to use the global default.
	CooldownSec int64 `json:"cooldown_sec,omitempty"`
	CreatedMS   int64 `json:"created_ms"`
	UpdatedMS   int64 `json:"updated_ms"`
}

// ErrNotFound is returned for operations on an unknown order id.
var ErrNotFound = errors.New("standing: order not found")

// Validate checks an order is well-formed enough to persist (SPEC-16 §4). Pure,
// so the CLI/control plane and tests share one definition of "valid".
func Validate(o Order) error {
	if strings.TrimSpace(o.Name) == "" {
		return errors.New("standing: name is required")
	}
	if len(o.Triggers) == 0 {
		return errors.New("standing: at least one trigger is required")
	}
	for i, t := range o.Triggers {
		switch t.Type {
		case TriggerCron:
			if strings.TrimSpace(t.Schedule) == "" {
				return fmt.Errorf("standing: trigger %d (cron) needs a schedule", i)
			}
		case TriggerEvent:
			if strings.TrimSpace(t.Subject) == "" {
				return fmt.Errorf("standing: trigger %d (event) needs a subject", i)
			}
		default:
			return fmt.Errorf("standing: trigger %d has unknown type %q", i, t.Type)
		}
	}
	if o.Initiative.Mode != "" && !validMode(o.Initiative.Mode) {
		return fmt.Errorf("standing: unknown initiative mode %q", o.Initiative.Mode)
	}
	if o.CooldownSec < 0 {
		return errors.New("standing: cooldown_sec must be non-negative")
	}
	return nil
}

// Store is the persistent set of standing orders, a single JSON file rewritten
// atomically on change. Safe for concurrent use. Mirrors kernel/cadence.Store.
type Store struct {
	path   string
	mu     sync.Mutex
	now    func() time.Time
	orders []*Order
}

// Open opens (or creates) the standing-order store under dir.
