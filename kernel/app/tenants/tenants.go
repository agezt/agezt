// SPDX-License-Identifier: MIT

// Package tenants owns the operator's tenant registry operations: creating,
// listing, releasing and removing isolated tenants, minting their tokens and
// summarising each tenant's run activity.
package tenants

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/tenant"
)

// ErrDisabled reports a daemon running without a tenant registry.
var ErrDisabled = errors.New("multi-tenancy is disabled (no tenant registry configured)")

// Registry is the daemon's tenant registry.
type Registry interface {
	Exists(id string) bool
	Acquire(id string, now time.Time) (*tenant.Tenant, error)
	Token(id string) (string, error)
	List() ([]tenant.Info, error)
	Release(id string) (bool, error)
	Remove(id string) (bool, error)
}

// Run is the slice of one run the activity summary counts.
type Run struct {
	SpentMicrocents                              int64
	StartedUnixMS, CompletedUnixMS, FailedUnixMS int64
	Completed, Failed                            bool
}

// Activity opens one tenant's kernel and returns a reader for its runs; an
// error means the kernel could not be opened.
type Activity func(id string) (func() ([]Run, error), error)

// Service runs the tenant operations; a nil registry means multi-tenancy is
// disabled.
type Service struct {
	registry Registry
	activity Activity
	now      func() time.Time
}

func New(registry Registry, activity Activity, now func() time.Time) *Service {
	return &Service{registry: registry, activity: activity, now: now}
}

type IDRequest struct {
	ID json.RawMessage `json:"id,omitempty"`
}

// ListRequest takes no arguments.
type ListRequest struct{}

type CreateOutput struct {
	ID      string `json:"id"`
	BaseDir string `json:"base_dir"`
	Created bool   `json:"created"`
	Token   string `json:"token"`
}

type TokenOutput struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

type Row struct {
	ID      string `json:"id"`
	BaseDir string `json:"base_dir"`
	Open    bool   `json:"open"`
}

type ListOutput struct {
	Tenants []Row `json:"tenants"`
	Count   int   `json:"count"`
}

type ReleaseOutput struct {
	Released bool `json:"released"`
}

type RemoveOutput struct {
	Removed bool `json:"removed"`
}

// StatsRow is one tenant's activity, or the error that kept it from being read.
type StatsRow struct {
	ID                 string  `json:"id"`
	Error              *string `json:"error,omitempty"`
	Runs               *int    `json:"runs,omitempty"`
	Completed          *int    `json:"completed,omitempty"`
	Failed             *int    `json:"failed,omitempty"`
	Active             *int    `json:"active,omitempty"`
	SpentMicrocents    *int64  `json:"spent_microcents,omitempty"`
	LastActivityUnixMS *int64  `json:"last_activity_unix_ms,omitempty"`
}

type StatsOutput struct {
	Tenants              []StatsRow `json:"tenants"`
	Count                int        `json:"count"`
	TotalRuns            int        `json:"total_runs"`
	TotalSpentMicrocents int64      `json:"total_spent_microcents"`
}

func rawValue(raw json.RawMessage) any {
	var v any
	if len(raw) != 0 {
		_ = json.Unmarshal(raw, &v)
	}
	return v
}

// requiredID reads a strict string id; a blank one is required, but the value
// passes untrimmed for the registry to validate.
func requiredID(raw json.RawMessage) (string, error) {
	var id string
	if len(raw) != 0 {
		s, ok := rawValue(raw).(string)
		if !ok {
			return "", errors.New("args.id must be a string")
		}
		id = s
	}
	if strings.TrimSpace(id) == "" {
		return "", fmt.Errorf("args.id required")
	}
	return id, nil
}

// Create opens (and on first use creates) a tenant and returns its token.
func (s *Service) Create(_ context.Context, in IDRequest) (CreateOutput, error) {
	if s.registry == nil {
		return CreateOutput{}, ErrDisabled
	}
	id, err := requiredID(in.ID)
	if err != nil {
		return CreateOutput{}, err
	}
	existed := s.registry.Exists(id)
	t, err := s.registry.Acquire(id, s.now())
	if err != nil {
		return CreateOutput{}, err
	}
	return CreateOutput{ID: t.ID, BaseDir: t.BaseDir, Created: !existed, Token: t.Token}, nil
}

// Token returns an existing tenant's credential.
func (s *Service) Token(_ context.Context, in IDRequest) (TokenOutput, error) {
	if s.registry == nil {
		return TokenOutput{}, ErrDisabled
	}
	id, err := requiredID(in.ID)
	if err != nil {
		return TokenOutput{}, err
	}
	token, err := s.registry.Token(id)
	if err != nil {
		return TokenOutput{}, err
	}
	return TokenOutput{ID: id, Token: token}, nil
}

// List reports every tenant on disk and whether its kernel is open.
func (s *Service) List(context.Context, ListRequest) (ListOutput, error) {
	if s.registry == nil {
		return ListOutput{}, ErrDisabled
	}
	infos, err := s.registry.List()
	if err != nil {
		return ListOutput{}, err
	}
	out := ListOutput{Tenants: make([]Row, 0, len(infos)), Count: len(infos)}
	for _, i := range infos {
		out.Tenants = append(out.Tenants, Row{ID: i.ID, BaseDir: i.BaseDir, Open: i.Open})
	}
	return out, nil
}

