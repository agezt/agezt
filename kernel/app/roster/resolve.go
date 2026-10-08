// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/roster"
)

// ResolveRequest keeps every argument raw: ref is a strict string; everything
// else is read leniently (a non-string is empty, a non-array chain is absent).
type ResolveRequest struct {
	Ref              json.RawMessage `json:"ref,omitempty"`
	Resolution       json.RawMessage `json:"resolution,omitempty"`
	Summary          json.RawMessage `json:"summary,omitempty"`
	DelegateTo       json.RawMessage `json:"delegate_to,omitempty"`
	TaskType         json.RawMessage `json:"task_type,omitempty"`
	TaskModelChain   json.RawMessage `json:"task_model_chain,omitempty"`
	IncidentID       json.RawMessage `json:"incident_id,omitempty"`
	RootIncidentID   json.RawMessage `json:"root_incident_id,omitempty"`
	ParentIncidentID json.RawMessage `json:"parent_incident_id,omitempty"`
}

type ResolveOutput struct {
	Applied       bool   `json:"applied"`
	Agent         string `json:"agent"`
	Resolution    string `json:"resolution"`
	CorrelationID string `json:"correlation_id"`
}

// RoutingChainResult is what forcing a task-type model chain changed.
type RoutingChainResult struct {
	TaskType string
	Chain    []string
	Previous []string
}

// ResolvePorts are the native effects an incident resolution may use.
type ResolvePorts struct {
	Get            func(string) (core.Profile, bool)
	NewCorrelation func() string
	Publish        func(subject, corr string, payload map[string]any)
	Pause          func(slug string) error
	Retire         func(slug, reason string) error
	// HelpRequest posts an operator help request to target on the shared board
	// and returns the message id.
	HelpRequest func(target, text string) (string, error)
	// ExhaustedChain is the latest chain the doctor found exhausted for this
	// agent, task type and incident lineage.
	ExhaustedChain func(slug string, lineage IncidentLineage, taskType string) []string
	// ForceGeneration is the latest applied force-chain generation.
	ForceGeneration func(slug, taskType string) int
	ApplyChain      func(slug, taskType string, chain []string, reason string) (RoutingChainResult, error)
}

// ResolveService applies an operator's resolution to an agent incident and
// journals the request and its outcome under one correlation.
type ResolveService struct{ ports ResolvePorts }

func NewResolve(ports ResolvePorts) *ResolveService { return &ResolveService{ports: ports} }

type appliedResolution struct {
	delegateTo                     string
	messageID                      string
	taskType                       string
	taskModelChain                 []string
	previousTaskModelChain         []string
	routingForceGeneration         int
	previousRoutingForceGeneration int
}

func (r appliedResolution) addTo(payload map[string]any) {
	if r.delegateTo != "" {
		payload["delegate_to"] = r.delegateTo
	}
	if r.messageID != "" {
		payload["message_id"] = r.messageID
	}
	if r.taskType != "" {
		payload["routing_task_type"] = r.taskType
	}
	if len(r.taskModelChain) > 0 {
		payload["routing_task_model_chain"] = r.taskModelChain
	}
	if len(r.previousTaskModelChain) > 0 {
		payload["previous_routing_task_model_chain"] = r.previousTaskModelChain
	}
	if r.routingForceGeneration > 0 {
		payload["routing_force_generation"] = r.routingForceGeneration
	}
	if r.previousRoutingForceGeneration > 0 {
		payload["previous_routing_force_generation"] = r.previousRoutingForceGeneration
	}
}

// rawChain is the task_model_chain argument when it is a JSON array.
func rawChain(raw json.RawMessage) []any {
	v, _ := rawValue(raw)
	items, _ := v.([]any)
	return items
}

// normalizeTaskModelChain keeps the trimmed, non-empty string models. A
// non-empty input always yields a non-nil slice.
func normalizeTaskModelChain(raw []any) []string {
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if v, ok := item.(string); ok {
			if v = strings.TrimSpace(v); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

func equalChains(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(strings.TrimSpace(a[i]), strings.TrimSpace(b[i])) {
			return false
		}
	}
	return true
}

// validateDelegateTarget refuses a delegation back to the agent or its owner,
// or to a missing, retired or managed agent.
func validateDelegateTarget(p core.Profile, target string, get func(string) (core.Profile, bool)) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return fmt.Errorf("delegated resolution requires delegate_to")
	}
	if strings.EqualFold(target, strings.TrimSpace(p.Slug)) {
		return fmt.Errorf("delegated resolution points back to the root agent %s", p.Slug)
	}
	if owner := firstNonEmpty(p.ParentAgent, p.OwnerAgent); owner != "" && strings.EqualFold(target, owner) {
		return fmt.Errorf("delegated resolution points back to the current owner %s", owner)
	}
	dst, ok := get(target)
	if !ok {
		return fmt.Errorf("delegated resolution target %s does not exist", target)
	}
	if dst.Retired {
		return fmt.Errorf("delegated resolution target %s is retired", dst.Slug)
	}
	if !dst.AllowsDirectCall() {
		return fmt.Errorf("delegated resolution target %s is a managed sub-agent", dst.Slug)
	}
	return nil
}

