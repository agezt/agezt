// SPDX-License-Identifier: MIT

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

type Dependencies struct {
	Auth   opapi.Authenticator
	Router opapi.TenantRouter
	Audit  opapi.Auditor
}

// Dispatcher retains a private immutable registry; callers receive schema copies.
type Dispatcher struct {
	ops  map[string]Operation
	deps Dependencies
}

func NewDispatcher(operations []Operation, deps Dependencies) (*Dispatcher, error) {
	if deps.Auth == nil || deps.Router == nil {
		return nil, errors.New("operation auth and tenant router are required")
	}
	d := &Dispatcher{ops: make(map[string]Operation, len(operations)), deps: deps}
	for _, operation := range operations {
		if operation.spec.Name == "" || operation.decode == nil || operation.run == nil {
			return nil, errors.New("unbound operation")
		}
		if _, exists := d.ops[operation.spec.Name]; exists {
			return nil, fmt.Errorf("duplicate operation %q", operation.spec.Name)
		}
		d.ops[operation.spec.Name] = operation
	}
	return d, nil
}

func (d *Dispatcher) Specs() []opapi.Spec {
	names := make([]string, 0, len(d.ops))
	for name := range d.ops {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]opapi.Spec, 0, len(names))
	for _, name := range names {
		result = append(result, d.ops[name].Spec())
	}
	return result
}

func (d *Dispatcher) Dispatch(ctx context.Context, caller opapi.Caller, name string, raw json.RawMessage, emitter opapi.Emitter) (output any, err error) {
	principal, err := d.deps.Auth.Authenticate(ctx, caller)
	if err != nil {
		return nil, err
	}
	operation, found := d.ops[name]
	// Preserve tenant denial opacity: unknown and primary-only operations share
	// the same authorization outcome for non-operator principals.
	if principal.Kind != opapi.Operator && (!found || operation.spec.Authz != opapi.OwnTenant || principal.Kind != opapi.Tenant) {
		return nil, errors.New("forbidden: primary operator required")
	}
	if !found {
		return nil, fmt.Errorf("unknown operation: %s", name)
	}
	if principal.Kind == opapi.Tenant && (principal.Tenant == "" || caller.Tenant != principal.Tenant) {
		return nil, errors.New("forbidden: tenant identity mismatch")
	}
	if principal.Kind == opapi.Operator {
		principal.Tenant = ""
		if operation.spec.Tenancy == opapi.CallerTenant {
			principal.Tenant = caller.Tenant
		}
	}
	ctx, err = d.deps.Router.Route(ctx, principal, operation.Spec())
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		return nil, errors.New("tenant router returned no context")
	}
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	input, err := operation.decode(raw)
	if err != nil {
		return nil, fmt.Errorf("operation %s input: %w", name, err)
	}
	if operation.spec.Stream != opapi.StreamNone && emitter == nil {
		return nil, errors.New("stream emitter required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var span opapi.AuditSpan
	defer func() {
		if panicValue := recover(); panicValue != nil {
			err = fmt.Errorf("operation %s panicked: %v", name, panicValue)
			output = nil
		}
		if span != nil {
			err = errors.Join(err, span.End(ctx, err))
		}
	}()
	if !operation.spec.ReadOnly {
		if d.deps.Audit == nil {
			return nil, errors.New("operation audit unavailable")
		}
		var beginErr error
		span, beginErr = d.deps.Audit.Begin(ctx, opapi.AuditRecord{Operation: name, Principal: principal, Input: append(json.RawMessage(nil), raw...)})
		if beginErr != nil {
			span = nil // failed admission does not transfer span ownership
			return nil, beginErr
		}
		if span == nil {
			return nil, errors.New("operation auditor returned no span")
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return operation.run(ctx, input, emitter)
}
