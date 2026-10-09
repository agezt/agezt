// SPDX-License-Identifier: MIT

package edict

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	core "github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
)

// OverlayJournal is the routed kernel's journal as the overlay folds read it.
type OverlayJournal interface {
	Range(func(*event.Event) error) error
	Head() (int64, string)
}

// SaveSnapshot writes the routed kernel's overlay snapshot file.
type SaveSnapshot func(*core.OverlaySnapshot) error

// Overlay folds the journaled policy changes into the net runtime overlay, with
// the same projection the daemon replays at boot.
type Overlay struct {
	journal OverlayJournal
	save    SaveSnapshot
	publish Publish
}

func NewOverlay(journal OverlayJournal, save SaveSnapshot, publish Publish) *Overlay {
	return &Overlay{journal: journal, save: save, publish: publish}
}

// OverlayRequest takes no arguments; the tenant only routes.
type OverlayRequest struct{}

type OverlayDenyRow struct {
	Name      string   `json:"name"`
	Substring string   `json:"substring"`
	AppliesTo []string `json:"applies_to"`
}

type OverlayOutput struct {
	Levels        map[string]string `json:"levels"`
	DenyRules     []OverlayDenyRow  `json:"deny_rules"`
	Mode          string            `json:"mode"`
	Empty         bool              `json:"empty"`
	ChangesFolded int               `json:"changes_folded"`
}

type CompactOutput struct {
	Folded     int   `json:"folded"`
	Compacted  int   `json:"compacted"`
	ThroughSeq int64 `json:"through_seq"`
	Empty      bool  `json:"empty"`
}

// changes collects every well-formed policy.changed payload in journal order;
// a malformed one is skipped, exactly as the boot replay does.
func (o *Overlay) changes() ([]core.PolicyChange, error) {
	var changes []core.PolicyChange
	err := o.journal.Range(func(e *event.Event) error {
		if e.Kind != event.KindPolicyChanged {
			return nil
		}
		var ch core.PolicyChange
		if json.Unmarshal(e.Payload, &ch) != nil {
			return nil
		}
		changes = append(changes, ch)
		return nil
	})
	return changes, err
}

// Show reports the net overlay: levels, deny rules in fold order, the mode
// override ("" when none) and how many changes were folded.
func (o *Overlay) Show(context.Context, OverlayRequest) (OverlayOutput, error) {
	changes, err := o.changes()
	if err != nil {
		return OverlayOutput{}, err
	}
	overlay := core.ProjectPolicyChanges(changes)
	out := OverlayOutput{Levels: map[string]string{}, DenyRules: make([]OverlayDenyRow, 0, len(overlay.DenyRules)), Empty: overlay.IsEmpty(), ChangesFolded: len(changes)}
	for c, level := range overlay.Levels {
		out.Levels[string(c)] = level.String()
	}
	for _, r := range overlay.DenyRules {
		row := OverlayDenyRow{Name: r.Name, Substring: r.Substring, AppliesTo: make([]string, 0, len(r.AppliesTo))}
		for _, c := range r.AppliesTo {
			row.AppliesTo = append(row.AppliesTo, string(c))
		}
		out.DenyRules = append(out.DenyRules, row)
	}
	if overlay.Mode != nil {
		out.Mode = overlay.Mode.String()
	}
	return out, nil
}

// Compact writes a snapshot of the overlay's minimal change list through the
// current head, then journals its content hash as policy.compacted so a boot
// trusts only the snapshot the journal vouches for. The journal itself is
// untouched.
func (o *Overlay) Compact(context.Context, OverlayRequest) (CompactOutput, error) {
	changes, err := o.changes()
	if err != nil {
		return CompactOutput{}, err
	}
	overlay := core.ProjectPolicyChanges(changes)
	head, _ := o.journal.Head()
	snap := &core.OverlaySnapshot{ThroughSeq: head, Changes: overlay.ToChanges()}
	if err := o.save(snap); err != nil {
		return CompactOutput{}, err
	}
	o.publish(event.Spec{Subject: "policy.compacted", Kind: event.KindPolicyCompacted, Actor: "controlplane", Payload: map[string]any{"through_seq": head, "content_hash": snap.ContentHash()}})
	return CompactOutput{Folded: len(changes), Compacted: len(snap.Changes), ThroughSeq: head, Empty: overlay.IsEmpty()}, nil
}

// OverlayOperations declares the overlay view and its compaction. Both stay
// audited, as before. The view is tenant-owned. Compaction rewrites durable
// policy state, so only the primary may run it, for any tenant it names.
func OverlayOperations(provider func(context.Context) *Overlay) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("edict overlay provider required")
	}
	input := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"tenant":{}}}`)
	var ops []app.Operation
	for _, b := range []func() error{
		func() error {
			return bind(&ops, opapi.Spec{Name: "edict_overlay", InputSchema: input}, func(ctx context.Context, in OverlayRequest) (OverlayOutput, error) {
				return provider(ctx).Show(ctx, in)
			})
		},
		func() error {
			return bindAs(&ops, opapi.Spec{Name: "edict_compact", InputSchema: input}, opapi.PrimaryOnly, func(ctx context.Context, in OverlayRequest) (CompactOutput, error) {
				return provider(ctx).Compact(ctx, in)
			})
		},
	} {
		if err := b(); err != nil {
			return nil, err
		}
	}
	return ops, nil
}
