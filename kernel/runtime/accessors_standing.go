// SPDX-License-Identifier: MIT

// Kernel accessors: data (DataLake, ConfigCenter) + standing CRUD (AddStanding/SetStandingEnabled/UpdateStanding/RemoveStanding).
// Code extracted from accessors.go during the Day-56 god-file split. Public API unchanged.
package runtime


import (
	"errors"

	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/datalake"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/standing"
)



// Artifacts returns the content-addressed artifact store (SPEC-04 §3.6), where
// the loop offloads oversized tool outputs. Used by retrieval surfaces.
func (k *Kernel) Artifacts() *artifact.Store { return k.artifacts }

// Voice returns the configured voice adapter (STT/TTS), or nil when unset. The
// channel inbound path uses it to auto-transcribe inbound voice notes.
func (k *Kernel) Voice() Voice { return k.cfg.Voice }

// ArtifactIndex returns the metadata index over the blob store (M822) — the
// browsable/deletable per-arrival entries (inbound images, tool outputs) the
// file manager and inbound-image persistence use.
func (k *Kernel) ArtifactIndex() *artifact.Index { return k.artIndex }

// DataLake returns the Personal Data Lake (M834) — the file-based structured
// collections agents build and share, surfaced by the `db` tool and the Web UI.
func (k *Kernel) DataLake() *datalake.Lake { return k.lake }

// Standing returns the standing wake-rule store (SPEC-16 §4), backing `agt
// standing`. Always non-nil after Open.
func (k *Kernel) Standing() *standing.Store { return k.standing }

// AddStanding validates and persists a standing order, journaling
// standing.created so the lifecycle is auditable (SPEC-16 §4).
func (k *Kernel) AddStanding(o standing.Order) (standing.Order, error) {
	saved, err := k.standing.Add(o)
	if err != nil {
		return standing.Order{}, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "standing." + saved.ID, Kind: event.KindStandingCreated, Actor: "standing",
		Payload: map[string]any{"id": saved.ID, "name": saved.Name, "triggers": len(saved.Triggers)},
	})
	return saved, nil
}

// SetStandingEnabled pauses/resumes a standing order, journaling standing.updated.
func (k *Kernel) SetStandingEnabled(id string, enabled bool) (standing.Order, error) {
	o, err := k.standing.SetEnabled(id, enabled)
	if err != nil {
		return standing.Order{}, err
	}
	state := "paused"
	if enabled {
		state = "resumed"
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "standing." + id, Kind: event.KindStandingUpdated, Actor: "standing",
		Payload: map[string]any{"id": id, "name": o.Name, "enabled": enabled, "action": state},
	})
	return o, nil
}

// UpdateStanding edits a standing order's mutable fields via mutate, journaling
// standing.updated (action "edited") on success. Identity/lifecycle fields are
// protected by the store. Returns the updated order and whether the id existed
// (false + nil error for an unknown id, mirroring the schedule-edit path).
func (k *Kernel) UpdateStanding(id string, mutate func(*standing.Order)) (standing.Order, bool, error) {
	o, err := k.standing.Update(id, mutate)
	if errors.Is(err, standing.ErrNotFound) {
		return standing.Order{}, false, nil
	}
	if err != nil {
		return standing.Order{}, false, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "standing." + id, Kind: event.KindStandingUpdated, Actor: "standing",
		Payload: map[string]any{"id": id, "name": o.Name, "action": "edited"},
	})
	return o, true, nil
}

// RemoveStanding deletes a standing order, journaling standing.removed when it
// existed. Returns whether it existed.
func (k *Kernel) RemoveStanding(id string) (bool, error) {
	o, _ := k.standing.Get(id)
	ok, err := k.standing.Remove(id)
	if err != nil {
		return false, err
	}
	if ok {
		_, _ = k.bus.Publish(event.Spec{
			Subject: "standing." + id, Kind: event.KindStandingRemoved, Actor: "standing",
			Payload: map[string]any{"id": id, "name": o.Name},
		})
	}
	return ok, nil
}

// Roster returns the durable agent-profile store (M783). Always non-nil after Open.