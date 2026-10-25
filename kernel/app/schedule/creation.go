// SPDX-License-Identifier: MIT

package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"time"
)

// CreationStore is the cadence creation and target binding port.
type CreationStore interface {
	Add(string, time.Duration, string, string, time.Time) (cadenceapi.Entry, error)
	AddOnce(string, time.Time, string, string, time.Time) (cadenceapi.Entry, error)
	AddContinuous(string, time.Duration, string, string, time.Time) (cadenceapi.Entry, error)
	AddWindow(string, time.Duration, int, int, int, string, string, string, time.Time) (cadenceapi.Entry, error)
	AddDaily(string, int, int, string, string, string, time.Time) (cadenceapi.Entry, error)
	SetAgent(string, string) (bool, error)
	SetWorkflowTarget(string, string, json.RawMessage) (bool, error)
	SetSystemTaskTarget(string, string) (bool, error)
	SetToolTarget(string, string, json.RawMessage) (bool, error)
	Remove(string) (bool, error)
	Get(string) (cadenceapi.Entry, bool)
}

// CreateCadence carries only the selected, parsed cadence variant.
type CreateCadence struct {
	Mode                           string
	Seconds, At, End, Days, OnceAt float64
	TZ                             string
}
type CreateInput struct {
	Target  AddTarget
	Cadence CreateCadence
}
type Creation struct {
	store CreationStore
	now   func() time.Time
}

func NewCreation(store CreationStore, now func() time.Time) *Creation {
	if now == nil {
		now = time.Now
	}
	return &Creation{store: store, now: now}
}
func (s *Creation) Create(_ context.Context, in CreateInput) (Record, error) {
	if s.store == nil {
		return Record{}, errNoScheduleLibrary
	}
	plan, c := in.Target, in.Cadence
	var e cadenceapi.Entry
	var err error
	switch c.Mode {
	case cadenceapi.ModeOnce:
		e, err = s.store.AddOnce(plan.Intent, time.Unix(int64(c.OnceAt), 0), plan.Model, cadenceapi.SourceOperator, s.now())
	case cadenceapi.ModeContinuous:
		if c.Seconds < 1 {
			return Record{}, errors.New("args.cooldown_sec must be >= 1")
		}
		e, err = s.store.AddContinuous(plan.Intent, time.Duration(c.Seconds)*time.Second, plan.Model, cadenceapi.SourceOperator, s.now())
	case cadenceapi.ModeWindow:
		e, err = s.store.AddWindow(plan.Intent, time.Duration(c.Seconds)*time.Second, int(c.At), int(c.End), int(c.Days), c.TZ, plan.Model, cadenceapi.SourceOperator, s.now())
	case cadenceapi.ModeDaily:
		e, err = s.store.AddDaily(plan.Intent, int(c.At), int(c.Days), c.TZ, plan.Model, cadenceapi.SourceOperator, s.now())
	case cadenceapi.ModeInterval:
		if c.Seconds < 1 {
			return Record{}, errors.New("args.interval_sec must be >= 1 (or pass at_minutes)")
		}
		e, err = s.store.Add(plan.Intent, time.Duration(c.Seconds)*time.Second, plan.Model, cadenceapi.SourceOperator, s.now())
	default:
		return Record{}, errors.New("unknown schedule cadence mode: " + c.Mode)
	}
	if err != nil {
		return Record{}, err
	}
	// Preserve existing best-effort compensation; this is not a store transaction.
	failCreated := func(cause error) (Record, error) { _, _ = s.store.Remove(e.ID); return Record{}, cause }
	if plan.Agent != "" {
		ok, err := s.store.SetAgent(e.ID, plan.Agent)
		if err != nil {
			return failCreated(err)
		}
		if !ok {
			return failCreated(errors.New("schedule disappeared before agent binding"))
		}
		e.Agent = plan.Agent
	}
	if plan.Target == cadenceapi.TargetWorkflow {
		ok, err := s.store.SetWorkflowTarget(e.ID, plan.Workflow, plan.WorkflowPayload)
		if err != nil {
			return failCreated(err)
		}
		if !ok {
			return failCreated(errors.New("schedule disappeared before workflow binding"))
		}
		e.Target, e.Workflow, e.Payload = cadenceapi.TargetWorkflow, plan.Workflow, plan.WorkflowPayload
	}
	if plan.Target == cadenceapi.TargetSystemTask {
		ok, err := s.store.SetSystemTaskTarget(e.ID, plan.SystemTask)
		if err != nil {
			return failCreated(err)
		}
		if !ok {
			return failCreated(errors.New("schedule disappeared before system task binding"))
		}
		e.Target, e.SystemTask = cadenceapi.TargetSystemTask, plan.SystemTask
	}
	if plan.Target == cadenceapi.TargetTool {
		ok, err := s.store.SetToolTarget(e.ID, plan.Tool, plan.ToolPayload)
		if err != nil {
			return failCreated(err)
		}
		if !ok {
			return failCreated(errors.New("schedule disappeared before tool binding"))
		}
		e.Target, e.Tool, e.Payload = cadenceapi.TargetTool, plan.Tool, plan.ToolPayload
	}
	if refreshed, ok := s.store.Get(e.ID); ok {
		e = refreshed
	}
	return Project(e), nil
}