// Release closes a tenant's kernel, keeping its data.
func (s *Service) Release(_ context.Context, in IDRequest) (ReleaseOutput, error) {
	if s.registry == nil {
		return ReleaseOutput{}, ErrDisabled
	}
	id, err := requiredID(in.ID)
	if err != nil {
		return ReleaseOutput{}, err
	}
	released, err := s.registry.Release(id)
	if err != nil {
		return ReleaseOutput{}, err
	}
	return ReleaseOutput{Released: released}, nil
}

// Remove deletes a tenant and its data.
func (s *Service) Remove(_ context.Context, in IDRequest) (RemoveOutput, error) {
	if s.registry == nil {
		return RemoveOutput{}, ErrDisabled
	}
	id, err := requiredID(in.ID)
	if err != nil {
		return RemoveOutput{}, err
	}
	removed, err := s.registry.Remove(id)
	if err != nil {
		return RemoveOutput{}, err
	}
	return RemoveOutput{Removed: removed}, nil
}

// Stats summarises every tenant's runs: totals, outcomes, spend and the latest
// activity. A tenant whose kernel was closed is released again afterwards, so
// the summary leaves residency as it found it.
func (s *Service) Stats(context.Context, ListRequest) (StatsOutput, error) {
	if s.registry == nil {
		return StatsOutput{}, ErrDisabled
	}
	infos, err := s.registry.List()
	if err != nil {
		return StatsOutput{}, err
	}
	out := StatsOutput{Tenants: make([]StatsRow, 0, len(infos))}
	for _, info := range infos {
		read, err := s.activity(info.ID)
		if err != nil {
			out.Tenants = append(out.Tenants, StatsRow{ID: info.ID, Error: new(err.Error())})
			continue
		}
		runs, err := read()
		if err != nil {
			out.Tenants = append(out.Tenants, StatsRow{ID: info.ID, Error: new(err.Error())})
			if !info.Open {
				_, _ = s.registry.Release(info.ID)
			}
			continue
		}
		var total, completed, failed, active int
		var spent, last int64
		for _, r := range runs {
			total++
			spent += r.SpentMicrocents
			last = max(last, r.StartedUnixMS, r.CompletedUnixMS, r.FailedUnixMS)
			switch {
			case r.Completed:
				completed++
			case r.Failed:
				failed++
			default:
				active++
			}
		}
		out.Tenants = append(out.Tenants, StatsRow{ID: info.ID, Runs: new(total), Completed: new(completed), Failed: new(failed), Active: new(active), SpentMicrocents: new(spent), LastActivityUnixMS: new(last)})
		out.TotalRuns += total
		out.TotalSpentMicrocents += spent
		if !info.Open {
			_, _ = s.registry.Release(info.ID)
		}
	}
	out.Count = len(out.Tenants)
	return out, nil
}

func bind[I, O any](ops *[]app.Operation, spec opapi.Spec, handler func(context.Context, I) (O, error)) error {
	output, err := schema.FromType(reflect.TypeFor[O](), false)
	if err != nil {
		return err
	}
	spec.OutputSchema, spec.AllowUnknownInput = output, true
	op, err := app.NewOperation(spec, handler)
	if err != nil {
		return err
	}
	*ops = append(*ops, op)
	return nil
}

// Operations declares the six operator-only tenant operations. The four
// registry changes are audited. The activity summary stays caller-tenant
// routed, so a named tenant is still resolved first.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("tenants provider required")
	}
	idSchema := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"id":{}}}`)
	none := json.RawMessage(`{"type":"object","additionalProperties":true}`)
	var ops []app.Operation
	for _, b := range []func() error{
		func() error {
			return bind(&ops, opapi.Spec{Name: "tenant_create", InputSchema: idSchema}, func(ctx context.Context, in IDRequest) (CreateOutput, error) {
				return provider(ctx).Create(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "tenant_list", ReadOnly: true, InputSchema: none}, func(ctx context.Context, in ListRequest) (ListOutput, error) {
				return provider(ctx).List(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "tenant_release", InputSchema: idSchema}, func(ctx context.Context, in IDRequest) (ReleaseOutput, error) {
				return provider(ctx).Release(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "tenant_remove", InputSchema: idSchema}, func(ctx context.Context, in IDRequest) (RemoveOutput, error) {
				return provider(ctx).Remove(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "tenant_token", InputSchema: idSchema}, func(ctx context.Context, in IDRequest) (TokenOutput, error) {
				return provider(ctx).Token(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "tenant_stats", ReadOnly: true, Tenancy: opapi.CallerTenant, InputSchema: none}, func(ctx context.Context, in ListRequest) (StatsOutput, error) {
				return provider(ctx).Stats(ctx, in)
			})
		},
	} {
		if err := b(); err != nil {
			return nil, err
		}
	}
	return ops, nil
}
