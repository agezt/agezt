// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/roster"
)

// SetRetiredRequest keeps every argument raw: ref is a strict string and the
// reason is read leniently.
type SetRetiredRequest struct {
	Ref    json.RawMessage `json:"ref,omitempty"`
	Reason json.RawMessage `json:"reason,omitempty"`
}

// RetireImpactSummary is the teardown preview taken before the retirement,
// extended with what the retirement paused.
type RetireImpactSummary struct {
	ImpactOutput
	StandingPaused  int `json:"standing_paused"`
	SchedulesPaused int `json:"schedules_paused"`
}

type RetireOutput struct {
	Profile         ProfileOutput        `json:"profile"`
	Impact          []string             `json:"impact"`
	ImpactSummary   *RetireImpactSummary `json:"impact_summary"`
	StandingPaused  int                  `json:"standing_paused"`
	SchedulesPaused int                  `json:"schedules_paused"`
}

type ReviveOutput struct {
	Profile         ProfileOutput `json:"profile"`
	StandingPaused  int           `json:"standing_paused"`
	SchedulesPaused int           `json:"schedules_paused"`
}

// SetRetiredPorts are the native effects of retiring and reviving an agent.
type SetRetiredPorts struct {
	Get        func(string) (core.Profile, bool)
	Impact     func(core.Profile) ImpactOutput
	SetRetired func(ref string, retired bool, reason string) (core.Profile, error)
	// PauseStanding/PauseSchedules pause the agent's triggers on retire and
	// report how many; the counts report what a revive leaves paused.
	PauseStanding        func(slug string) (int, error)
	PauseSchedules       func(slug string) (int, error)
	CountPausedStanding  func(slug string) int
	CountPausedSchedules func(slug string) int
	NewCorrelation       func() string
	Publish              func(subject, corr string, payload map[string]any)
	Invalidate           func()
}

// SetRetiredService moves an agent to and from the graveyard. A retirement
// reports what it affected, computed before the change; a revival first
// re-checks the agent's hierarchy references.
type SetRetiredService struct{ ports SetRetiredPorts }

func NewSetRetired(ports SetRetiredPorts) *SetRetiredService { return &SetRetiredService{ports: ports} }

func (s *SetRetiredService) setRetired(in SetRetiredRequest, retired bool) (core.Profile, string, error) {
	ref, err := RefPageRequest{Ref: in.Ref}.ref()
	if err != nil {
		return core.Profile{}, "", err
	}
	p, err := s.ports.SetRetired(ref, retired, lenientString(in.Reason))
	if errors.Is(err, core.ErrNotFound) {
		return core.Profile{}, ref, errors.New("unknown agent: " + ref)
	}
	return p, ref, err
}

func (s *SetRetiredService) Retire(_ context.Context, in SetRetiredRequest) (RetireOutput, error) {
	// Compute the impact BEFORE the state change so a retire reports what it
	// affected.
	var impact []string
	var summary *ImpactOutput
	if ref, err := (RefPageRequest{Ref: in.Ref}).ref(); err == nil {
		if p, ok := s.ports.Get(ref); ok {
			out := s.ports.Impact(p)
			impact, summary = out.StandingOrders, &out
		}
	}
	p, _, err := s.setRetired(in, true)
	if err != nil {
		return RetireOutput{}, err
	}
	standing, err := s.ports.PauseStanding(p.Slug)
	if err != nil {
		return RetireOutput{}, err
	}
	schedules, err := s.ports.PauseSchedules(p.Slug)
	if err != nil {
		return RetireOutput{}, err
	}
	out := RetireOutput{Profile: profileWriteOutput(p).Profile, Impact: impact, StandingPaused: standing, SchedulesPaused: schedules}
	var journaled any
	if summary != nil {
		out.ImpactSummary = &RetireImpactSummary{ImpactOutput: *summary, StandingPaused: standing, SchedulesPaused: schedules}
		journaled = objectPayload(*out.ImpactSummary)
	}
	s.ports.Publish("agent.retire", s.ports.NewCorrelation(), map[string]any{
		"agent":            p.Slug,
		"reason":           p.RetiredReason,
		"retired_ms":       p.RetiredMS,
		"standing_paused":  standing,
		"schedules_paused": schedules,
		"impact_summary":   journaled,
	})
	s.ports.Invalidate()
	return out, nil
}

func (s *SetRetiredService) Revive(_ context.Context, in SetRetiredRequest) (ReviveOutput, error) {
	if ref, err := (RefPageRequest{Ref: in.Ref}).ref(); err == nil {
		if p, ok := s.ports.Get(ref); ok {
			if err := ValidateHierarchyRefs(p, s.ports.Get); err != nil {
				return ReviveOutput{}, err
			}
		}
	}
	p, _, err := s.setRetired(in, false)
	if err != nil {
		return ReviveOutput{}, err
	}
	standing, schedules := s.ports.CountPausedStanding(p.Slug), s.ports.CountPausedSchedules(p.Slug)
	s.ports.Publish("agent.revive", s.ports.NewCorrelation(), map[string]any{
		"agent":            p.Slug,
		"standing_paused":  standing,
		"schedules_paused": schedules,
	})
	s.ports.Invalidate()
	return ReviveOutput{Profile: profileWriteOutput(p).Profile, StandingPaused: standing, SchedulesPaused: schedules}, nil
}

// objectPayload journals a typed value as the generic object the event payload
// has always carried.
func objectPayload(v any) map[string]any {
	raw, _ := json.Marshal(v)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

type retireSchema struct {
	Profile         profileSchema        `json:"profile"`
	Impact          []string             `json:"impact"`
	ImpactSummary   *RetireImpactSummary `json:"impact_summary"`
	StandingPaused  int                  `json:"standing_paused"`
	SchedulesPaused int                  `json:"schedules_paused"`
}

type reviveSchema struct {
	Profile         profileSchema `json:"profile"`
	StandingPaused  int           `json:"standing_paused"`
	SchedulesPaused int           `json:"schedules_paused"`
}

func SetRetiredOperations(provider func(context.Context) *SetRetiredService) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("roster set-retired provider required")
	}
	retireOut, err := schema.FromType(reflect.TypeFor[retireSchema](), false)
	if err != nil {
		return nil, err
	}
	reviveOut, err := schema.FromType(reflect.TypeFor[reviveSchema](), false)
	if err != nil {
		return nil, err
	}
	retire, err := app.NewOperation(opapi.Spec{Name: "agent_retire", OutputSchema: retireOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{},"reason":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/agents/retire"}}, func(ctx context.Context, in SetRetiredRequest) (RetireOutput, error) {
		return provider(ctx).Retire(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	revive, err := app.NewOperation(opapi.Spec{Name: "agent_revive", OutputSchema: reviveOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{},"reason":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/agents/revive"}}, func(ctx context.Context, in SetRetiredRequest) (ReviveOutput, error) {
		return provider(ctx).Revive(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{retire, revive}, nil
}
