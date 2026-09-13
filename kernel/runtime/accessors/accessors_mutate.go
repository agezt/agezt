// SPDX-License-Identifier: MIT
//
// Accessor mutators: SetMarket + the standing-order mutators
// (AddStanding + SetStandingEnabled + UpdateStanding + RemoveStanding)
// + the profile mutators (AddProfile + SetProfileEnabled +
// SetProfileRetired + AgentImpact + UpdateProfile + RemoveProfile).
// Extracted from accessors.go during the Day-207 god-file split.
// Public API unchanged.
package accessors

import (
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/standing"
)

// ---- Day 15: Standing / Roster CRUD ----

// AddStanding validates and persists a standing order, journaling
// standing.created so the lifecycle is auditable (SPEC-16 §4).
func (a *Accessor) AddStanding(o standing.Order) (standing.Order, error) {
	saved, err := a.k.Standing().Add(o)
	if err != nil {
		return standing.Order{}, err
	}
	_, _ = a.k.PublishBusEvent(event.Spec{
		Subject: "standing." + saved.ID, Kind: event.KindStandingCreated, Actor: "standing",
		Payload: map[string]any{"id": saved.ID, "name": saved.Name, "triggers": len(saved.Triggers)},
	})
	return saved, nil
}

// SetStandingEnabled pauses/resumes a standing order, journaling
// standing.updated.
func (a *Accessor) SetStandingEnabled(id string, enabled bool) (standing.Order, error) {
	o, err := a.k.Standing().SetEnabled(id, enabled)
	if err != nil {
		return standing.Order{}, err
	}
	state := "paused"
	if enabled {
		state = "resumed"
	}
	_, _ = a.k.PublishBusEvent(event.Spec{
		Subject: "standing." + id, Kind: event.KindStandingUpdated, Actor: "standing",
		Payload: map[string]any{"id": id, "name": o.Name, "enabled": enabled, "action": state},
	})
	return o, nil
}

// UpdateStanding edits a standing order's mutable fields via mutate,
// journaling standing.updated (action "edited") on success.
func (a *Accessor) UpdateStanding(id string, mutate func(*standing.Order)) (standing.Order, bool, error) {
	o, err := a.k.Standing().Update(id, mutate)
	if errors.Is(err, standing.ErrNotFound) {
		return standing.Order{}, false, nil
	}
	if err != nil {
		return standing.Order{}, false, err
	}
	_, _ = a.k.PublishBusEvent(event.Spec{
		Subject: "standing." + id, Kind: event.KindStandingUpdated, Actor: "standing",
		Payload: map[string]any{"id": id, "name": o.Name, "action": "edited"},
	})
	return o, true, nil
}

// RemoveStanding deletes a standing order, journaling
// standing.removed when it existed.
func (a *Accessor) RemoveStanding(id string) (bool, error) {
	o, _ := a.k.Standing().Get(id)
	ok, err := a.k.Standing().Remove(id)
	if err != nil {
		return false, err
	}
	if ok {
		_, _ = a.k.PublishBusEvent(event.Spec{
			Subject: "standing." + id, Kind: event.KindStandingRemoved, Actor: "standing",
			Payload: map[string]any{"id": id, "name": o.Name},
		})
	}
	return ok, nil
}

// AddProfile validates and persists a named agent profile, journaling
// roster.created so the agent's birth is auditable.
func (a *Accessor) AddProfile(p roster.Profile) (roster.Profile, error) {
	saved, err := a.k.Roster().Add(p)
	if err != nil {
		return roster.Profile{}, err
	}
	_, _ = a.k.PublishBusEvent(event.Spec{
		Subject: "roster." + saved.Slug, Kind: event.KindRosterCreated, Actor: "roster",
		Payload: map[string]any{"id": saved.ID, "slug": saved.Slug, "name": saved.Name, "model": saved.Model},
	})
	return saved, nil
}

// SetProfileEnabled pauses/resumes an agent profile, journaling
// roster.updated.
func (a *Accessor) SetProfileEnabled(ref string, enabled bool) (roster.Profile, error) {
	p, err := a.k.Roster().SetEnabled(ref, enabled)
	if err != nil {
		return roster.Profile{}, err
	}
	state := "paused"
	if enabled {
		state = "resumed"
	}
	_, _ = a.k.PublishBusEvent(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "enabled": enabled, "action": state},
	})
	return p, nil
}

// SetProfileRetired moves an agent to the graveyard (true) or revives
// it (false) by ref, journaling roster.updated. Retiring also pauses
// the agent so it stops firing (M846). A graveyard agent is excluded
// from delegation (runSubAgent).
func (a *Accessor) SetProfileRetired(ref string, retired bool, reason ...string) (roster.Profile, error) {
	p, err := a.k.Roster().SetRetired(ref, retired, reason...)
	if err != nil {
		return roster.Profile{}, err
	}
	action := "revived"
	if retired {
		action = "retired"
	}
	_, _ = a.k.PublishBusEvent(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "retired": retired, "reason": p.RetiredReason, "action": action},
	})
	return p, nil
}

// AgentImpact reports what depends on an agent before it is
// retired/removed (M846). The operator sees this in the retire
// confirmation so the "etkileri" are explicit. Returns the affected
// orders as "name (id)" strings, or nil when nothing references it.
func (a *Accessor) AgentImpact(slug string) []string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil
	}
	var out []string
	for _, o := range a.k.Standing().List() {
		if strings.EqualFold(strings.TrimSpace(o.Agent), slug) {
			name := o.Name
			if name == "" {
				name = o.ID
			}
			out = append(out, fmt.Sprintf("%s (%s)", name, o.ID))
		}
	}
	return out
}

// UpdateProfile edits a profile's mutable fields via mutate,
// journaling roster.updated (action "edited"). Returns false + nil
// error for an unknown ref.
func (a *Accessor) UpdateProfile(ref string, mutate func(*roster.Profile)) (roster.Profile, bool, error) {
	p, err := a.k.Roster().Update(ref, mutate)
	if errors.Is(err, roster.ErrNotFound) {
		return roster.Profile{}, false, nil
	}
	if err != nil {
		return roster.Profile{}, false, err
	}
	_, _ = a.k.PublishBusEvent(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "action": "edited"},
	})
	return p, true, nil
}

// RemoveProfile deletes an agent profile, journaling roster.removed
// when it existed. Shipped guardians (System) are protected from
// hard delete (M961): they are the daemon's own self-healing fleet.
// They can still be paused or retired.
func (a *Accessor) RemoveProfile(ref string) (bool, error) {
	if p, ok := a.k.Roster().Get(ref); ok && p.System {
		return false, fmt.Errorf("agent %q is a protected system guardian — pause or retire it instead of removing", p.Slug)
	}
	gone, ok, err := a.k.Roster().Remove(ref)
	if err != nil {
		return false, err
	}
	if ok {
		_, _ = a.k.PublishBusEvent(event.Spec{
			Subject: "roster." + gone.Slug, Kind: event.KindRosterRemoved, Actor: "roster",
			Payload: map[string]any{"id": gone.ID, "slug": gone.Slug, "name": gone.Name},
		})
	}
	return ok, nil
}
