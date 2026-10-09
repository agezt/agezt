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

type SecretPolicy struct {
	Mode              string `json:"mode"`
	Scope             string `json:"scope"`
	ValuesForwarded   bool   `json:"values_forwarded"`
	MetadataForwarded bool   `json:"metadata_forwarded"`
	Valid             bool   `json:"valid"`
	Detail            string `json:"detail"`
}

// Profile is one execution profile; every field is always present, the lists
// are null when empty, and the secret policy appears only when the profile
// carries one.
type Profile struct {
	ID                 string        `json:"id"`
	Name               string        `json:"name"`
	Summary            string        `json:"summary"`
	Status             string        `json:"status"`
	Routed             bool          `json:"routed"`
	RequestedIsolation string        `json:"requested_isolation"`
	EffectiveIsolation string        `json:"effective_isolation"`
	Degraded           bool          `json:"degraded"`
	DegradeReason      string        `json:"degrade_reason"`
	Tools              []string      `json:"tools"`
	Backends           []string      `json:"backends"`
	FileSystem         string        `json:"filesystem"`
	Network            string        `json:"network"`
	Environment        string        `json:"environment"`
	Secrets            string        `json:"secrets"`
	Limits             []string      `json:"limits"`
	BrowserAccess      string        `json:"browser_access"`
	Cleanup            string        `json:"cleanup"`
	PolicyCapability   string        `json:"policy_capability"`
	Notes              []string      `json:"notes"`
	SecretPolicy       *SecretPolicy `json:"secret_policy,omitempty"`
}

func profile(p executionprofile.Profile) Profile {
	row := Profile{
		ID: p.ID, Name: p.Name, Summary: p.Summary, Status: string(p.Status), Routed: p.Routed,
		RequestedIsolation: p.RequestedIsolation, EffectiveIsolation: p.EffectiveIsolation,
		Degraded: p.Degraded, DegradeReason: p.DegradeReason,
		Tools: copyList(p.Tools), Backends: copyList(p.Backends),
		FileSystem: p.FileSystem, Network: p.Network, Environment: p.Environment, Secrets: p.Secrets,
		Limits: copyList(p.Limits), BrowserAccess: p.BrowserAccess, Cleanup: p.Cleanup,
		PolicyCapability: p.PolicyCapability, Notes: copyList(p.Notes),
	}
	if sp := p.SecretPolicy; sp != nil {
		row.SecretPolicy = &SecretPolicy{Mode: sp.Mode, Scope: sp.Scope, ValuesForwarded: sp.ValuesForwarded, MetadataForwarded: sp.MetadataForwarded, Valid: sp.Valid, Detail: sp.Detail}
	}
	return row
}

type ListRequest struct{}

type ListOutput struct {
	HostOS         string    `json:"host_os"`
	HostArch       string    `json:"host_arch"`
	Profiles       []Profile `json:"profiles"`
	Count          int       `json:"count"`
	RoutedCount    int       `json:"routed_count"`
	SupportedCount int       `json:"supported_count"`
	DegradedCount  int       `json:"degraded_count"`
}

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

// Check is one health check; every field is always present.
type Check struct {
	ID               string `json:"id"`
	ProfileID        string `json:"profile_id"`
	Status           string `json:"status"`
	Title            string `json:"title"`
	Detail           string `json:"detail"`
	Next             string `json:"next"`
	Routed           bool   `json:"routed"`
	Degraded         bool   `json:"degraded"`
	BackendAvailable bool   `json:"backend_available"`
	Backend          string `json:"backend"`
}

type CheckOutput struct {
	HostOS              string   `json:"host_os"`
	HostArch            string   `json:"host_arch"`
	Checks              []Check  `json:"checks"`
	Count               int      `json:"count"`
	OKCount             int      `json:"ok_count"`
	WarningCount        int      `json:"warning_count"`
	FailCount           int      `json:"fail_count"`
	RoutableRunProfiles []string `json:"routable_run_profiles"`
}

// Check diagnoses the inventory under the configured profile policy.
func (s *Service) Check(_ context.Context, _ CheckRequest) (CheckOutput, error) {
	report := executionprofile.Diagnose(s.ports.Inventory(), executionprofile.HealthOptions{Policy: s.ports.Policy()})
	checks := make([]Check, 0, len(report.Checks))
	for _, c := range report.Checks {
		checks = append(checks, Check{ID: c.ID, ProfileID: c.ProfileID, Status: string(c.Status), Title: c.Title, Detail: c.Detail, Next: c.Next, Routed: c.Routed, Degraded: c.Degraded, BackendAvailable: c.BackendAvailable, Backend: c.Backend})
	}
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
