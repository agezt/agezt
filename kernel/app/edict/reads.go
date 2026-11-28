// SPDX-License-Identifier: MIT

package edict

import (
	"context"
	"errors"
	"reflect"
	"sort"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	core "github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// Engine is the routed kernel's policy engine.
type Engine interface {
	Levels() map[core.Capability]core.TrustLevel
	HardDenyRules() []core.HardDenyRule
	AskPolicy() core.AskPolicy
	Decide(core.Capability, string) core.Outcome
}

// HardDenyRow lists a rule; a nil applies_to means every capability.
type HardDenyRow struct {
	Name      string   `json:"name"`
	Substring string   `json:"substring"`
	AppliesTo []string `json:"applies_to"`
}

type DenyRuleRow struct {
	HardDenyRow
	Removable bool `json:"removable"`
}

type ShowOutput struct {
	AskPolicy string            `json:"ask_policy"`
	Levels    map[string]string `json:"levels"`
	HardDeny  []HardDenyRow     `json:"hard_deny"`
}

type DenyListOutput struct {
	Rules []DenyRuleRow `json:"rules"`
}

type TestOutput struct {
	Decision         string `json:"decision"`
	Capability       string `json:"capability"`
	Level            string `json:"level"`
	Reason           string `json:"reason"`
	HardDenied       bool   `json:"hard_denied"`
	HardDenyRule     string `json:"hard_deny_rule"`
	WouldAsk         bool   `json:"would_ask"`
	RequiresApproval bool   `json:"requires_approval"`
}

// sortedRules returns the rules ordered by name.
func sortedRules(rules []core.HardDenyRule) []core.HardDenyRule {
	sort.Slice(rules, func(i, j int) bool { return rules[i].Name < rules[j].Name })
	return rules
}

func hardDenyRow(r core.HardDenyRule) HardDenyRow {
	var applies []string
	for _, c := range r.AppliesTo {
		applies = append(applies, string(c))
	}
	return HardDenyRow{Name: r.Name, Substring: r.Substring, AppliesTo: applies}
}

// Service reads the routed kernel's policy engine.
type Service struct{ engine Engine }

func New(engine Engine) *Service { return &Service{engine: engine} }

// Show reports the ask policy, every capability's trust level and the
// hard-deny rules sorted by name.
func (s *Service) Show(_ context.Context, _ opapi.EdictTenantRequest) (ShowOutput, error) {
	out := ShowOutput{AskPolicy: s.engine.AskPolicy().String(), Levels: map[string]string{}, HardDeny: []HardDenyRow{}}
	for c, level := range s.engine.Levels() {
		out.Levels[string(c)] = level.String()
	}
	for _, r := range sortedRules(s.engine.HardDenyRules()) {
		out.HardDeny = append(out.HardDeny, hardDenyRow(r))
	}
	return out, nil
}

// DenyList lists the hard-deny rules by name, marking those added at runtime as
// removable.
func (s *Service) DenyList(_ context.Context, _ opapi.EdictTenantRequest) (DenyListOutput, error) {
	out := DenyListOutput{Rules: []DenyRuleRow{}}
	for _, r := range sortedRules(s.engine.HardDenyRules()) {
		out.Rules = append(out.Rules, DenyRuleRow{HardDenyRow: hardDenyRow(r), Removable: core.IsRuntimeRule(r.Name)})
	}
	return out, nil
}

// Test dry-runs one policy decision in the vocabulary the runtime journals. The
// capability is required; an empty input is a valid probe.
func (s *Service) Test(_ context.Context, in opapi.EdictTestRequest) (TestOutput, error) {
	if in.Capability == "" {
		return TestOutput{}, errors.New("args.capability required")
	}
	o := s.engine.Decide(core.Capability(in.Capability), in.Input)
	return TestOutput{Decision: string(o.Decision), Capability: string(o.Capability), Level: o.Level.String(), Reason: o.Reason, HardDenied: o.HardDenied, HardDenyRule: o.HardDenyRule, WouldAsk: o.WouldAsk, RequiresApproval: o.RequiresApproval}, nil
}

// bind declares a tenant-owned operation routed to the caller's tenant kernel.
func bind[I, O any](ops *[]app.Operation, spec opapi.Spec, handler func(context.Context, I) (O, error)) error {
	return bindAs(ops, spec, opapi.OwnTenant, handler)
}

// bindAs declares an operation routed to the caller's tenant kernel under authz.
func bindAs[I, O any](ops *[]app.Operation, spec opapi.Spec, authz opapi.Authz, handler func(context.Context, I) (O, error)) error {
	output, err := schema.FromType(reflect.TypeFor[O](), false)
	if err != nil {
		return err
	}
	spec.OutputSchema, spec.Authz, spec.Tenancy, spec.AllowUnknownInput = output, authz, opapi.CallerTenant, true
	op, err := app.NewOperation(spec, handler)
	if err != nil {
		return err
	}
	*ops = append(*ops, op)
	return nil
}

// Operations declares the three unaudited policy reads. Each routes to the
// caller's tenant kernel, so a tenant inspects its own policy.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("edict provider required")
	}
	var ops []app.Operation
	for _, b := range []func() error{
		func() error {
			return bind(&ops, opapi.Spec{Name: opapi.EdictShow, ReadOnly: true}, func(ctx context.Context, in opapi.EdictTenantRequest) (ShowOutput, error) {
				return provider(ctx).Show(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: opapi.EdictDenyList, ReadOnly: true}, func(ctx context.Context, in opapi.EdictTenantRequest) (DenyListOutput, error) {
				return provider(ctx).DenyList(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: opapi.EdictTest, ReadOnly: true}, func(ctx context.Context, in opapi.EdictTestRequest) (TestOutput, error) {
				return provider(ctx).Test(ctx, in)
			})
		},
	} {
		if err := b(); err != nil {
			return nil, err
		}
	}
	return ops, nil
}
