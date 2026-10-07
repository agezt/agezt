// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

type GatewayRequest struct {
	URL     json.RawMessage `json:"url,omitempty"`
	Backend json.RawMessage `json:"backend,omitempty"`
	Session json.RawMessage `json:"session,omitempty"`
	Key     json.RawMessage `json:"key,omitempty"`
}

func gatewayText(raw json.RawMessage) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	text, _ := value.(string)
	return text
}
func (r GatewayRequest) input() GatewayInput {
	return GatewayInput{URL: gatewayText(r.URL), Backend: gatewayText(r.Backend), Session: gatewayText(r.Session), Key: gatewayText(r.Key)}
}

func GatewayOperations(provider func(context.Context) *Gateway) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("channel gateway service provider required")
	}
	spec := func(name, path string) opapi.Spec {
		return opapi.Spec{Name: name, ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true,
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"url":{},"backend":{},"session":{},"key":{}}}`),
			HTTP:        opapi.HTTP{Method: "POST", Path: path}}
	}
	status, err := app.NewOperation(spec("whatsappgw_status", "/api/whatsappgw/status"), func(ctx context.Context, in GatewayRequest) (GatewayStatusOutput, error) {
		return provider(ctx).Status(ctx, in.input())
	})
	if err != nil {
		return nil, err
	}
	qr, err := app.NewOperation(spec("whatsappgw_qr", "/api/whatsappgw/qr"), func(ctx context.Context, in GatewayRequest) (GatewayQROutput, error) {
		return provider(ctx).QR(ctx, in.input())
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{status, qr}, nil
}
