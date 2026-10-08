// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/roster"
)

// SetEnabledRequest keeps enabled raw: CLI/JSON callers send a bool and the
// Web UI query transport sends "true"/"false"/"1"/"0"; anything else pauses.
type SetEnabledRequest struct {
	Ref     json.RawMessage `json:"ref,omitempty"`
	Enabled json.RawMessage `json:"enabled,omitempty"`
}

func (r SetEnabledRequest) enabled() bool {
	v, _ := rawValue(r.Enabled)
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true") || t == "1"
	}
	return false
}

type SetEnabledOutput struct {
	Profile         ProfileOutput `json:"profile"`
	StandingPaused  *int          `json:"standing_paused,omitempty"`
	SchedulesPaused *int          `json:"schedules_paused,omitempty"`
}

// SetEnabledService pauses or resumes an agent. Resuming reports how many of
// its standing orders and schedules are still paused, since retirement paused
// them and resuming the agent does not re-enable them.
type SetEnabledService struct {
	set             func(string, bool) (core.Profile, error)
	pausedStanding  func(string) int
	pausedSchedules func(string) int
	invalidate      func()
}

func NewSetEnabled(set func(string, bool) (core.Profile, error), pausedStanding, pausedSchedules func(string) int, invalidate func()) *SetEnabledService {
	return &SetEnabledService{set: set, pausedStanding: pausedStanding, pausedSchedules: pausedSchedules, invalidate: invalidate}
}

func (s *SetEnabledService) SetEnabled(_ context.Context, in SetEnabledRequest) (SetEnabledOutput, error) {
	ref, err := RefPageRequest{Ref: in.Ref}.ref()
	if err != nil {
		return SetEnabledOutput{}, err
	}
	enabled := in.enabled()
	p, err := s.set(ref, enabled)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return SetEnabledOutput{}, errors.New("unknown agent: " + ref)
		}
		if errors.Is(err, core.ErrRetired) {
			return SetEnabledOutput{}, errors.New("agent " + ref + " is retired — revive it first")
		}
		return SetEnabledOutput{}, err
	}
	out := SetEnabledOutput{Profile: ProfileOutput{Profile: p, Kind: p.Kind(), Managed: !p.AllowsDirectCall()}}
	if enabled {
		standing, schedules := s.pausedStanding(p.Slug), s.pausedSchedules(p.Slug)
		out.StandingPaused, out.SchedulesPaused = &standing, &schedules
	}
	s.invalidate()
	return out, nil
}

// setEnabledSchema mirrors SetEnabledOutput with the marshaler-free profile view.
type setEnabledSchema struct {
	Profile         profileSchema `json:"profile"`
	StandingPaused  *int          `json:"standing_paused,omitempty"`
	SchedulesPaused *int          `json:"schedules_paused,omitempty"`
}

func SetEnabledOperations(provider func(context.Context) *SetEnabledService) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("roster set-enabled provider required")
	}
	output, err := schema.FromType(reflect.TypeFor[setEnabledSchema](), false)
	if err != nil {
		return nil, err
	}
	op, err := app.NewOperation(opapi.Spec{Name: "agent_set_enabled", OutputSchema: output, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{},"enabled":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/agents/enable"}}, func(ctx context.Context, in SetEnabledRequest) (SetEnabledOutput, error) {
		return provider(ctx).SetEnabled(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
