// SPDX-License-Identifier: MIT

// Cadence store setters: SetIntent/Model/Agent/IntentTarget/WorkflowTarget/SystemTaskTarget/ToolTarget + Reschedule.
// Code extracted from cadence_store.go during the Day-58 god-file split. Public API unchanged.
package cadence


import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)


// validateWindow checks the shared constraints for windowed schedules.
func validateWindow(interval time.Duration, startMin, endMin, days int) error {
	if interval < MinInterval {
		return fmt.Errorf("cadence: interval %s is below the %s minimum", interval, MinInterval)
	}
	if startMin < 0 || startMin > 1439 || endMin < 0 || endMin > 1439 {
		return fmt.Errorf("cadence: window bounds must be 00:00..23:59")
	}
	if endMin <= startMin {
		return fmt.Errorf("cadence: window end must be after its start")
	}
	if days < 0 || days > AllDays {
		return fmt.Errorf("cadence: day-mask must be 0..%d", AllDays)
	}
	return nil
}

// SetIntent changes an entry's intent in place. Returns whether it exists.
func (s *Store) SetIntent(id, intent string) (bool, error) {
	intent = strings.TrimSpace(intent)
	if intent == "" {
		return false, fmt.Errorf("cadence: intent is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			e.Intent = intent
			return true, s.save()
		}
	}
	return false, nil
}

// SetModel changes an entry's model in place (empty clears it). Returns whether
// it exists.
func (s *Store) SetModel(id, model string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			e.Model = strings.TrimSpace(model)
			return true, s.save()
		}
	}
	return false, nil
}

// SetAgent changes the roster agent binding in place (empty clears it). Returns
// whether the entry exists. The caller validates that a non-empty slug exists.
func (s *Store) SetAgent(id, agent string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			e.Agent = strings.TrimSpace(agent)
			return true, s.save()
		}
	}
	return false, nil
}

// SetIntentTarget restores the historical LLM intent target and clears
// target-specific fields.
func (s *Store) SetIntentTarget(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			e.Target = TargetIntent
			e.Workflow = ""
			e.SystemTask = ""
			e.Tool = ""
			e.Payload = nil
			return true, s.save()
		}
	}
	return false, nil
}

// SetWorkflowTarget makes an entry fire a stored workflow instead of a governed
// agent/intent run. The caller validates the workflow ref exists. Agent binding
// is intentionally preserved: a workflow schedule may run under an agent's
// identity, budget, and tool policy.
func (s *Store) SetWorkflowTarget(id, ref string, payload json.RawMessage) (bool, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false, fmt.Errorf("cadence: workflow ref is required")
	}
	cp := append(json.RawMessage(nil), payload...)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			e.Target = TargetWorkflow
			e.Workflow = ref
			e.SystemTask = ""
			e.Tool = ""
			e.Payload = cp
			return true, s.save()
		}
	}
	return false, nil
}

// SetSystemTaskTarget makes an entry fire a daemon maintenance task instead of
// an LLM intent. The task name is validated against the known system-task enum
// (IsSystemTask) as defense-in-depth: the control plane already validates, but
// a direct store caller (env-seeded schedule, a future code path, or a
// hand-edited schedules.json) should not be able to set an arbitrary task name
// that could smuggle unexpected behavior through the fire path.
func (s *Store) SetSystemTaskTarget(id, task string) (bool, error) {
	task = strings.TrimSpace(task)
	if task == "" {
		return false, fmt.Errorf("cadence: system task is required")
	}
	if !IsSystemTask(task) {
		return false, fmt.Errorf("cadence: unknown system task %q (valid: %s)", task, strings.Join(SystemTasks(), ", "))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			e.Target = TargetSystemTask
			e.SystemTask = task
			e.Workflow = ""
			e.Tool = ""
			e.Payload = nil
			e.Agent = ""
			e.Model = ""
			return true, s.save()
		}
	}
	return false, nil
}

