// SPDX-License-Identifier: MIT

package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/journal"
	"strings"
	"time"
)

// RequestInput preserves absent versus null and legacy per-command validation order.
type RequestInput struct {
	Status      json.RawMessage `json:"status,omitempty"`
	ID          json.RawMessage `json:"id,omitempty"`
	Intent      json.RawMessage `json:"intent,omitempty"`
	Model       json.RawMessage `json:"model,omitempty"`
	Agent       json.RawMessage `json:"agent,omitempty"`
	Target      json.RawMessage `json:"target,omitempty"`
	Workflow    json.RawMessage `json:"workflow,omitempty"`
	SystemTask  json.RawMessage `json:"system_task,omitempty"`
	Tool        json.RawMessage `json:"tool,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	Enabled     json.RawMessage `json:"enabled,omitempty"`
	Count       json.RawMessage `json:"count,omitempty"`
	OnceAt      json.RawMessage `json:"once_at_unix,omitempty"`
	Cooldown    json.RawMessage `json:"cooldown_sec,omitempty"`
	WindowStart json.RawMessage `json:"window_start,omitempty"`
	WindowEnd   json.RawMessage `json:"window_end,omitempty"`
	Interval    json.RawMessage `json:"interval_sec,omitempty"`
	Days        json.RawMessage `json:"days,omitempty"`
	AtMinutes   json.RawMessage `json:"at_minutes,omitempty"`
	TZ          json.RawMessage `json:"tz,omitempty"`
	Limit       json.RawMessage `json:"limit,omitempty"`
	Cursor      json.RawMessage `json:"cursor,omitempty"`
	Since       json.RawMessage `json:"since_ms,omitempty"`
}
type Providers struct {
	Reads     func(context.Context) *Service
	Lifecycle func(context.Context) *Lifecycle
	Admission func(context.Context) *Admission
	Creation  func(context.Context) *Creation
	Editing   func(context.Context) *Editing
	Firings   func(context.Context) *FiringService
	Now       func(context.Context) time.Time
}

func rawValue(raw json.RawMessage) any {
	var out any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}
func requestString(raw json.RawMessage, key string) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	value, ok := rawValue(raw).(string)
	if !ok {
		return "", fmt.Errorf("args.%s must be a string", key)
	}
	return value, nil
}
func requiredRequestString(raw json.RawMessage, key string) (string, error) {
	value, err := requestString(raw, key)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("args.%s required", key)
	}
	return value, nil
}

type requestField struct {
	raw json.RawMessage
	key string
}

func requestStrings(values ...requestField) ([]string, error) {
	out := make([]string, 0, len(values))
	for _, value := range values {
		text, err := requestString(value.raw, value.key)
		if err != nil {
			return nil, err
		}
		out = append(out, text)
	}
	return out, nil
}
func NumberRequest(raw json.RawMessage, key string) CadenceNumber {
	if len(raw) == 0 {
		return CadenceNumber{}
	}
	value, ok := rawValue(raw).(float64)
	if !ok {
		return CadenceNumber{Present: true, Err: fmt.Errorf("args.%s must be numeric", key)}
	}
	return CadenceNumber{Present: true, Value: value}
}
func (in RequestInput) EditCadence() EditCadence {
	tz, _ := rawValue(in.TZ).(string)
	return EditCadence{TZ: tz, OnceAt: NumberRequest(in.OnceAt, "once_at_unix"), Cooldown: NumberRequest(in.Cooldown, "cooldown_sec"), WindowStart: NumberRequest(in.WindowStart, "window_start"), WindowEnd: NumberRequest(in.WindowEnd, "window_end"), Interval: NumberRequest(in.Interval, "interval_sec"), Days: NumberRequest(in.Days, "days"), AtMinutes: NumberRequest(in.AtMinutes, "at_minutes")}
}
func requestCreateCadence(in RequestInput) (CreateCadence, error) {
	number := func(raw json.RawMessage, key string) (float64, error) {
		n := NumberRequest(raw, key)
		return n.Value, n.Err
	}
	if len(in.OnceAt) > 0 {
		at, err := number(in.OnceAt, "once_at_unix")
		return CreateCadence{Mode: cadence.ModeOnce, OnceAt: at}, err
	}
	if len(in.Cooldown) > 0 {
		sec, err := number(in.Cooldown, "cooldown_sec")
		return CreateCadence{Mode: cadence.ModeContinuous, Seconds: sec}, err
	}
	if len(in.WindowStart) > 0 {
		start, err := number(in.WindowStart, "window_start")
		if err != nil {
			return CreateCadence{}, err
		}
		end, err := number(in.WindowEnd, "window_end")
		if err != nil {
			return CreateCadence{}, err
		}
		sec, err := number(in.Interval, "interval_sec")
		if err != nil {
			return CreateCadence{}, err
		}
		days, err := number(in.Days, "days")
		if err != nil {
			return CreateCadence{}, err
		}
		tz, err := requestString(in.TZ, "tz")
		return CreateCadence{Mode: cadence.ModeWindow, At: start, End: end, Seconds: sec, Days: days, TZ: tz}, err
	}
	if len(in.AtMinutes) > 0 {
		at, err := number(in.AtMinutes, "at_minutes")
		if err != nil {
			return CreateCadence{}, err
		}
		days, err := number(in.Days, "days")
		if err != nil {
			return CreateCadence{}, err
		}
		tz, err := requestString(in.TZ, "tz")
		return CreateCadence{Mode: cadence.ModeDaily, At: at, Days: days, TZ: tz}, err
	}
	sec, err := number(in.Interval, "interval_sec")
	return CreateCadence{Mode: cadence.ModeInterval, Seconds: sec}, err
}
func bindSchedule[I, O any](ops *[]app.Operation, spec opapi.Spec, handler func(context.Context, I) (O, error)) error {
	spec.AllowUnknownInput = true
	spec.InputSchema = scheduleRequestSchema
	op, err := app.NewOperation(spec, handler)
	if err == nil {
		*ops = append(*ops, op)
	}
	return err
}
func Operations(providers Providers) ([]app.Operation, error) {
	if providers.Reads == nil || providers.Lifecycle == nil || providers.Admission == nil || providers.Creation == nil || providers.Editing == nil || providers.Firings == nil {
		return nil, errors.New("schedule service providers required")
	}
	if providers.Now == nil {
		providers.Now = func(context.Context) time.Time { return time.Now() }
	}
	recordSchema, listSchema, editSchema, err := scheduleOutputSchemas()
	if err != nil {
		return nil, err
	}
	var ops []app.Operation
	bindings := []func() error{
		func() error {
			return bindSchedule(&ops, opapi.Spec{Name: "schedule_list", ReadOnly: true, OutputSchema: listSchema}, func(ctx context.Context, _ RequestInput) (ListOutput, error) {
				return providers.Reads(ctx).List(ctx, ListInput{})
			})
		},
		func() error {
			return bindSchedule(&ops, opapi.Spec{Name: "schedule_system_tasks", ReadOnly: true}, func(ctx context.Context, _ RequestInput) (SystemTasksOutput, error) {
				return providers.Reads(ctx).SystemTasks(ctx, SystemTasksInput{})
			})
		},
		func() error {
			return bindSchedule(&ops, opapi.Spec{Name: "schedule_add", OutputSchema: recordSchema}, func(ctx context.Context, in RequestInput) (Record, error) {
				values, err := requestStrings(requestField{in.Intent, "intent"}, requestField{in.Model, "model"}, requestField{in.Agent, "agent"}, requestField{in.Target, "target"}, requestField{in.Workflow, "workflow"}, requestField{in.SystemTask, "system_task"}, requestField{in.Tool, "tool"})
				if err != nil {
					return Record{}, err
				}
				target, err := providers.Admission(ctx).AddTarget(AddTargetInput{Intent: values[0], Model: values[1], Agent: values[2], Target: values[3], Workflow: values[4], SystemTask: values[5], Tool: values[6], Payload: rawValue(in.Payload), PayloadPresent: len(in.Payload) > 0})
				if err != nil {
					return Record{}, err
				}
				selected, err := requestCreateCadence(in)
				if err != nil {
					return Record{}, err
				}
				return providers.Creation(ctx).Create(ctx, CreateInput{Target: target, Cadence: selected})
			})
		},
		func() error {
			return bindSchedule(&ops, opapi.Spec{Name: "schedule_rm"}, func(ctx context.Context, in RequestInput) (RemoveOutput, error) {
				id, err := requiredRequestString(in.ID, "id")
				if err != nil {
					return RemoveOutput{}, err
				}
				return providers.Lifecycle(ctx).Remove(ctx, IDInput{ID: id})
			})
		},
		func() error {
			return bindSchedule(&ops, opapi.Spec{Name: "schedule_run"}, func(ctx context.Context, in RequestInput) (RunOutput, error) {
				id, err := requiredRequestString(in.ID, "id")
				if err != nil {
					return RunOutput{}, err
				}
				return providers.Lifecycle(ctx).Run(ctx, IDInput{ID: id})
			})
		},
		func() error {
			return bindSchedule(&ops, opapi.Spec{Name: "schedule_enable"}, func(ctx context.Context, in RequestInput) (EnableOutput, error) {
				id, err := requiredRequestString(in.ID, "id")
				if err != nil {
					return EnableOutput{}, err
				}
				enabled := false
				switch value := rawValue(in.Enabled).(type) {
				case bool:
					enabled = value
				case string:
					enabled = strings.EqualFold(value, "true") || value == "1"
				}
				return providers.Lifecycle(ctx).Enable(ctx, EnableInput{ID: id, Enabled: enabled})
			})
		},
		func() error {
			return bindSchedule(&ops, opapi.Spec{Name: "schedule_edit", OutputSchema: editSchema}, func(ctx context.Context, in RequestInput) (EditOutput, error) {
				id, err := requiredRequestString(in.ID, "id")
				if err != nil {
					return EditOutput{}, err
				}
				editing := providers.Editing(ctx)
				state, found := editing.Begin(ctx, IDInput{ID: id})
				if !found {
					return EditOutput{Updated: false}, nil
				}
				values, err := requestStrings(requestField{in.Target, "target"}, requestField{in.Workflow, "workflow"}, requestField{in.SystemTask, "system_task"}, requestField{in.Tool, "tool"})
				if err != nil {
					return EditOutput{}, err
				}
				agent, _ := rawValue(in.Agent).(string)
				target, err := providers.Admission(ctx).EditTarget(EditTargetInput{Current: state.Current, Target: values[0], Workflow: values[1], SystemTask: values[2], Tool: values[3], Agent: agent, TargetPresent: len(in.Target) > 0, AgentPresent: len(in.Agent) > 0, Payload: rawValue(in.Payload), PayloadPresent: len(in.Payload) > 0})
				if err != nil {
					return EditOutput{}, err
				}
				field := func(raw json.RawMessage) EditField {
					value, _ := rawValue(raw).(string)
					return EditField{Value: value, Present: len(raw) > 0}
				}
				tz, tzErr := requestString(in.TZ, "tz")
				return editing.Apply(ctx, EditInput{ID: id, State: state, Target: target, TargetPresent: len(in.Target) > 0, AgentPresent: len(in.Agent) > 0, Payload: rawValue(in.Payload), PayloadPresent: len(in.Payload) > 0, Intent: field(in.Intent), Model: field(in.Model), Cadence: in.EditCadence(), TZ: tz, TZErr: tzErr})
			})
		},
		func() error {
			return bindSchedule(&ops, opapi.Spec{Name: "schedule_test", ReadOnly: true}, func(ctx context.Context, in RequestInput) (TestOutput, error) {
				id, err := requiredRequestString(in.ID, "id")
				if err != nil {
					return TestOutput{}, err
				}
				count := 5
				n := NumberRequest(in.Count, "count")
				if n.Err != nil {
					return TestOutput{}, n.Err
				}
				if n.Present {
					count = int(n.Value)
				}
				if count < 1 {
					count = 1
				}
				if count > 100 {
					count = 100
				}
				return providers.Reads(ctx).Test(ctx, TestInput{ID: id, Count: count})
			})
		},
		func() error {
			return bindSchedule(&ops, opapi.Spec{Name: "schedule_fires", ReadOnly: true, Authz: opapi.OwnTenant, Tenancy: opapi.CallerTenant}, func(ctx context.Context, in RequestInput) (FiresOutput, error) {
				limit := 20
				if value, ok := rawValue(in.Limit).(float64); ok {
					limit = int(value)
				}
				cursorMS, cursorSeq, cursorOK := journal.DecodeCursor(rawValue(in.Cursor))
				values, err := requestStrings(requestField{in.ID, "id"}, requestField{in.Status, "status"}, requestField{in.Intent, "intent"})
				if err != nil {
					return FiresOutput{}, err
				}
				var cutoff int64
				if since, ok := rawValue(in.Since).(float64); ok && int64(since) > 0 {
					cutoff = providers.Now(ctx).UnixMilli() - int64(since)
				}
				return providers.Firings(ctx).Fires(ctx, FiresInput{Limit: limit, ID: values[0], Status: values[1], Intent: values[2], CutoffMS: cutoff, CursorMS: cursorMS, CursorSeq: cursorSeq, CursorOK: cursorOK})
			})
		},
		func() error {
			return bindSchedule(&ops, opapi.Spec{Name: "schedule_stats", ReadOnly: true, Authz: opapi.OwnTenant, Tenancy: opapi.CallerTenant}, func(ctx context.Context, in RequestInput) (StatsOutput, error) {
				id, err := requestString(in.ID, "id")
				if err != nil {
					return StatsOutput{}, err
				}
				var since, cutoff int64
				if value, ok := rawValue(in.Since).(float64); ok {
					since = int64(value)
				}
				if since > 0 {
					cutoff = providers.Now(ctx).UnixMilli() - since
				}
				return providers.Firings(ctx).Stats(ctx, StatsInput{ID: id, SinceMS: since, CutoffMS: cutoff})
			})
		},
	}
	for _, binding := range bindings {
		if err := binding(); err != nil {
			return nil, err
		}
	}
	return ops, nil
}
