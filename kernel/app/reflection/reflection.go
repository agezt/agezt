// SPDX-License-Identifier: MIT

// Package reflection triggers and reads the kernel's reflection passes, the
// path behind `agt reflect`: a pass folds the journal into observations,
// applies world-model decay, derives advisory proposals and journals its
// report under the pass's correlation, so the decay is explainable via
// `agt why`.
package reflection

import (
	"context"
	"errors"
	"reflect"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	kreflect "github.com/agezt/agezt/kernel/reflect"
)

// Engine is the primary kernel's reflection engine.
type Engine interface {
	Reflect(ctx context.Context, corr string) (kreflect.Report, error)
	Latest() (kreflect.Report, bool)
}

// Service runs passes on one engine, minting each pass's correlation.
type Service struct {
	engine Engine
	mint   func() string
}

func New(engine Engine, mint func() string) *Service { return &Service{engine: engine, mint: mint} }

type RunRequest struct{}

// RunOutput is the pass's report with the correlation it was journaled under.
type RunOutput struct {
	kreflect.Report
	CorrelationID string `json:"correlation_id"`
}

// Run triggers one pass under a fresh "reflect-" correlation. The pass is
// offline and deterministic, so it runs to completion once admitted.
func (s *Service) Run(_ context.Context, _ RunRequest) (RunOutput, error) {
	corr := "reflect-" + s.mint()
	rep, err := s.engine.Reflect(context.Background(), corr)
	if err != nil {
		return RunOutput{}, err
	}
	return RunOutput{Report: rep, CorrelationID: corr}, nil
}

type ShowRequest struct{}

// ShowOutput reports whether a pass has run and, if so, the latest report.
type ShowOutput struct {
	Found  bool             `json:"found"`
	Report *kreflect.Report `json:"report,omitempty"`
}

func (s *Service) Show(_ context.Context, _ ShowRequest) (ShowOutput, error) {
	rep, ok := s.engine.Latest()
	if !ok {
		return ShowOutput{Found: false}, nil
	}
	return ShowOutput{Found: true, Report: &rep}, nil
}

// Operations declares the audited pass trigger and the read-only latest
// report, both operator-only and without Web UI routes.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("reflection provider required")
	}
	runOut, err := schema.FromType(reflect.TypeFor[RunOutput](), false)
	if err != nil {
		return nil, err
	}
	showOut, err := schema.FromType(reflect.TypeFor[ShowOutput](), false)
	if err != nil {
		return nil, err
	}
	run, err := app.NewOperation(opapi.Spec{Name: "reflect_run", OutputSchema: runOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}, func(ctx context.Context, in RunRequest) (RunOutput, error) {
		return provider(ctx).Run(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	show, err := app.NewOperation(opapi.Spec{Name: "reflect_show", ReadOnly: true, OutputSchema: showOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}, func(ctx context.Context, in ShowRequest) (ShowOutput, error) {
		return provider(ctx).Show(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{run, show}, nil
}
