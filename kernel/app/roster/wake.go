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

// WakeRequest keeps every argument raw: ref and intent are strict strings, while
// reason and the incident ids are read leniently (anything but a string is empty).
type WakeRequest struct {
	Ref              json.RawMessage `json:"ref,omitempty"`
	Intent           json.RawMessage `json:"intent,omitempty"`
	Reason           json.RawMessage `json:"reason,omitempty"`
	IncidentID       json.RawMessage `json:"incident_id,omitempty"`
	RootIncidentID   json.RawMessage `json:"root_incident_id,omitempty"`
	ParentIncidentID json.RawMessage `json:"parent_incident_id,omitempty"`
}

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

func lenientString(raw json.RawMessage) string {
	v, _ := rawValue(raw)
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// incidentLineage reads the incident ids leniently, like the reason.
func incidentLineage(incident, root, parent json.RawMessage) IncidentLineage {
	return IncidentLineage{IncidentID: lenientString(incident), RootIncidentID: lenientString(root), ParentIncidentID: lenientString(parent)}
}

// ManagedDirectCallError is the operator-facing refusal for an action sent to a
// managed sub-agent instead of its manager.
func ManagedDirectCallError(p core.Profile, action string) string {
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
func directTarget(get func(string) (core.Profile, bool), rawRef json.RawMessage, action string) (core.Profile, error) {
	ref, err := RefPageRequest{Ref: rawRef}.ref()
	if err != nil {
		return core.Profile{}, err
	}
	p, ok := get(ref)
	if !ok {
		return core.Profile{}, errors.New("unknown agent: " + ref)
	}
	if p.Retired {
		return core.Profile{}, errors.New("agent " + p.Slug + " is retired — revive it first")
	}
	if !p.Enabled {
		return core.Profile{}, errors.New("agent " + p.Slug + " is paused")
	}
	if !p.AllowsDirectCall() {
		return core.Profile{}, errors.New(ManagedDirectCallError(p, action))
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
	get            func(string) (core.Profile, bool)
	newCorrelation func() string
	publish        func(subject, corr string, payload map[string]any)
	launch         func(corr string, p core.Profile, intent, reason string, lineage IncidentLineage)
}

func NewWake(get func(string) (core.Profile, bool), newCorrelation func() string, publish func(subject, corr string, payload map[string]any), launch func(corr string, p core.Profile, intent, reason string, lineage IncidentLineage)) *WakeService {
	return &WakeService{get: get, newCorrelation: newCorrelation, publish: publish, launch: launch}
}

func (s *WakeService) Wake(_ context.Context, in WakeRequest) (WakeOutput, error) {
	p, err := directTarget(s.get, in.Ref, "called")
	if err != nil {
		return WakeOutput{}, err
	}
	intent, _, err := optionalString(in.Intent, "intent")
	if err != nil {
		return WakeOutput{}, err
	}
	reason := lenientString(in.Reason)
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
		"autonomy_runbook":   core.AutonomyRunbook(p),
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
	op, err := app.NewOperation(opapi.Spec{Name: "agent_wake", OutputSchema: output, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{},"intent":{},"reason":{},"incident_id":{},"root_incident_id":{},"parent_incident_id":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/agents/wake"}}, func(ctx context.Context, in WakeRequest) (WakeOutput, error) {
		return provider(ctx).Wake(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
