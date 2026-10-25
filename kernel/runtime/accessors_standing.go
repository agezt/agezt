// SPDX-License-Identifier: MIT

package runtime

// Provenance: Kernel accessors: data (DataLake, ConfigCenter) + standing CRUD
//             (AddStanding/SetStandingEnabled/UpdateStanding/RemoveStanding). Code
//             extracted from accessors.go during the Day-56 god-file split. Public
//             API unchanged.

import (
	"errors"

	"github.com/agezt/agezt/kernel/contract/artifactapi"
	"github.com/agezt/agezt/kernel/contract/datalakeapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/event"
)

// Artifacts returns the content-addressed artifact store (SPEC-04 §3.6), where
// the loop offloads oversized tool outputs. Used by retrieval surfaces.
func (k *Kernel) Artifacts() artifactapi.Blob { return k.artifacts }

// Voice returns the configured voice adapter (STT/TTS), or nil when unset. The
// channel inbound path uses it to auto-transcribe inbound voice notes.
func (k *Kernel) Voice() Voice { return k.cfg.Voice }

// ArtifactIndex returns the metadata index over the blob store (M822) — the
// browsable/deletable per-arrival entries (inbound images, tool outputs) the
// file manager and inbound-image persistence use.
//
// It is the HOST-facing port: the kernel counts, reclaims and guards the
// index, so it needs more than the four verbs a tool gets.
func (k *Kernel) ArtifactIndex() artifactapi.HostIndex { return k.artIndex }

// DataLake returns the Personal Data Lake (M834) — the file-based structured
// collections agents build and share, surfaced by the `db` tool and the Web UI.
func (k *Kernel) DataLake() datalakeapi.Lake { return k.lake }

// errNoStandingLibrary is the answer the kernel gives when the host holds no
// standing-order library.
//
// The library is the DAEMON's file (W3.50): the kernel holds the contract's
// store port, and a host that was given none holds a nil port. The refusal
// is the answer — not a panic, and not a silent "nothing to do".
var errNoStandingLibrary = errors.New("runtime: the standing library is not available on this daemon")

// Standing returns the standing-order library (SPEC-16 §4), backing `agt
// standing`.
//
// It is the contract's STORE port (W3.50): the daemon opens the file and
// hands the port over, so the kernel names the contract and not the
// module's implementation. Nil when the host gave no library — callers must
// check, and the surfaces that do answer "not available" instead of calling
// into nothing.
func (k *Kernel) Standing() standingapi.Store { return k.standing }

// AddStanding validates and persists a standing order, journaling
// standing.created so the lifecycle is auditable (SPEC-16 §4).
func (k *Kernel) AddStanding(o standingapi.Order) (standingapi.Order, error) {
	if k.standing == nil {
		return standingapi.Order{}, errNoStandingLibrary
	}
	saved, err := k.standing.Add(o)
	if err != nil {
		return standingapi.Order{}, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "standing." + saved.ID, Kind: event.KindStandingCreated, Actor: "standing",
		Payload: map[string]any{"id": saved.ID, "name": saved.Name, "triggers": len(saved.Triggers)},
	})
	return saved, nil
}

// SetStandingEnabled pauses/resumes a standing order, journaling standing.updated.
func (k *Kernel) SetStandingEnabled(id string, enabled bool) (standingapi.Order, error) {
	if k.standing == nil {
		return standingapi.Order{}, errNoStandingLibrary
	}
	o, err := k.standing.SetEnabled(id, enabled)
	if err != nil {
		return standingapi.Order{}, err
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
func (k *Kernel) UpdateStanding(id string, mutate func(*standingapi.Order)) (standingapi.Order, bool, error) {
	if k.standing == nil {
		return standingapi.Order{}, false, errNoStandingLibrary
	}
	o, err := k.standing.Update(id, mutate)
	if errors.Is(err, standingapi.ErrNotFound) {
		return standingapi.Order{}, false, nil
	}
	if err != nil {
		return standingapi.Order{}, false, err
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
	if k.standing == nil {
		return false, errNoStandingLibrary
	}
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
