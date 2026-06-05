// SPDX-License-Identifier: MIT

// Package standing implements the Chronos standing-order model and store
// (SPEC-16 §4). A standing order is a persistent goal: a named, pausable rule
// that — on one of its triggers — evaluates observers within a scope and acts
// within an initiative ceiling, then briefs. This package owns the durable
// record and its lifecycle (CRUD); the runner that fires triggers and drives the
// observe→salience→initiative→briefing pipeline is layered on top.
//
// The SPEC sketches the order as YAML authored in Flow Studio, but Agezt is
// stdlib-first (no YAML dependency, DECISIONS B0c), so the on-disk and wire form
// is JSON — the same declarative shape, a different serialisation.
package standing

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/ulid"
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

func validMode(m InitiativeMode) bool {
	switch m {
	case InitiativeInformOnly, InitiativeAsk, InitiativeActOrAsk:
		return true
	}
	return false
}

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
	CreatedMS     int64      `json:"created_ms"`
	UpdatedMS     int64      `json:"updated_ms"`
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
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("standing: mkdir %s: %w", dir, err)
	}
	s := &Store{path: filepath.Join(dir, "standing.json"), now: time.Now}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("standing: read %s: %w", s.path, err)
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.orders); err != nil {
			return nil, fmt.Errorf("standing: parse %s: %w", s.path, err)
		}
	}
	return s, nil
}

// Add validates and persists a new enabled order, assigning an id + timestamps.
// The caller-supplied o.ID/Enabled/timestamps are ignored (kernel-assigned).
func (s *Store) Add(o Order) (Order, error) {
	if err := Validate(o); err != nil {
		return Order{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UnixMilli()
	o.ID = ulid.New()
	o.Enabled = true
	o.CreatedMS = now
	o.UpdatedMS = now
	cp := o
	s.orders = append(s.orders, &cp)
	if err := s.save(); err != nil {
		s.orders = s.orders[:len(s.orders)-1]
		return Order{}, err
	}
	return cp, nil
}

// SetEnabled pauses (false) or resumes (true) an order. Returns the new state
// and whether the id existed.
func (s *Store) SetEnabled(id string, enabled bool) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, o := range s.orders {
		if o.ID == id {
			// Roll back the in-memory mutation if the durable write fails, so the
			// running view never diverges from disk on a transient save error.
			prevEnabled, prevUpdated := o.Enabled, o.UpdatedMS
			o.Enabled = enabled
			o.UpdatedMS = s.now().UnixMilli()
			if err := s.save(); err != nil {
				o.Enabled, o.UpdatedMS = prevEnabled, prevUpdated
				return Order{}, err
			}
			return *o, nil
		}
	}
	return Order{}, ErrNotFound
}

// Remove deletes an order. Returns whether it existed.
func (s *Store) Remove(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, o := range s.orders {
		if o.ID == id {
			removed := s.orders
			s.orders = append(append([]*Order{}, s.orders[:i]...), s.orders[i+1:]...)
			if err := s.save(); err != nil {
				s.orders = removed // restore: disk write failed, keep the order
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

// Get returns one order by id.
func (s *Store) Get(id string) (Order, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, o := range s.orders {
		if o.ID == id {
			return *o, true
		}
	}
	return Order{}, false
}

// List returns all orders, sorted by creation time then id (deterministic).
func (s *Store) List() []Order {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Order, 0, len(s.orders))
	for _, o := range s.orders {
		out = append(out, *o)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedMS != out[j].CreatedMS {
			return out[i].CreatedMS < out[j].CreatedMS
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Count returns the number of orders.
func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.orders)
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.orders, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
