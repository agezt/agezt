// SPDX-License-Identifier: MIT

package files

import (
	"context"
	"errors"
	"reflect"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// Ports binds one invocation: the governed tool runner, the operation's audit
// correlation, the caller's request identity (the tool call's id) and the
// daemon's rollback catalog.
type Ports struct {
	Runner      Runner
	Correlation string
	CallID      string
	CatalogPath string
}

type Service struct{ ports Ports }

func New(ports Ports) *Service { return &Service{ports: ports} }

// Mutate applies mkdir, rename or delete through the governed tool runner.
//
// The arguments are the operation's request row, read by the caller through
// opapi.Args — so the field names are spelled once, in the contract.
func (s *Service) Mutate(ctx context.Context, operation string, args map[string]any) (MutationOutput, error) {
	return Apply(ctx, s.ports.Runner, s.ports.Correlation, s.ports.CallID, operation, args)
}

// Restore applies a file checkpoint from the daemon's own catalog. Snapshot
// data the caller sends is never read.
func (s *Service) Restore(ctx context.Context, in opapi.FileRestoreRequest) (RestoreOutput, error) {
	return ApplyRestore(ctx, s.ports.Runner, s.ports.Correlation, s.ports.CallID, s.ports.CatalogPath, in.ID)
}

// classified keeps every failure's domain code: an unclassified error is
// reported as unavailable.
func classified(err error) error {
	var domain *Error
	if err == nil || errors.As(err, &domain) {
		return err
	}
	return &Error{Code: Unavailable, Cause: err}
}

// Operations declares the audited, operator-only workspace mutations and the
// snapshot restore. The console workspace is daemon-global, so tenant tokens
// are refused. They have no shared Web UI route: the File Manager's handlers
// call them.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("files provider required")
	}
	mutationOut, err := schema.FromType(reflect.TypeFor[MutationOutput](), false)
	if err != nil {
		return nil, err
	}
	restoreOut, err := schema.FromType(reflect.TypeFor[RestoreOutput](), false)
	if err != nil {
		return nil, err
	}
	// The three mutations are three operations with three request rows, and
	// each row's own shape is the operation's declared input — no schema is
	// written by hand here.
	mkdir, err := app.NewOperation(opapi.Spec{Name: Mkdir, OutputSchema: mutationOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}, func(ctx context.Context, in opapi.FileMkdirRequest) (MutationOutput, error) {
		out, err := provider(ctx).Mutate(ctx, Mkdir, opapi.Args(in))
		return out, classified(err)
	})
	if err != nil {
		return nil, err
	}
	rename, err := app.NewOperation(opapi.Spec{Name: Rename, OutputSchema: mutationOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}, func(ctx context.Context, in opapi.FileRenameRequest) (MutationOutput, error) {
		out, err := provider(ctx).Mutate(ctx, Rename, opapi.Args(in))
		return out, classified(err)
	})
	if err != nil {
		return nil, err
	}
	remove, err := app.NewOperation(opapi.Spec{Name: Delete, OutputSchema: mutationOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}, func(ctx context.Context, in opapi.FileDeleteRequest) (MutationOutput, error) {
		out, err := provider(ctx).Mutate(ctx, Delete, opapi.Args(in))
		return out, classified(err)
	})
	if err != nil {
		return nil, err
	}
	restore, err := app.NewOperation(opapi.Spec{Name: RestoreOperation, OutputSchema: restoreOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}, func(ctx context.Context, in opapi.FileRestoreRequest) (RestoreOutput, error) {
		out, err := provider(ctx).Restore(ctx, in)
		return out, classified(err)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{mkdir, rename, remove, restore}, nil
}
