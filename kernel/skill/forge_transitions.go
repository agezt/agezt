// SPDX-License-Identifier: MIT
//
// Forge status transitions: Promote + promoteWithReason + Reassign +
// Quarantine + quarantineLocked + Archive + RestoreStatus + Revert.
// Extracted from forge_lifecycle.go during the Day-203 god-file split.
// Public API unchanged.
package skill

import (
	"fmt"

	"github.com/agezt/agezt/kernel/event"
)

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
