// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/app/system"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
)

type systemHostKey struct{}

var systemOperations = func() []app.Operation {
	ops, err := system.Operations(func(ctx context.Context) *system.Service { return ctx.Value(systemHostKey{}).(*Server).systemService() })
	if err != nil {
		panic(err)
	}
	return ops
}()

func registerAppSystemCommands() {
	for _, operation := range systemOperations {
		spec, err := appCommandSpec(operation)
		if err != nil {
			panic(err)
		}
		register(spec)
	}
}

// appCommandSpec binds the native protocol's object result and Event envelopes
// before any operation can mutate state through this adapter.
func appCommandSpec(operation app.Operation) (commandSpec, error) {
	spec := operation.Spec()
	var declaration struct {
		Type any `json:"type"`
	}
	if err := json.Unmarshal(spec.OutputSchema, &declaration); err != nil {
		return commandSpec{}, err
	}
	var types []any
	switch declared := declaration.Type.(type) {
	case string:
		types = []any{declared}
	case []any:
		types = declared
	}
	object := false
	for _, typ := range types {
		if typ == "object" {
			object = true
		} else if typ != "null" {
			return commandSpec{}, errors.New("control-plane terminal output requires an object schema")
		}
	}
	if !object {
		return commandSpec{}, errors.New("control-plane terminal output requires an object schema")
	}
	if spec.Stream != opapi.StreamNone && spec.Emission != reflect.TypeFor[event.Event]() && spec.Emission != reflect.TypeFor[*event.Event]() {
		return commandSpec{}, errors.New("control-plane streams require kernel event frames")
	}
	return commandSpec{Cmd: spec.Name, ReadOnly: spec.ReadOnly, AppOwned: true,
		TenantAllowed: spec.Authz == opapi.OwnTenant, TenantRouted: spec.Tenancy == opapi.CallerTenant,
		Streaming: StreamMode(spec.Stream), Handler: handleAppOperation}, nil
}

func handleAppOperation(dc *DispatchCtx) {
	dc.S.operationOnce.Do(func() {
		dc.S.operations, dc.S.operationErr = app.NewDispatcher(systemOperations, app.Dependencies{Auth: appAuthenticator{dc.S}, Router: appTenantRouter{dc.S}, Audit: appAuditor{}})
	})
	if dc.S.operationErr != nil {
		dc.S.fail(dc.Conn, dc.Req, dc.S.operationErr)
		return
	}
	raw, err := json.Marshal(dc.Req.Args)
	if err != nil {
		dc.S.fail(dc.Conn, dc.Req, err)
		return
	}
	if dc.Req.Args == nil {
		raw = json.RawMessage(`{}`)
	}
	output, err := dc.S.operations.Dispatch(dc.Ctx, opapi.Caller{Credential: dc.Req.Token, Tenant: tenantOf(dc.Req), Source: "controlplane"}, dc.Req.Cmd, raw, appEmitter{dc.Conn, dc.Req.ID})
	if err != nil {
		dc.S.fail(dc.Conn, dc.Req, err)
		return
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		dc.S.fail(dc.Conn, dc.Req, err)
		return
	}
	var result map[string]any
	if err := json.Unmarshal(encoded, &result); err != nil {
		dc.S.fail(dc.Conn, dc.Req, err)
		return
	}

	dc.S.writeResp(dc.Conn, Response{ID: dc.Req.ID, Type: RespResult, Result: result})
}

type appAuthenticator struct{ server *Server }

func (a appAuthenticator) Authenticate(_ context.Context, caller opapi.Caller) (opapi.Principal, error) {
	if a.server.tokenIsPrimary(caller.Credential) {
		return opapi.Principal{Kind: opapi.Operator}, nil
	}
	tenant := strings.TrimSpace(caller.Tenant)
	if tenant == "" || a.server.tenants == nil || !a.server.tenants.Authorize(tenant, caller.Credential) {
		return opapi.Principal{}, errors.New("unauthorized")
	}
	return opapi.Principal{Kind: opapi.Tenant, Tenant: tenant}, nil
}

type appTenantRouter struct{ server *Server }

func (r appTenantRouter) Route(ctx context.Context, principal opapi.Principal, spec opapi.Spec) (context.Context, error) {
	k := r.server.k
	if spec.Tenancy == opapi.CallerTenant {
		var err error
		k, err = r.server.kernelFor(principal.Tenant)
		if err != nil {
			return nil, err
		}
	}
	host := appHost{kernel: k}
	if !spec.ReadOnly {
		host.correlation = k.NewCorrelation()
		ctx = k.WithActorCorrelation(ctx, string(principal.Kind), host.correlation)
	}
	ctx = context.WithValue(ctx, appHostKey{}, host)
	return context.WithValue(ctx, systemHostKey{}, r.server), nil
}
