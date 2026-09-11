// SPDX-License-Identifier: MIT

// Kernel accessors: roster CRUD (Roster/AddProfile/SetProfileEnabled/SetProfileRetired/AgentImpact/UpdateProfile/RemoveProfile).
// Code extracted from accessors.go during the Day-56 god-file split. Public API unchanged.
package runtime


import (
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
)


func (k *Kernel) Roster() *roster.Store { return k.roster }

// AddProfile validates and persists a named agent profile, journaling
// roster.created so the agent's birth is auditable.
func (k *Kernel) AddProfile(p roster.Profile) (roster.Profile, error) {
	saved, err := k.roster.Add(p)
	if err != nil {
		return roster.Profile{}, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "roster." + saved.Slug, Kind: event.KindRosterCreated, Actor: "roster",
		Payload: map[string]any{"id": saved.ID, "slug": saved.Slug, "name": saved.Name, "model": saved.Model},
	})
	return saved, nil
}

// SetProfileEnabled pauses/resumes an agent profile, journaling roster.updated.
func (k *Kernel) SetProfileEnabled(ref string, enabled bool) (roster.Profile, error) {
	p, err := k.roster.SetEnabled(ref, enabled)
	if err != nil {
		return roster.Profile{}, err
	}
	state := "paused"
	if enabled {
		state = "resumed"
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "enabled": enabled, "action": state},
	})
	return p, nil
}

// SetProfileRetired moves an agent to the graveyard (true) or revives it (false)
// by ref, journaling roster.updated. Retiring also pauses the agent so it stops
// firing (M846). A graveyard agent is excluded from delegation (runSubAgent).
func (k *Kernel) SetProfileRetired(ref string, retired bool, reason ...string) (roster.Profile, error) {
	p, err := k.roster.SetRetired(ref, retired, reason...)
	if err != nil {
		return roster.Profile{}, err
	}
	action := "revived"
	if retired {
		action = "retired"
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "retired": retired, "reason": p.RetiredReason, "action": action},
	})
	return p, nil
}

// AgentImpact reports what depends on an agent before it is retired/removed
// (M846) — the standing orders that fire AS it. The operator sees this in the
// retire confirmation so the "etkileri" are explicit, not a surprise. Returns the
// affected orders as "name (id)" strings, or nil when nothing references it.
func (k *Kernel) AgentImpact(slug string) []string {
	slug = strings.TrimSpace(slug)
	if slug == "" || k.standing == nil {
		return nil
	}
	var out []string
	for _, o := range k.standing.List() {
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

// UpdateProfile edits a profile's mutable fields via mutate, journaling
// roster.updated (action "edited"). Identity/lifecycle fields are protected by
// the store. Returns false + nil error for an unknown ref (standing pattern).
func (k *Kernel) UpdateProfile(ref string, mutate func(*roster.Profile)) (roster.Profile, bool, error) {
	p, err := k.roster.Update(ref, mutate)
	if errors.Is(err, roster.ErrNotFound) {
		return roster.Profile{}, false, nil
	}
	if err != nil {
		return roster.Profile{}, false, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "action": "edited"},
	})
	return p, true, nil
}

// RemoveProfile deletes an agent profile, journaling roster.removed when it
// existed. Returns whether it existed.
func (k *Kernel) RemoveProfile(ref string) (bool, error) {
	// Shipped guardians (System) are protected from hard delete (M961): they are
	// the daemon's own self-healing fleet. They can still be paused or retired.
	if p, ok := k.roster.Get(ref); ok && p.System {
		return false, fmt.Errorf("agent %q is a protected system guardian — pause or retire it instead of removing", p.Slug)
	}
	gone, ok, err := k.roster.Remove(ref)
	if err != nil {
		return false, err
	}
	if ok {
		_, _ = k.bus.Publish(event.Spec{
			Subject: "roster." + gone.Slug, Kind: event.KindRosterRemoved, Actor: "roster",
			Payload: map[string]any{"id": gone.ID, "slug": gone.Slug, "name": gone.Name},
		})
	}
	return ok, nil
}
