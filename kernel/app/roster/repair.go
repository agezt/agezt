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

// RepairRequest keeps every argument raw: ref is a strict string, while reason
// and the incident ids are read leniently (anything but a string is empty).
type RepairRequest struct {
	Ref              json.RawMessage `json:"ref,omitempty"`
	Reason           json.RawMessage `json:"reason,omitempty"`
	IncidentID       json.RawMessage `json:"incident_id,omitempty"`
	RootIncidentID   json.RawMessage `json:"root_incident_id,omitempty"`
	ParentIncidentID json.RawMessage `json:"parent_incident_id,omitempty"`
}

// RepairOutput acknowledges an accepted repair; the outcome is journaled under
// its correlation.
type RepairOutput = WakeOutput

// RepairService accepts an operator repair: it validates the agent, journals the
// request and hands the governed repair run to launch, which must not block.
type RepairService struct {
	get            func(string) (core.Profile, bool)
	newCorrelation func() string
	publish        func(subject, corr string, payload map[string]any)
	launch         func(corr string, p core.Profile, reason string, lineage IncidentLineage)
}

func NewRepair(get func(string) (core.Profile, bool), newCorrelation func() string, publish func(subject, corr string, payload map[string]any), launch func(corr string, p core.Profile, reason string, lineage IncidentLineage)) *RepairService {
	return &RepairService{get: get, newCorrelation: newCorrelation, publish: publish, launch: launch}
}

func (s *RepairService) Repair(_ context.Context, in RepairRequest) (RepairOutput, error) {
	p, err := directTarget(s.get, in.Ref, "repaired")
	if err != nil {
		return RepairOutput{}, err
	}
	corr := s.newCorrelation()
	reason := lenientString(in.Reason)
	lineage := incidentLineage(in.IncidentID, in.RootIncidentID, in.ParentIncidentID)
	s.publish("agent.repair", corr, map[string]any{
		"phase":              "requested",
		"agent":              p.Slug,
		"reason":             reason,
		"incident_id":        lineage.IncidentID,
		"root_incident_id":   lineage.RootIncidentID,
		"parent_incident_id": lineage.ParentIncidentID,
	})
	s.launch(corr, p, reason, lineage)
	return RepairOutput{Accepted: true, Agent: p.Slug, CorrelationID: corr}, nil
}

func RepairOperations(provider func(context.Context) *RepairService) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("roster repair provider required")
	}
	output, err := schema.FromType(reflect.TypeFor[RepairOutput](), false)
	if err != nil {
		return nil, err
	}
	op, err := app.NewOperation(opapi.Spec{Name: "agent_repair", OutputSchema: output, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{},"reason":{},"incident_id":{},"root_incident_id":{},"parent_incident_id":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/agents/repair"}}, func(ctx context.Context, in RepairRequest) (RepairOutput, error) {
		return provider(ctx).Repair(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
