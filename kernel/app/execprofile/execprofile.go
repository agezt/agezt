// SPDX-License-Identifier: MIT

// Package execprofile reports the execution profiles a kernel can route work
// to: the inventory, one profile, and the health check of each profile and
// policy, built from the kernel's tools and warden and the host's remote
// backend configuration.
package execprofile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// Ports builds the inventory for the routed kernel and reads the profile
// policy; both are read on every call so configuration changes show at once.
type Ports struct {
	Inventory func() executionprofile.Inventory
	Policy    func() executionprofile.ProfilePolicy
}

type Service struct{ ports Ports }

func New(ports Ports) *Service { return &Service{ports: ports} }

// copyList keeps the wire's null for an empty list.
func copyList(in []string) []string { return append([]string(nil), in...) }

// The rows are the module's rows — one declaration per row, published as they
// are. The application does not rebuild them field by field any more; it only
// keeps the list rule (an empty list is null on the wire).
type (
	SecretPolicy = executionprofile.SecretPolicy
	Profile      = executionprofile.Profile
	Check        = executionprofile.HealthCheck
	ListOutput   = executionprofile.Inventory
	CheckOutput  = executionprofile.HealthReport
)

// profile hands the module's row over as it is; the list rule is the only
// thing the application keeps: an empty list is null on the wire.
func profile(p executionprofile.Profile) Profile {
	p.Tools = copyList(p.Tools)
	p.Backends = copyList(p.Backends)
	p.Limits = copyList(p.Limits)
	p.Notes = copyList(p.Notes)
	return p
}

type ListRequest struct{}

func (s *Service) List(_ context.Context, _ ListRequest) (ListOutput, error) {
	inv := s.ports.Inventory()
	rows := make([]Profile, 0, len(inv.Profiles))
	for _, p := range inv.Profiles {
		rows = append(rows, profile(p))
	}
	return ListOutput{HostOS: inv.HostOS, HostArch: inv.HostArch, Profiles: rows, Count: inv.Count, RoutedCount: inv.RoutedCount, SupportedCount: inv.SupportedCount, DegradedCount: inv.DegradedCount}, nil
}

type ShowRequest struct {
	ID json.RawMessage `json:"id,omitempty"`
}

type ShowOutput struct {
	Profile  Profile `json:"profile"`
	HostOS   string  `json:"host_os"`
	HostArch string  `json:"host_arch"`
}

// requiredID is a string argument that must not be blank; an absent id is
// blank, any other type is refused.
func requiredID(raw json.RawMessage) (string, error) {
	var v any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &v)
		if _, ok := v.(string); !ok {
			return "", errors.New("args.id must be a string")
		}
	}
	id, _ := v.(string)
	if strings.TrimSpace(id) == "" {
		return "", errors.New("args.id required")
	}
	return strings.TrimSpace(id), nil
}

func (s *Service) Show(_ context.Context, in ShowRequest) (ShowOutput, error) {
	id, err := requiredID(in.ID)
	if err != nil {
		return ShowOutput{}, err
	}
	inv := s.ports.Inventory()
	p, ok := inv.Find(id)
	if !ok {
		return ShowOutput{}, errors.New("unknown execution profile: " + id)
	}
	return ShowOutput{Profile: profile(p), HostOS: inv.HostOS, HostArch: inv.HostArch}, nil
}

type CheckRequest struct{}

func (s *Service) Check(_ context.Context, _ CheckRequest) (CheckOutput, error) {
	report := executionprofile.Diagnose(s.ports.Inventory(), executionprofile.HealthOptions{Policy: s.ports.Policy()})
	checks := make([]Check, 0, len(report.Checks))
	checks = append(checks, report.Checks...)
	return CheckOutput{HostOS: report.HostOS, HostArch: report.HostArch, Checks: checks, Count: report.Count, OKCount: report.OKCount, WarningCount: report.WarningCount, FailCount: report.FailCount, RoutableRunProfiles: copyList(report.RoutableRunProfiles)}, nil
}

// Operations declares the three read-only, unaudited reads, the inventory and
// the check on their Web UI routes. Each runs against the caller's kernel: a tenant token reads its own, the operator reads the
// primary or a named tenant.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("execution profile provider required")
	}
	var ops []app.Operation
	for _, op := range []struct {
		name, path string
		output     reflect.Type
		input      string
	}{
		{"execution_profiles", "/api/execution_profiles", reflect.TypeFor[ListOutput](), ""},
		{"execution_profile_show", "", reflect.TypeFor[ShowOutput](), `{"type":"object","additionalProperties":true,"properties":{"id":{}}}`},
		{"execution_profile_check", "/api/execution_profile_check", reflect.TypeFor[CheckOutput](), ""},
	} {
		out, err := schema.FromType(op.output, false)
		if err != nil {
			return nil, err
		}
		spec := opapi.Spec{Name: op.name, ReadOnly: true, OutputSchema: out, Authz: opapi.OwnTenant, Tenancy: opapi.CallerTenant, AllowUnknownInput: true}
		if op.path != "" {
			spec.HTTP = opapi.HTTP{Method: "GET", Path: op.path}
		}
		if op.input != "" {
			spec.InputSchema = json.RawMessage(op.input)
		}
		var operation app.Operation
		switch op.name {
		case "execution_profiles":
			operation, err = app.NewOperation(spec, func(ctx context.Context, in ListRequest) (ListOutput, error) { return provider(ctx).List(ctx, in) })
		case "execution_profile_show":
			operation, err = app.NewOperation(spec, func(ctx context.Context, in ShowRequest) (ShowOutput, error) { return provider(ctx).Show(ctx, in) })
		default:
			operation, err = app.NewOperation(spec, func(ctx context.Context, in CheckRequest) (CheckOutput, error) { return provider(ctx).Check(ctx, in) })
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op.name, err)
		}
		ops = append(ops, operation)
	}
	return ops, nil
}
