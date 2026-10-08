// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/roster"
)

// ApplyMutableProfilePatch copies only the fields the caller explicitly
// provided, so a partial edit keeps everything it did not mention.
func ApplyMutableProfilePatch(dst *core.Profile, in core.Profile, provided map[string]bool) {
	if provided["name"] {
		dst.Name = in.Name
	}
	if provided["soul"] {
		dst.Soul = in.Soul
	}
	if provided["instructions"] {
		dst.Instructions = in.Instructions
	}
	if provided["model"] {
		dst.Model = in.Model
	}
	if provided["fallbacks"] {
		dst.Fallbacks = in.Fallbacks
	}
	if provided["task_type"] {
		dst.TaskType = in.TaskType
	}
	if provided["max_cost_mc"] {
		dst.MaxCostMc = in.MaxCostMc
	}
	if provided["max_daily_mc"] {
		dst.MaxDailyMc = in.MaxDailyMc
	}
	if provided["memory_scope"] {
		dst.MemoryScope = in.MemoryScope
	}
	if provided["workdir"] {
		dst.Workdir = in.Workdir
	}
	if provided["owner_agent"] {
		dst.OwnerAgent = in.OwnerAgent
	}
	if provided["parent_agent"] {
		dst.ParentAgent = in.ParentAgent
	}
	if provided["direct_callable"] {
		dst.DirectCallable = in.DirectCallable
	}
	if provided["retry_policy"] {
		dst.RetryPolicy = in.RetryPolicy
	}
	if provided["health_policy"] {
		dst.HealthPolicy = in.HealthPolicy
	}
	if provided["self_repair"] {
		dst.SelfRepairPolicy = in.SelfRepairPolicy
	}
	if provided["noise_policy"] {
		dst.NoisePolicy = in.NoisePolicy
	}
	if provided["tool_allow"] {
		dst.ToolAllow = in.ToolAllow
	}
	if provided["tool_deny"] {
		dst.ToolDeny = in.ToolDeny
	}
	if provided["trust_ceiling"] {
		dst.TrustCeiling = in.TrustCeiling
	}
	if provided["execution_profile"] {
		dst.ExecutionProfile = strings.TrimSpace(in.ExecutionProfile)
	}
	if provided["config_overrides"] {
		dst.ConfigOverrides = in.ConfigOverrides
	}
	if provided["lifecycle"] {
		dst.Lifecycle = in.Lifecycle
	}
	if provided["tasklist"] {
		dst.TaskList = in.TaskList
	}
	if provided["description"] {
		dst.Description = in.Description
	}
}

// NormalizeProfileKind maps the client-facing kind "subagent" onto the
// managed (not directly callable) flag.
func NormalizeProfileKind(raw []byte, p *core.Profile) {
	var meta struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return
	}
	if strings.EqualFold(strings.TrimSpace(meta.Kind), "subagent") {
		no := false
		p.DirectCallable = &no
	}
}

// ValidateHierarchyRefs rejects owner/parent references to the agent itself,
// to unknown agents or to retired agents. Owner is checked before parent so a
// profile with two bad references always reports the same one.
func ValidateHierarchyRefs(p core.Profile, get func(string) (core.Profile, bool)) error {
	for _, ref := range []struct{ label, value string }{{"owner_agent", p.OwnerAgent}, {"parent_agent", p.ParentAgent}} {
		value := strings.TrimSpace(ref.value)
		if value == "" {
			continue
		}
		if strings.EqualFold(value, strings.TrimSpace(p.Slug)) {
			return fmt.Errorf("roster: %s cannot point to the same agent", ref.label)
		}
		target, ok := get(value)
		if !ok {
			return fmt.Errorf("roster: %s %q does not exist", ref.label, value)
		}
		if target.Retired {
			return fmt.Errorf("roster: %s %q is retired", ref.label, value)
		}
	}
	return nil
}

type AddRequest struct {
	Profile json.RawMessage `json:"profile,omitempty"`
}

type EditRequest struct {
	Ref     json.RawMessage `json:"ref,omitempty"`
	Profile json.RawMessage `json:"profile,omitempty"`
}

type ProfileWriteOutput struct {
	Profile ProfileOutput `json:"profile"`
}

// ProfileWriteService creates and edits roster profiles through the kernel's
// journaled store, validating hierarchy references against the live roster.
type ProfileWriteService struct {
	get        func(string) (core.Profile, bool)
	add        func(core.Profile) (core.Profile, error)
	update     func(string, func(*core.Profile)) (core.Profile, bool, error)
	invalidate func()
}

