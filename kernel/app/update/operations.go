// SPDX-License-Identifier: MIT
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

type CheckRequest struct{}
type ApplyRequest struct {
	Version json.RawMessage `json:"version,omitempty"`
	SHA256  json.RawMessage `json:"sha256,omitempty"`
	URL     json.RawMessage `json:"url,omitempty"`
	Notes   json.RawMessage `json:"notes,omitempty"`
}

func (in ApplyRequest) input() ApplyInput {
	values := []struct {
		name string
		raw  json.RawMessage
	}{{"version", in.Version}, {"sha256", in.SHA256}, {"url", in.URL}, {"notes", in.Notes}}
	var strings [4]string
	for i, value := range values {
		if len(value.raw) == 0 {
			continue
		}
		var decoded any
		_ = json.Unmarshal(value.raw, &decoded)
		text, ok := decoded.(string)
		if !ok {
			return ApplyInput{DecodeError: fmt.Errorf("args.%s must be a string", value.name)}
		}
		strings[i] = text
	}
	return ApplyInput{Version: strings[0], SHA256: strings[1], URL: strings[2], Notes: strings[3]}
}
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("update service provider required")
	}
	check, err := app.NewOperation(opapi.Spec{Name: "update_check", ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}, func(ctx context.Context, _ CheckRequest) (out CheckOutput, cause error) {
		defer func() {
			if recover() != nil {
				out = CheckOutput{}
				cause = errors.New("internal error")
			}
		}()
		provider(ctx).Check(ctx, func(result CheckOutput, err error) { out, cause = result, err })
		return
	})
	if err != nil {
		return nil, err
	}
	apply, err := app.NewOperation(opapi.Spec{Name: "update_apply", Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"version":{},"sha256":{},"url":{},"notes":{}}}`)}, func(ctx context.Context, in ApplyRequest) (out ApplyOutput, cause error) {
		defer func() {
			if recover() != nil {
				out = ApplyOutput{}
				cause = errors.New("internal error")
			}
		}()
		provider(ctx).Apply(ctx, in.input(), func(result ApplyOutput, err error) { out, cause = result, err })
		return
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{check, apply}, nil
}