func (s *ResolveService) apply(p core.Profile, resolution, summary string, in ResolveRequest, lineage IncidentLineage) (appliedResolution, error) {
	switch resolution {
	case "paused":
		if p.Retired {
			return appliedResolution{}, fmt.Errorf("agent %s is retired — revive it first", p.Slug)
		}
		return appliedResolution{}, s.ports.Pause(p.Slug)
	case "retired":
		reason := summary
		if reason == "" {
			reason = "retired by operator incident resolution"
		}
		return appliedResolution{}, s.ports.Retire(p.Slug, reason)
	case "delegated":
		target := lenientString(in.DelegateTo)
		if err := validateDelegateTarget(p, target, s.ports.Get); err != nil {
			return appliedResolution{}, err
		}
		text := strings.TrimSpace(summary)
		if text == "" {
			text = "Operator delegated this incident for ownership review."
		}
		id, err := s.ports.HelpRequest(target, text)
		if err != nil {
			return appliedResolution{}, err
		}
		return appliedResolution{delegateTo: target, messageID: strings.TrimSpace(id)}, nil
	case "force_chain":
		taskType := lenientString(in.TaskType)
		chain := normalizeTaskModelChain(rawChain(in.TaskModelChain))
		if taskType == "" || len(chain) == 0 {
			return appliedResolution{}, fmt.Errorf("force_chain resolution requires task_type and task_model_chain")
		}
		if exhausted := s.ports.ExhaustedChain(p.Slug, lineage, taskType); len(exhausted) > 0 && equalChains(exhausted, chain) {
			return appliedResolution{}, fmt.Errorf("force_chain resolution must choose a new chain for exhausted routing policy")
		}
		prevGen := s.ports.ForceGeneration(p.Slug, taskType)
		res, err := s.ports.ApplyChain(p.Slug, taskType, chain, summary)
		if err != nil {
			return appliedResolution{}, err
		}
		return appliedResolution{
			taskType:                       firstNonEmpty(res.TaskType, taskType),
			taskModelChain:                 append([]string(nil), strutil.FirstNonEmptySlice(res.Chain, chain)...),
			previousTaskModelChain:         append([]string(nil), res.Previous...),
			routingForceGeneration:         prevGen + 1,
			previousRoutingForceGeneration: prevGen,
		}, nil
	default:
		return appliedResolution{}, nil
	}
}

func (s *ResolveService) Resolve(_ context.Context, in ResolveRequest) (ResolveOutput, error) {
	ref, err := RefPageRequest{Ref: in.Ref}.ref()
	if err != nil {
		return ResolveOutput{}, err
	}
	p, ok := s.ports.Get(ref)
	if !ok {
		return ResolveOutput{}, errors.New("unknown agent: " + ref)
	}
	resolution := lenientString(in.Resolution)
	switch resolution {
	case "paused", "retired", "delegated", "force_chain":
	default:
		return ResolveOutput{}, errors.New("args.resolution must be paused, retired, delegated, or force_chain")
	}
	summary := lenientString(in.Summary)
	lineage := incidentLineage(in.IncidentID, in.RootIncidentID, in.ParentIncidentID)
	corr := s.ports.NewCorrelation()
	payload := func(phase string) map[string]any {
		return map[string]any{
			"phase":              phase,
			"agent":              p.Slug,
			"resolution":         resolution,
			"resolution_summary": summary,
			"incident_id":        lineage.IncidentID,
			"root_incident_id":   lineage.RootIncidentID,
			"parent_incident_id": lineage.ParentIncidentID,
		}
	}
	requested := payload("requested")
	if delegateTo := lenientString(in.DelegateTo); delegateTo != "" {
		requested["delegate_to"] = delegateTo
	}
	if taskType := lenientString(in.TaskType); taskType != "" {
		requested["routing_task_type"] = taskType
	}
	if chain := rawChain(in.TaskModelChain); len(chain) > 0 {
		requested["routing_task_model_chain"] = normalizeTaskModelChain(chain)
	}
	s.ports.Publish("agent.resolve", corr, requested)

	result, err := s.apply(p, resolution, summary, in, lineage)
	if err != nil {
		failed := payload("failed")
		failed["reason"] = err.Error()
		result.addTo(failed)
		s.ports.Publish("agent.resolve", corr, failed)
		return ResolveOutput{}, err
	}
	completed := payload("completed")
	result.addTo(completed)
	s.ports.Publish("agent.resolve", corr, completed)
	return ResolveOutput{Applied: true, Agent: p.Slug, Resolution: resolution, CorrelationID: corr}, nil
}

func ResolveOperations(provider func(context.Context) *ResolveService) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("roster resolve provider required")
	}
	output, err := schema.FromType(reflect.TypeFor[ResolveOutput](), false)
	if err != nil {
		return nil, err
	}
	op, err := app.NewOperation(opapi.Spec{Name: "agent_resolve", OutputSchema: output, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{},"resolution":{},"summary":{},"delegate_to":{},"task_type":{},"task_model_chain":{},"incident_id":{},"root_incident_id":{},"parent_incident_id":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/agents/resolve"}}, func(ctx context.Context, in ResolveRequest) (ResolveOutput, error) {
		return provider(ctx).Resolve(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
