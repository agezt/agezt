// SPDX-License-Identifier: MIT

// Forge lifecycle: create + auto-shadow + writeBundle + lineageFor.
// The status-transition methods (Promote / Reassign / Quarantine / Archive /
// RestoreStatus / Revert) live in forge_transitions.go.
// Extracted from forge_lifecycle.go during the Day-203 god-file split.
// Public API unchanged.
package skill

import (
	"errors"
	"strings"

	"github.com/agezt/agezt/kernel/event"
)

func (f *Forge) Create(corr string, spec CreateSpec) (Skill, bool, error) {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return Skill{}, false, errors.New("skill: empty name")
	}
	if strings.TrimSpace(spec.Body) == "" {
		return Skill{}, false, ErrEmptyBody
	}
	nowMS := f.now().UnixMilli()
	id := ContentID(name, spec.Body)

	f.mu.Lock()
	defer f.mu.Unlock()

	// Materialize any bundle first: resources are keyed by name, so they attach
	// to both a fresh skill and a refreshed-existing one. A bundle-write failure
	// fails the create (a half-installed skill that points at missing files is
	// worse than no skill).
	resources, err := f.writeBundle(name, spec.Resources)
	if err != nil {
		return Skill{}, false, err
	}

	if existing, found, err := f.store.Get(id); err != nil {
		return Skill{}, false, err
	} else if found {
		existing.LastSeenMS = nowMS
		if resources != nil {
			existing.Resources = resources
		}
		if err := f.store.Put(existing); err != nil {
			return Skill{}, false, err
		}
		return existing, false, nil
	}

	sk := Skill{
		ID:            id,
		Name:          name,
		Description:   spec.Description,
		Triggers:      normalizeList(spec.Triggers),
		Body:          spec.Body,
		ToolsRequired: normalizeList(spec.ToolsRequired),
		Resources:     resources,
		Agent:         strings.TrimSpace(spec.Agent),
		Version:       DefaultVersion,
		Lineage:       f.lineageFor(name),
		Status:        StatusDraft,
		CreatedMS:     nowMS,
		LastSeenMS:    nowMS,
	}
	ev := f.publish(event.KindSkillCreated, corr, map[string]any{
		"action": "create", "id": id, "name": name, "status": string(StatusDraft), "agent": sk.Agent,
	})
	if ev != nil {
		sk.SourceEvent = ev.ID
	}
	if err := f.store.Put(sk); err != nil {
		return Skill{}, false, err
	}
	// Auto-stage a well-formed draft to shadow (SPEC-05 §5.2), when enabled. The
	// returned skill reflects the post-staging status, so callers see "shadow".
	f.maybeAutoShadow(corr, sk)
	if cur, found, gerr := f.store.Get(sk.ID); gerr == nil && found {
		sk = cur
	}
	return sk, true, nil
}

// maybeAutoShadow advances a freshly-created draft to shadow when auto-staging is
// enabled and the draft passes the deterministic ShadowTest (SPEC-05 §5.2). Only
// drafts are affected; the promotion is journaled with the gate reason and is
// reversible via the normal lifecycle. Best-effort: a staging failure leaves the
// skill a draft (no worse than auto-staging being off).
func (f *Forge) maybeAutoShadow(corr string, sk Skill) {
	if !f.autoShadow || sk.Status != StatusDraft {
		return
	}
	if ok, _ := ShadowTest(sk); !ok {
		return
	}
	_, _ = f.promoteWithReason(corr, sk.ID, "auto-shadow: shadow-test passed")
}

// writeBundle materializes a skill's resource bundle when both resources and a
// bundle store are present. It returns the manifest (sorted relative paths) to
// record on the skill, or nil when there is nothing to attach (no resources, or
// no bundle store wired). Caller holds f.mu.
func (f *Forge) writeBundle(name string, resources map[string][]byte) ([]string, error) {
	if len(resources) == 0 || f.bundles == nil {
		return nil, nil
	}
	return f.bundles.Write(name, resources)
}

// lineageFor returns the ids of non-archived skills sharing name — the
// versions a new body evolves from.
func (f *Forge) lineageFor(name string) []string {
	all, err := f.store.All()
	if err != nil {
		return nil
	}
	folded := strings.ToLower(strings.TrimSpace(name))
	var out []string
	for _, sk := range all {
		if sk.Status == StatusArchived {
			continue
		}
		if strings.ToLower(strings.TrimSpace(sk.Name)) == folded {
			out = append(out, sk.ID)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

