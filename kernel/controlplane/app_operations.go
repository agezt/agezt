// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/app/system"
	"github.com/agezt/agezt/kernel/contract/opapi"
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
		spec := operation.Spec()
		register(commandSpec{Cmd: spec.Name, ReadOnly: spec.ReadOnly,
			TenantAllowed: spec.Authz == opapi.OwnTenant, TenantRouted: spec.Tenancy == opapi.CallerTenant,
			Streaming: StreamMode(spec.Stream), Handler: handleAppSystemOperation})
	}
}

func handleAppSystemOperation(dc *DispatchCtx) {
	dc.S.operationOnce.Do(func() {
		dc.S.operations, dc.S.operationErr = app.NewDispatcher(systemOperations, app.Dependencies{Auth: appAuthenticator{dc.S}, Router: appTenantRouter{dc.S}})
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
	output, err := dc.S.operations.Dispatch(dc.Ctx, opapi.Caller{Credential: dc.Req.Token, Tenant: tenantOf(dc.Req), Source: "controlplane"}, dc.Req.Cmd, raw, nil)
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
	if spec.Tenancy != opapi.Primary {
		return nil, errors.New("system operations must use the primary host")
	}
	return context.WithValue(ctx, systemHostKey{}, r.server), nil
}