func NewProfileWrite(get func(string) (core.Profile, bool), add func(core.Profile) (core.Profile, error), update func(string, func(*core.Profile)) (core.Profile, bool, error), invalidate func()) *ProfileWriteService {
	return &ProfileWriteService{get: get, add: add, update: update, invalidate: invalidate}
}

func profileWriteOutput(p core.Profile) ProfileWriteOutput {
	return ProfileWriteOutput{Profile: ProfileOutput{Profile: p, Kind: p.Kind(), Managed: !p.AllowsDirectCall()}}
}

func (s *ProfileWriteService) Add(_ context.Context, in AddRequest) (ProfileWriteOutput, error) {
	if len(in.Profile) == 0 {
		return ProfileWriteOutput{}, errors.New("args.profile required")
	}
	var p core.Profile
	if err := json.Unmarshal(in.Profile, &p); err != nil {
		return ProfileWriteOutput{}, errors.New("args.profile: " + err.Error())
	}
	NormalizeProfileKind(in.Profile, &p)
	p.System = false // System is kernel-owned (set only by guardian seeding); never accept it from a client (M961)
	if err := ValidateHierarchyRefs(p, s.get); err != nil {
		return ProfileWriteOutput{}, err
	}
	saved, err := s.add(p)
	if err != nil {
		return ProfileWriteOutput{}, err
	}
	s.invalidate()
	return profileWriteOutput(saved), nil
}

func (s *ProfileWriteService) Edit(_ context.Context, in EditRequest) (ProfileWriteOutput, error) {
	ref, err := RefPageRequest{Ref: in.Ref}.ref()
	if err != nil {
		return ProfileWriteOutput{}, err
	}
	if len(in.Profile) == 0 {
		return ProfileWriteOutput{}, errors.New("args.profile required")
	}
	// The top-level keys the caller sent decide which fields change, as opposed
	// to zero values from omitted fields.
	provided := map[string]bool{}
	var keys map[string]json.RawMessage
	if json.Unmarshal(in.Profile, &keys) == nil {
		for k := range keys {
			provided[k] = true
		}
	}
	var patch core.Profile
	if err := json.Unmarshal(in.Profile, &patch); err != nil {
		return ProfileWriteOutput{}, errors.New("args.profile: " + err.Error())
	}
	NormalizeProfileKind(in.Profile, &patch)
	// kind "subagent" sets DirectCallable=false; apply it like an explicit field.
	if provided["kind"] && patch.DirectCallable != nil && !*patch.DirectCallable {
		provided["direct_callable"] = true
	}
	current, ok := s.get(ref)
	if !ok {
		return ProfileWriteOutput{}, errors.New("unknown agent: " + ref)
	}
	candidate := current
	ApplyMutableProfilePatch(&candidate, patch, provided)
	if err := ValidateHierarchyRefs(candidate, s.get); err != nil {
		return ProfileWriteOutput{}, err
	}
	p, found, err := s.update(ref, func(dst *core.Profile) { ApplyMutableProfilePatch(dst, patch, provided) })
	if err != nil {
		return ProfileWriteOutput{}, err
	}
	if !found {
		return ProfileWriteOutput{}, errors.New("unknown agent: " + ref)
	}
	s.invalidate()
	return profileWriteOutput(p), nil
}

// profileWriteSchema mirrors ProfileWriteOutput with the marshaler-free view.
type profileWriteSchema struct {
	Profile profileSchema `json:"profile"`
}

func ProfileWriteOperations(provider func(context.Context) *ProfileWriteService) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("roster profile write provider required")
	}
	output, err := schema.FromType(reflect.TypeFor[profileWriteSchema](), false)
	if err != nil {
		return nil, err
	}
	add, err := app.NewOperation(opapi.Spec{Name: "agent_add", OutputSchema: output, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"profile":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/agents/add"}}, func(ctx context.Context, in AddRequest) (ProfileWriteOutput, error) {
		return provider(ctx).Add(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	edit, err := app.NewOperation(opapi.Spec{Name: "agent_edit", OutputSchema: output, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{},"profile":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/agents/edit"}}, func(ctx context.Context, in EditRequest) (ProfileWriteOutput, error) {
		return provider(ctx).Edit(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{add, edit}, nil
}
