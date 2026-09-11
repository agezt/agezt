// SPDX-License-Identifier: MIT

// Forge lifecycle: create, promote, reassign, quarantine, archive, restore, and revert.
// Code extracted from forge.go during the Day-36 god-file split. Public API unchanged.
package skill


import (
	"errors"
	"fmt"
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

// Promote advances a skill along draft→shadow→active (or un-quarantines back
// to active), journaling skill.promoted. Returns the new status.
func (f *Forge) Promote(corr, id string) (Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.promoteWithReason(corr, id, "")
}

// promoteWithReason is Promote with an optional reason recorded on the event —
// used by auto-staging (M399) to mark the gate that advanced the skill. An empty
// reason omits the field, keeping a manual promote's payload unchanged.
func (f *Forge) promoteWithReason(corr, id, reason string) (Status, error) {
	sk, _, err := f.get(id)
	if err != nil {
		return "", err
	}
	target, ok := PromoteTarget(sk.Status)
	if !ok {
		return sk.Status, fmt.Errorf("%w: %s cannot be promoted", ErrIllegalTransition, sk.Status)
	}
	if !CanTransition(sk.Status, target) {
		return sk.Status, fmt.Errorf("%w: %s→%s", ErrIllegalTransition, sk.Status, target)
	}
	from := sk.Status
	sk.Status = target
	sk.LastSeenMS = f.now().UnixMilli()
	if err := f.store.Put(sk); err != nil {
		return "", err
	}
	payload := map[string]any{
		"id": id, "name": sk.Name, "from": string(from), "to": string(target),
	}
	if reason != "" {
		payload["reason"] = reason
	}
	f.publish(event.KindSkillPromoted, corr, payload)
	return target, nil
}

// Reassign changes a skill's owning agent (M942), the ownership analogue of the
// per-agent memory promote valve (M915). newAgent == "" shares the skill with
// the whole pool (clears the private-to-one-agent wall); a non-empty slug makes
// it private to that agent. It is a no-op (found=true) when the owner is already
// newAgent. Emits skill.shared when sharing, skill.reassigned otherwise. The
// caller (controlplane) validates that a non-empty target slug exists in the
// roster before calling. Ownership is orthogonal to the draft→active lifecycle,
// so Status is untouched and any skill may be reassigned.
func (f *Forge) Reassign(corr, id, newAgent string) (Skill, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sk, found, err := f.get(id)
	if err != nil {
		return Skill{}, found, err
	}
	if sk.Agent == newAgent {
		return sk, true, nil // already there — no event, no churn
	}
	fromAgent := sk.Agent
	sk.Agent = newAgent
	sk.LastSeenMS = f.now().UnixMilli()
	if err := f.store.Put(sk); err != nil {
		return Skill{}, true, err
	}
	if newAgent == "" {
		f.publish(event.KindSkillShared, corr, map[string]any{
			"id": id, "name": sk.Name, "from_agent": fromAgent,
		})
	} else {
		f.publish(event.KindSkillReassigned, corr, map[string]any{
			"id": id, "name": sk.Name, "from_agent": fromAgent, "to_agent": newAgent,
		})
	}
	return sk, true, nil
}

// Quarantine pulls an active or shadow skill out of production, journaling
// skill.quarantined with the reason.
func (f *Forge) Quarantine(corr, id, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.quarantineLocked(corr, id, reason)
}

// quarantineLocked is Quarantine's body; the caller must hold f.mu. Used directly by
// maybeAutoQuarantine (already under the lock via RecordOutcome) to avoid re-locking.
func (f *Forge) quarantineLocked(corr, id, reason string) error {
	sk, _, err := f.get(id)
	if err != nil {
		return err
	}
	if !CanTransition(sk.Status, StatusQuarantined) {
		return fmt.Errorf("%w: %s→quarantined", ErrIllegalTransition, sk.Status)
	}
	from := sk.Status
	sk.Status = StatusQuarantined
	sk.LastSeenMS = f.now().UnixMilli()
	if err := f.store.Put(sk); err != nil {
		return err
	}
	f.publish(event.KindSkillQuarantined, corr, map[string]any{
		"id": id, "name": sk.Name, "from": string(from), "reason": reason,
	})
	return nil
}

// Archive retires a skill without restoring lineage. This is for ownership
// cleanup, e.g. deleting an agent and removing its private skills, not reverting
// a bad version back to a parent.
func (f *Forge) Archive(corr, id, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	sk, _, err := f.get(id)
	if err != nil {
		return err
	}
	if sk.Status == StatusArchived {
		return nil
	}
	if !CanTransition(sk.Status, StatusArchived) {
		return fmt.Errorf("%w: %s→archived", ErrIllegalTransition, sk.Status)
	}
	from := sk.Status
	sk.Status = StatusArchived
	sk.LastSeenMS = f.now().UnixMilli()
	if err := f.store.Put(sk); err != nil {
		return err
	}
	f.publish(event.KindSkillReverted, corr, map[string]any{
		"id": id, "name": sk.Name, "from": string(from), "reason": reason, "archived": true,
	})
	return nil
}

// RestoreStatus restores only the lifecycle status recorded by a rollback
// checkpoint. It deliberately bypasses the forward-only transition matrix, but
// it still validates that the target is a real persisted status and journals the
// restore as a new event.
func (f *Forge) RestoreStatus(corr, id string, target Status, reason string) (Status, Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !ValidStatus(target) {
		return "", "", fmt.Errorf("%w: invalid status %q", ErrIllegalTransition, target)
	}
	sk, _, err := f.get(id)
	if err != nil {
		return "", "", err
	}
	from := sk.Status
	sk.Status = target
	sk.LastSeenMS = f.now().UnixMilli()
	if err := f.store.Put(sk); err != nil {
		return "", "", err
	}
	payload := map[string]any{
		"id": id, "name": sk.Name, "from": string(from), "to": string(target),
	}
	if reason != "" {
		payload["reason"] = reason
	}
	f.publish(event.KindSkillRestored, corr, payload)
	return from, target, nil
}

// Revert appends a reversal (SPEC-05 §5.3): it archives the target skill and
// re-activates its most recent non-archived lineage parent if there is one, so
// reverting a bad new version restores the previous good one. History is never
// edited — the prior states remain in the journal; this just moves the records
// forward and emits skill.reverted. Returns the id of the restored parent (or
// "" if none).
func (f *Forge) Revert(corr, id string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sk, _, err := f.get(id)
	if err != nil {
		return "", err
	}
	if sk.Status == StatusArchived {
		return "", fmt.Errorf("%w: already archived", ErrIllegalTransition)
	}
	nowMS := f.now().UnixMilli()
	sk.Status = StatusArchived
	sk.LastSeenMS = nowMS
	if err := f.store.Put(sk); err != nil {
		return "", err
	}

	restored := ""
	for i := len(sk.Lineage) - 1; i >= 0; i-- {
		parent, found, err := f.store.Get(sk.Lineage[i])
		if err != nil {
			return "", err
		}
		if !found || parent.Status == StatusArchived {
			continue
		}
		// Respect the state machine: only restore a parent that may legally become
		// active (already active, or shadow/quarantined → active). A draft parent
		// must NOT be force-activated — that would skip the shadow gate (M424). Try
		// the next-older lineage parent instead.
		if parent.Status != StatusActive && !CanTransition(parent.Status, StatusActive) {
			continue
		}
		parent.Status = StatusActive
		parent.LastSeenMS = nowMS
		if err := f.store.Put(parent); err != nil {
			return "", err
		}
		restored = parent.ID
		break
	}
	f.publish(event.KindSkillReverted, corr, map[string]any{
		"id": id, "name": sk.Name, "restored": restored,
	})
	return restored, nil
}

// visibleTo reports whether an acting agent may retrieve a skill (M932):
// shared skills (no owner) are everyone's; a private skill is its owner's
// alone — the default daemon identity (empty slug) sees only the shared pool. The
// same scope wall per-agent memory draws (M915).