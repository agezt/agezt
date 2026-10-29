// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// RunRequest is the run command's arguments. Every field stays raw so the
// strict readers below decide what a value means: a mistyped flag is a usage
// error, never a silent default.
type RunRequest struct {
	Intent      json.RawMessage `json:"intent,omitempty"`
	Tenant      json.RawMessage `json:"tenant,omitempty"`
	Model       json.RawMessage `json:"model,omitempty"`
	Agent       json.RawMessage `json:"agent,omitempty"`
	Images      json.RawMessage `json:"images,omitempty"`
	System      json.RawMessage `json:"system,omitempty"`
	Timeout     json.RawMessage `json:"timeout,omitempty"`
	Tools       json.RawMessage `json:"tools,omitempty"`
	MaxCost     json.RawMessage `json:"max_cost,omitempty"`
	ExecProfile json.RawMessage `json:"execution_profile,omitempty"`
	RemotePeer  json.RawMessage `json:"remote_peer,omitempty"`
	ApproveCaps json.RawMessage `json:"auto_approve_caps,omitempty"`
	Trust       json.RawMessage `json:"prompt_injection_trust,omitempty"`
	Assure      json.RawMessage `json:"assure,omitempty"`
	DryRun      json.RawMessage `json:"dry_run,omitempty"`
}

// RunOutput is the run's terminal answer. A plain run reports the answer and
// the run's own accounting; a dry run reports the resolved plan instead. Both
// share one shape, so the operation has a single declared output contract.
//
// The optional fields are pointers on purpose: a plain run always carries an
// answer (even an empty one) while a dry run never does, and the dry-run plan
// always carries a model (even an empty one) while a plain run only reports
// the model the run actually billed.
type RunOutput struct {
	// A plain run's terminal answer.
	Answer        *string `json:"answer,omitempty"`
	CorrelationID *string `json:"correlation_id,omitempty"`
	Agent         *string `json:"agent,omitempty"`
	Iters         *int    `json:"iters,omitempty"`
	SpentMC       *int64  `json:"spent_mc,omitempty"`
	Model         *string `json:"model,omitempty"`

	// A dry run's resolved plan.
	DryRun           *bool     `json:"dry_run,omitempty"`
	Intent           *string   `json:"intent,omitempty"`
	Tenant           *string   `json:"tenant,omitempty"`
	ModelSource      *string   `json:"model_source,omitempty"`
	ModelKnown       *bool     `json:"model_known,omitempty"`
	SystemSource     *string   `json:"system_source,omitempty"`
	Timeout          *string   `json:"timeout,omitempty"`
	CostCap          *string   `json:"cost_cap,omitempty"`
	ExecutionProfile *string   `json:"execution_profile,omitempty"`
	ExecutionSource  *string   `json:"execution_profile_source,omitempty"`
	WardenProfile    *string   `json:"warden_profile,omitempty"`
	ToolsMode        *string   `json:"tools_mode,omitempty"`
	Tools            *[]string `json:"tools,omitempty"`
	ToolsDropped     *[]string `json:"tools_dropped,omitempty"`
	Warnings         *[]string `json:"warnings,omitempty"`
	SupportsVision   *bool     `json:"supports_vision,omitempty"`
	SupportsTools    *bool     `json:"supports_tools,omitempty"`
	RemotePeer       *string   `json:"remote_peer,omitempty"`
}

// RunService executes one governed run.
type RunService struct{ ports RunPorts }

// NewRunService binds the run service to its host.
func NewRunService(ports RunPorts) *RunService { return &RunService{ports: ports} }

// Execute resolves the request against the kernel and runs it, streaming the
// run's own events to the caller. A dry run resolves the same way but reports
// the plan instead of executing it.
func (s *RunService) Execute(ctx context.Context, in RunRequest, emit func(*event.Event) error) (RunOutput, error) {
	resolved, err := s.resolve(ctx, in)
	if err != nil {
		return RunOutput{}, err
	}
	if resolved.dryRun {
		return s.dryRun(resolved)
	}
	return s.execute(resolved.ctx, resolved, emit)
}

// RunOperations declares the run: an audited, tenant-routed operation that
// streams the run's own event frames.
func RunOperations(provider func(context.Context) *RunService) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("run provider required")
	}
	out, err := schema.FromType(reflect.TypeFor[RunOutput](), false)
	if err != nil {
		return nil, err
	}
	frames := json.RawMessage(event.WireSchema)
	input := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{
		"intent":{},"tenant":{},"model":{},"agent":{},"images":{},"system":{},
		"timeout":{},"tools":{},"max_cost":{},"execution_profile":{},"remote_peer":{},
		"auto_approve_caps":{},"prompt_injection_trust":{},"assure":{},"dry_run":{}}}`)
	run, err := app.NewStreamingOperation(opapi.Spec{
		Name: "run", Stream: opapi.StreamEvents, OutputSchema: out, EmissionSchema: frames,
		Authz: opapi.OwnTenant, Tenancy: opapi.CallerTenant, AllowUnknownInput: true,
		InputSchema: input,
	}, func(ctx context.Context, in RunRequest, emit func(*event.Event) error) (RunOutput, error) {
		return provider(ctx).Execute(ctx, in, emit)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{run}, nil
}
