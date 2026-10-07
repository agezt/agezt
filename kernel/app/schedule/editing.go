// SPDX-License-Identifier: MIT

package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/cadence"
	"time"
)

type EditingStore interface {
	Get(string) (cadence.Entry, bool)
	SetIntent(string, string) (bool, error)
	SetModel(string, string) (bool, error)
	SetAgent(string, string) (bool, error)
	SetWorkflowTarget(string, string, json.RawMessage) (bool, error)
	SetToolTarget(string, string, json.RawMessage) (bool, error)
	SetSystemTaskTarget(string, string) (bool, error)
	SetIntentTarget(string) (bool, error)
	Reschedule(string, string, time.Duration, int, int, int, string, time.Time, time.Time) (bool, error)
}
type EditingHost struct {
	WorkflowName func(string) (string, bool)
	Tool         func(string) bool
	Now          func() time.Time
}
type Editing struct {
	store EditingStore
	host  EditingHost
}

func NewEditing(store EditingStore, host EditingHost) *Editing {
	if host.Now == nil {
		host.Now = time.Now
	}
	return &Editing{store: store, host: host}
}

type EditState struct {
	Current cadence.Entry
	Now     time.Time
}

func (s *Editing) Begin(_ context.Context, in IDInput) (EditState, bool) {
	entry, found := s.store.Get(in.ID)
	if !found {
		return EditState{}, false
	}
	return EditState{Current: entry, Now: s.host.Now()}, true
}

type EditField struct {
	Value   string
	Present bool
}
type EditInput struct {
	ID                                          string
	State                                       EditState
	Target                                      EditTarget
	TargetPresent, AgentPresent, PayloadPresent bool
	Intent, Model                               EditField
	Payload                                     any
	Cadence                                     EditCadence
	TZ                                          string
	TZErr                                       error
}
type EditOutput struct {
	*Record
	Updated bool `json:"updated"`
}