// SetToolTarget makes an entry invoke a registered tool directly instead of
// asking an LLM to interpret an intent. The caller validates the tool exists.
// Agent binding is preserved so a tool schedule can run under that agent's
// permissions and spend limits; model overrides are cleared because direct
// tool invocations do not select an LLM.
func (s *Store) SetToolTarget(id, tool string, payload json.RawMessage) (bool, error) {
	tool = strings.TrimSpace(tool)
	if tool == "" {
		return false, fmt.Errorf("cadence: tool is required")
	}
	cp := append(json.RawMessage(nil), payload...)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			e.Target = TargetTool
			e.Tool = tool
			e.Workflow = ""
			e.SystemTask = ""
			e.Payload = cp
			e.Model = ""
			return true, s.save()
		}
	}
	return false, nil
}

// Reschedule replaces an entry's cadence in place (preserving id/source/created/
// enabled), recomputing its next-run time. mode selects which of the cadence
// parameters apply: ModeOnce → onceAt; ModeDaily → atMinutes+days; ModeWindow
// → interval+window; ModeContinuous → completion cooldown; ModeInterval →
// interval. Returns whether the entry exists.
func (s *Store) Reschedule(id, mode string, interval time.Duration, atMinutes, endMinutes, days int, tz string, onceAt, now time.Time) (bool, error) {
	zoned, err := applyZone(now, tz)
	if err != nil {
		return false, fmt.Errorf("cadence: unknown timezone %q: %w", tz, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID != id {
			continue
		}
		switch mode {
		case ModeOnce:
			if !onceAt.After(now) {
				return false, fmt.Errorf("cadence: one-shot time must be in the future")
			}
			e.Mode = ModeOnce
			e.IntervalSec, e.AtMinutes, e.EndMinutes, e.Days, e.TZ = 0, 0, 0, 0, ""
			e.NextRunUnix = onceAt.Unix()
		case ModeDaily:
			if atMinutes < 0 || atMinutes > 1439 {
				return false, fmt.Errorf("cadence: time-of-day must be 00:00..23:59")
			}
			if days < 0 || days > AllDays {
				return false, fmt.Errorf("cadence: day-mask must be 0..%d", AllDays)
			}
			e.Mode = ModeDaily
			e.IntervalSec, e.EndMinutes = 0, 0
			e.AtMinutes, e.Days, e.TZ = atMinutes, days, strings.TrimSpace(tz)
			e.NextRunUnix = nextDaily(zoned, atMinutes, days).Unix()
		case ModeWindow:
			if err := validateWindow(interval, atMinutes, endMinutes, days); err != nil {
				return false, err
			}
			e.Mode = ModeWindow
			e.IntervalSec = int64(interval / time.Second)
			e.AtMinutes, e.EndMinutes, e.Days, e.TZ = atMinutes, endMinutes, days, strings.TrimSpace(tz)
			e.NextRunUnix = nextWindowSlot(zoned, atMinutes, endMinutes, e.IntervalSec, days).Unix()
		case ModeContinuous:
			if interval < MinInterval {
				return false, fmt.Errorf("cadence: cooldown %s is below the %s minimum", interval, MinInterval)
			}
			e.Mode = ModeContinuous
			e.AtMinutes, e.EndMinutes, e.Days, e.TZ = 0, 0, 0, ""
			e.IntervalSec = int64(interval / time.Second)
			e.NextRunUnix = now.Unix()
		default: // ModeInterval
			if interval < MinInterval {
				return false, fmt.Errorf("cadence: interval %s is below the %s minimum", interval, MinInterval)
			}
			e.Mode = ModeInterval
			e.AtMinutes, e.EndMinutes, e.Days, e.TZ = 0, 0, 0, ""
			e.IntervalSec = int64(interval / time.Second)
			e.NextRunUnix = now.Add(interval).Unix()
		}
		return true, s.save()
	}
	return false, nil
}
