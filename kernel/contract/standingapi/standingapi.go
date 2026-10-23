// SPDX-License-Identifier: MIT

// Package standingapi is the contract the standing-order model and the
// surfaces that read it agree on.
//
// A standing order is an autonomous agent the operator leaves switched on:
// "when this event fires, do this". The row lives in the contract layer
// because more than one layer has to name it — the store that persists it, the
// control plane that edits it, the runner that fires it, and the PLUGINS that
// report on it (the introspector lists the daemon's standing orders; the
// guardians reconcile them). A plugin may reach the contract and the platform
// layers and nothing else, so a row a plugin has to name cannot live in a
// module's implementation package.
//
// The rules of the road:
//
//   - Rows, constants and pure functions only. No store, no file, no clock.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package standingapi

import (
	"errors"
	"fmt"
	"strings"
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

func validMode(m InitiativeMode) bool {
	switch m {
	case InitiativeInformOnly, InitiativeAsk, InitiativeActOrAsk:
		return true
	}
	return false
}

// MaxAutonomyTrust maps an initiative mode to the trust ceiling (Lx) that ENFORCES
// it on a firing (M999). The mode is the operator's autonomy dial; until now it was
// stored but never applied (only max_trust gated). The mapping:
//
//   - inform_only → "L0" (LevelDeny): no tools at all — the agent may reason and
//     brief from the trigger context, but cannot act. "Notice and tell me."
//   - ask         → "L1" (LevelAsk): every effectful call needs approval, so an
//     unattended firing can't silently change anything.
//   - act_or_ask / "" → ("", false): no extra cap — the order's own max_trust wins.
//     Empty mode preserves pre-M999 behavior (backward compatible).
//
// The fire path combines this with the order's max_trust by taking the MORE
// restrictive (lower) level. Pure, so it's testable without importing edict.
func (m InitiativeMode) MaxAutonomyTrust() (level string, capped bool) {
	switch m {
	case InitiativeInformOnly:
		return "L0", true
	case InitiativeAsk:
		return "L1", true
	default: // act_or_ask, "", or anything unrecognized → no extra cap
		return "", false
	}
}
