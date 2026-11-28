// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"errors"
	"reflect"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// IncidentLineage is the operator incident chain a wake or repair belongs to.
type IncidentLineage struct {
	IncidentID       string
	RootIncidentID   string
	ParentIncidentID string
}

type WakeOutput struct {
	Accepted      bool   `json:"accepted"`
	Agent         string `json:"agent"`
	CorrelationID string `json:"correlation_id"`
}

// incidentLineage reads the incident ids the row carried, trimmed.
func incidentLineage(incident, root, parent string) IncidentLineage {
	return IncidentLineage{IncidentID: strings.TrimSpace(incident), RootIncidentID: strings.TrimSpace(root), ParentIncidentID: strings.TrimSpace(parent)}
}

// ManagedDirectCallError is the operator-facing refusal for an action sent to a
// managed sub-agent instead of its manager.
func ManagedDirectCallError(p agentapi.Profile, action string) string {
	manager := strings.TrimSpace(p.ParentAgent)
	if manager == "" {
		manager = strings.TrimSpace(p.OwnerAgent)
	}
	hint := "route the work through its parent/owner agent"
	if manager != "" {
		hint = "wake " + manager + " or delegate through it"
	}
	return "agent " + p.Slug + " is a managed sub-agent and cannot be " + action + " directly; " + hint
}

// directTarget resolves the agent an operator action is sent to, rejecting
// unknown, retired, paused and managed agents in that order.
func directTarget(get func(string) (agentapi.Profile, bool), ref string, action string) (agentapi.Profile, error) {
	ref, err := refOf(ref)
	if err != nil {
		return agentapi.Profile{}, err
	}
	p, ok := get(ref)
	if !ok {
		return agentapi.Profile{}, errors.New("unknown agent: " + ref)
	}
	if p.Retired {
		return agentapi.Profile{}, errors.New("agent " + p.Slug + " is retired — revive it first")
	}
	if !p.Enabled {
		return agentapi.Profile{}, errors.New("agent " + p.Slug + " is paused")
	}
	if !p.AllowsDirectCall() {
		return agentapi.Profile{}, errors.New(ManagedDirectCallError(p, action))
	}
	return p, nil
}

// BuildOperatorWakeIntent returns the explicit intent, or the default manual
// wake-up prompt carrying the reason and incident lineage.
func BuildOperatorWakeIntent(explicit, slug, reason string, lineage IncidentLineage) string {
	if text := strings.TrimSpace(explicit); text != "" {
		return text
	}
	var b strings.Builder
	b.WriteString("Manual wake-up.\n")
	b.WriteString("You are agent ")
	b.WriteString(slug)
	b.WriteString(". You were explicitly woken by the operator/control plane.\n")
	if reason = strings.TrimSpace(reason); reason != "" {
		b.WriteString("Reason: ")
		b.WriteString(reason)
		b.WriteString("\n")
	}
	if root := strings.TrimSpace(lineage.RootIncidentID); root != "" {
		b.WriteString("Incident root: ")
		b.WriteString(root)
		b.WriteString("\n")
	}
	if incident := strings.TrimSpace(lineage.IncidentID); incident != "" {
		b.WriteString("Incident hop: ")
		b.WriteString(incident)
		b.WriteString("\n")
	}
	b.WriteString("Inspect your durable instructions, memory, mailbox, tasklist, and current health context. Do the next concrete recovery step and then stop.")
	return b.String()
}

// WakeService accepts an operator wake: it validates the agent, journals the
// request and hands the run to launch, which must not block.
type WakeService struct {
	get            func(string) (agentapi.Profile, bool)
	newCorrelation func() string
	publish        func(subject, corr string, payload map[string]any)
	launch         func(corr string, p agentapi.Profile, intent, reason string, lineage IncidentLineage)
}

func NewWake(get func(string) (agentapi.Profile, bool), newCorrelation func() string, publish func(subject, corr string, payload map[string]any), launch func(corr string, p agentapi.Profile, intent, reason string, lineage IncidentLineage)) *WakeService {
	return &WakeService{get: get, newCorrelation: newCorrelation, publish: publish, launch: launch}
}

func (s *WakeService) Wake(_ context.Context, in opapi.AgentWakeRequest) (WakeOutput, error) {
	p, err := directTarget(s.get, in.Ref, "called")
	if err != nil {
		return WakeOutput{}, err
	}
	intent := in.Intent
	reason := strings.TrimSpace(in.Reason)
	lineage := incidentLineage(in.IncidentID, in.RootIncidentID, in.ParentIncidentID)
	intent = BuildOperatorWakeIntent(strings.TrimSpace(intent), p.Slug, reason, lineage)
	if strings.TrimSpace(intent) == "" {
		return WakeOutput{}, errors.New("agent wake requires args.intent or args.reason")
	}
	corr := s.newCorrelation()
	s.publish("agent.wake", corr, map[string]any{
		"phase":              "requested",
		"agent":              p.Slug,
		"reason":             reason,
		"intent":             truncate(intent, 240),
		"autonomy_runbook":   agentapi.AutonomyRunbook(p),
		"incident_id":        lineage.IncidentID,
		"root_incident_id":   lineage.RootIncidentID,
		"parent_incident_id": lineage.ParentIncidentID,
	})
	s.launch(corr, p, intent, reason, lineage)
	return WakeOutput{Accepted: true, Agent: p.Slug, CorrelationID: corr}, nil
}

func WakeOperations(provider func(context.Context) *WakeService) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("roster wake provider required")
	}
	output, err := schema.FromType(reflect.TypeFor[WakeOutput](), false)
	if err != nil {
		return nil, err
	}
	op, err := app.NewOperation(opapi.Spec{Name: opapi.AgentWake, OutputSchema: output, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}, func(ctx context.Context, in opapi.AgentWakeRequest) (WakeOutput, error) {
		return provider(ctx).Wake(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
