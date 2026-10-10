// SPDX-License-Identifier: MIT

package files

import (
	"context"
	"encoding/json"
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

// MutationRequest carries a mutation's arguments as sent. Text and flags of
// another type read as empty and false.
type MutationRequest struct {
	Path      json.RawMessage `json:"path,omitempty"`
	From      json.RawMessage `json:"from,omitempty"`
	To        json.RawMessage `json:"to,omitempty"`
	Parents   json.RawMessage `json:"parents,omitempty"`
	Recursive json.RawMessage `json:"recursive,omitempty"`
}

func (in MutationRequest) args() map[string]any {
	args := map[string]any{}
	for name, raw := range map[string]json.RawMessage{"path": in.Path, "from": in.From, "to": in.To, "parents": in.Parents, "recursive": in.Recursive} {
		if len(raw) != 0 {
			var v any
			_ = json.Unmarshal(raw, &v)
			args[name] = v
		}
	}
	return args
}

// Mutate applies mkdir, rename or delete through the governed tool runner.
func (s *Service) Mutate(ctx context.Context, operation string, in MutationRequest) (MutationOutput, error) {
	return Apply(ctx, s.ports.Runner, s.ports.Correlation, s.ports.CallID, operation, in.args())
}

// RestoreRequest names the checkpoint to restore. Anything other than text
// reads as no id.
type RestoreRequest struct {
	ID json.RawMessage `json:"id,omitempty"`
}

// Restore applies a file checkpoint from the daemon's own catalog. Snapshot
// data the caller sends is never read.
func (s *Service) Restore(ctx context.Context, in RestoreRequest) (RestoreOutput, error) {
	var id string
	if len(in.ID) != 0 {
		var v any
		_ = json.Unmarshal(in.ID, &v)
		id, _ = v.(string)
	}
	return ApplyRestore(ctx, s.ports.Runner, s.ports.Correlation, s.ports.CallID, s.ports.CatalogPath, id)
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
	var ops []app.Operation
	for _, d := range []struct{ name, input string }{
		{Mkdir, `{"type":"object","additionalProperties":true,"properties":{"path":{},"parents":{}}}`},
		{Rename, `{"type":"object","additionalProperties":true,"properties":{"from":{},"to":{}}}`},
		{Delete, `{"type":"object","additionalProperties":true,"properties":{"path":{},"recursive":{}}}`},
	} {
		operation := d.name
		op, err := app.NewOperation(opapi.Spec{Name: operation, OutputSchema: mutationOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(d.input)}, func(ctx context.Context, in MutationRequest) (MutationOutput, error) {
			out, err := provider(ctx).Mutate(ctx, operation, in)
			return out, classified(err)
		})
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	restore, err := app.NewOperation(opapi.Spec{Name: RestoreOperation, OutputSchema: restoreOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"id":{}}}`)}, func(ctx context.Context, in RestoreRequest) (RestoreOutput, error) {
		out, err := provider(ctx).Restore(ctx, in)
		return out, classified(err)
	})
	if err != nil {
		return nil, err
	}
	return append(ops, restore), nil
}
