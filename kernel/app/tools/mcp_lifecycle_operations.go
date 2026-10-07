// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/mcp"
	"strings"
)

type MCPAddRequest struct {
	Server json.RawMessage `json:"server,omitempty"`
}
type MCPRefRequest struct {
	Ref json.RawMessage `json:"ref,omitempty"`
}
type MCPSetEnabledRequest struct {
	Ref     json.RawMessage `json:"ref,omitempty"`
	Enabled json.RawMessage `json:"enabled,omitempty"`
}

var mcpLifecycleInputSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"server":{},"ref":{},"enabled":{}}}`)

func mcpDecodeServer(raw json.RawMessage) (mcp.Server, error) {
	if len(raw) == 0 {
		return mcp.Server{}, errors.New("args.server required")
	}
	var srv mcp.Server
	if err := json.Unmarshal(raw, &srv); err != nil {
		return mcp.Server{}, fmt.Errorf("args.server: %w", err)
	}
	return srv, nil
}
func mcpRequiredRef(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", errors.New("args.ref required")
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	ref, ok := value.(string)
	if !ok {
		return "", errors.New("args.ref must be a string")
	}
	if strings.TrimSpace(ref) == "" {
		return "", errors.New("args.ref required")
	}
	return ref, nil
}

// The historical enable codec accepts booleans and exactly true (case-insensitive)
// or 1 strings. Missing, null and other types/strings select false.
func mcpEnabled(raw json.RawMessage) bool {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true") || v == "1"
	}
	return false
}
func MCPLifecycleOperations(provider func(context.Context) *MCPLifecycle) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("mcp lifecycle provider required")
	}
	spec := func(name string) opapi.Spec {
		return opapi.Spec{Name: name, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: mcpLifecycleInputSchema}
	}
	add, err := app.NewOperation(spec("mcp_add"), func(ctx context.Context, in MCPAddRequest) (MCPServerOutput, error) {
		srv, err := mcpDecodeServer(in.Server)
		if err != nil {
			return MCPServerOutput{}, err
		}
		return provider(ctx).Add(ctx, MCPAddInput{Server: srv})
	})
	if err != nil {
		return nil, err
	}
	attach, err := app.NewOperation(spec("mcp_attach"), func(ctx context.Context, in MCPRefRequest) (MCPAttachOutput, error) {
		ref, err := mcpRequiredRef(in.Ref)
		if err != nil {
			return MCPAttachOutput{}, err
		}
		return provider(ctx).Attach(ctx, MCPRefInput{Ref: ref})
	})
	if err != nil {
		return nil, err
	}
	detach, err := app.NewOperation(spec("mcp_detach"), func(ctx context.Context, in MCPRefRequest) (MCPDetachOutput, error) {
		ref, err := mcpRequiredRef(in.Ref)
		if err != nil {
			return MCPDetachOutput{}, err
		}
		return provider(ctx).Detach(ctx, MCPRefInput{Ref: ref})
	})
	if err != nil {
		return nil, err
	}
	enabled, err := app.NewOperation(spec("mcp_set_enabled"), func(ctx context.Context, in MCPSetEnabledRequest) (MCPServerOutput, error) {
		ref, err := mcpRequiredRef(in.Ref)
		if err != nil {
			return MCPServerOutput{}, err
		}
		return provider(ctx).SetEnabled(ctx, MCPSetEnabledInput{Ref: ref, Enabled: mcpEnabled(in.Enabled)})
	})
	if err != nil {
		return nil, err
	}
	remove, err := app.NewOperation(spec("mcp_remove"), func(ctx context.Context, in MCPRefRequest) (MCPRemoveOutput, error) {
		ref, err := mcpRequiredRef(in.Ref)
		if err != nil {
			return MCPRemoveOutput{}, err
		}
		return provider(ctx).Remove(ctx, MCPRefInput{Ref: ref})
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{add, attach, detach, enabled, remove}, nil
}