func (s *Editing) Apply(_ context.Context, in EditInput) (EditOutput, error) {
	if err := ValidateEditCadence(in.Cadence, in.State.Now); err != nil {
		return EditOutput{}, err
	}
	store, id, now := s.store, in.ID, in.State.Now
	target, workflowRef, systemTask, toolName, editAgent := in.Target.Target, in.Target.Workflow, in.Target.SystemTask, in.Target.Tool, in.Target.Agent
	// Field edits (any subset). A failure on intent (empty) is reported.
	if in.Intent.Present {
		intent := in.Intent.Value
		if _, err := store.SetIntent(id, intent); err != nil {
			return EditOutput{}, err
		}
	}
	if in.Model.Present {
		model := in.Model.Value
		_, _ = store.SetModel(id, model)
	}
	if in.AgentPresent {
		_, _ = store.SetAgent(id, editAgent)
	}
	if target == cadence.TargetWorkflow {
		if systemTask != "" {
			return EditOutput{}, errors.New("workflow schedules cannot also set args.system_task")
		}
		if toolName != "" {
			return EditOutput{}, errors.New("workflow schedules cannot also set args.tool")
		}
		w, found := s.host.WorkflowName(workflowRef)
		if !found {
			return EditOutput{}, errors.New("unknown workflow: " + workflowRef)
		}
		var payload json.RawMessage
		if p := in.Payload; in.PayloadPresent {
			b, err := json.Marshal(p)
			if err != nil {
				return EditOutput{}, errors.New("payload must be JSON-serializable: " + err.Error())
			}
			payload = b
		}
		if _, err := store.SetWorkflowTarget(id, w, payload); err != nil {
			return EditOutput{}, err
		}
	} else if target != "" && target != cadence.TargetIntent {
		if target != cadence.TargetSystemTask {
			if target != cadence.TargetTool {
				return EditOutput{}, errors.New("unknown schedule target: " + target)
			}
			if workflowRef != "" {
				return EditOutput{}, errors.New("tool schedules cannot also set args.workflow")
			}
			if systemTask != "" {
				return EditOutput{}, errors.New("tool schedules cannot also set args.system_task")
			}
			if toolName == "" {
				return EditOutput{}, errors.New("args.tool required for tool schedules")
			}
			if !s.host.Tool(toolName) {
				return EditOutput{}, errors.New("unknown tool: " + toolName)
			}
			var payload json.RawMessage
			if p := in.Payload; in.PayloadPresent {
				b, err := json.Marshal(p)
				if err != nil {
					return EditOutput{}, errors.New("payload must be JSON-serializable: " + err.Error())
				}
				payload = b
			}
			if _, err := store.SetToolTarget(id, toolName, payload); err != nil {
				return EditOutput{}, err
			}
		} else {
			if workflowRef != "" {
				return EditOutput{}, errors.New("system task schedules cannot also set args.workflow")
			}
			if toolName != "" {
				return EditOutput{}, errors.New("system task schedules cannot also set args.tool")
			}
			if in.PayloadPresent {
				return EditOutput{}, errors.New("system task schedules do not accept args.payload")
			}
			if !cadence.IsSystemTask(systemTask) {
				return EditOutput{}, errors.New("unknown system task: " + systemTask)
			}
			if _, err := store.SetSystemTaskTarget(id, systemTask); err != nil {
				return EditOutput{}, err
			}
		}
	} else if in.TargetPresent && target == cadence.TargetIntent {
		if _, err := store.SetIntentTarget(id); err != nil {
			return EditOutput{}, err
		}
	}

	// At most one cadence change: once | continuous | window | daily | interval.
	tz, err := in.TZ, in.TZErr
	if err != nil {
		return EditOutput{}, err
	}
	if in.Cadence.OnceAt.Present {
		at, _, parseErr := in.Cadence.OnceAt.values()
		if parseErr != nil {
			return EditOutput{}, errors.New(parseErr.Error())
		}
		_, err = store.Reschedule(id, cadence.ModeOnce, 0, 0, 0, 0, "", time.Unix(int64(at), 0), now)
	} else if in.Cadence.Cooldown.Present {
		sec, _, parseErr := in.Cadence.Cooldown.values()
		if parseErr != nil {
			return EditOutput{}, errors.New(parseErr.Error())
		}
		_, err = store.Reschedule(id, cadence.ModeContinuous, time.Duration(sec)*time.Second, 0, 0, 0, "", time.Time{}, now)
	} else if in.Cadence.WindowStart.Present {
		start, _, parseErr := in.Cadence.WindowStart.values()
		if parseErr != nil {
			return EditOutput{}, errors.New(parseErr.Error())
		}
		end, _, parseErr := in.Cadence.WindowEnd.values()
		if parseErr != nil {
			return EditOutput{}, errors.New(parseErr.Error())
		}
		sec, _, parseErr := in.Cadence.Interval.values()
		if parseErr != nil {
			return EditOutput{}, errors.New(parseErr.Error())
		}
		days, _, parseErr := in.Cadence.Days.values()
		if parseErr != nil {
			return EditOutput{}, errors.New(parseErr.Error())
		}
		_, err = store.Reschedule(id, cadence.ModeWindow, time.Duration(sec)*time.Second, int(start), int(end), int(days), tz, time.Time{}, now)
	} else if in.Cadence.AtMinutes.Present {
		at, _, parseErr := in.Cadence.AtMinutes.values()
		if parseErr != nil {
			return EditOutput{}, errors.New(parseErr.Error())
		}
		days, _, parseErr := in.Cadence.Days.values()
		if parseErr != nil {
			return EditOutput{}, errors.New(parseErr.Error())
		}
		_, err = store.Reschedule(id, cadence.ModeDaily, 0, int(at), 0, int(days), tz, time.Time{}, now)
	} else if in.Cadence.Interval.Present {
		sec, _, parseErr := in.Cadence.Interval.values()
		if parseErr != nil {
			return EditOutput{}, errors.New(parseErr.Error())
		}
		_, err = store.Reschedule(id, cadence.ModeInterval, time.Duration(sec)*time.Second, 0, 0, 0, "", time.Time{}, now)
	}
	if err != nil {
		return EditOutput{}, err
	}

	e, _ := store.Get(id)
	row := Project(e)
	return EditOutput{Record: &row, Updated: true}, nil
}
