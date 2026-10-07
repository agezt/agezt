// SPDX-License-Identifier: MIT

package artifacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"strings"
)

type GetRequestInput struct {
	Ref json.RawMessage `json:"ref,omitempty"`
}
type ListRequestInput struct {
	Kind          json.RawMessage `json:"kind,omitempty"`
	Source        json.RawMessage `json:"source,omitempty"`
	CorrelationID json.RawMessage `json:"corr,omitempty"`
}
type DeleteRequestInput struct {
	ID json.RawMessage `json:"id,omitempty"`
}
type CollectRequestInput struct {
	OlderThanDays any             `json:"older_than_days,omitempty"`
	DryRun        json.RawMessage `json:"dry_run,omitempty"`
}

func requiredText(raw json.RawMessage, key string) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("args.%s required", key)
	}
	var decoded any
	_ = json.Unmarshal(raw, &decoded)
	value, ok := decoded.(string)
	if !ok {
		return "", fmt.Errorf("args.%s must be a string", key)
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("args.%s required", key)
	}
	return value, nil
}
func optionalString(raw json.RawMessage, key string) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var value any
	_ = json.Unmarshal(raw, &value)
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("args.%s must be a string", key)
	}
	return text, nil
}
func days(raw any) int {
	switch value := raw.(type) {
	case float64:
		return int(value)
	case int:
		return value
	case string:
		n := 0
		for _, r := range value {
			if r < '0' || r > '9' {
				return 0
			}
			n = n*10 + int(r-'0')
		}
		return n
	}
	return 0
}
func dryRun(raw json.RawMessage) (bool, error) {
	if len(raw) == 0 {
		return true, nil
	}
	var value any
	_ = json.Unmarshal(raw, &value)
	switch typed := value.(type) {
	case bool:
		return typed, nil
	case string:
		return !(typed == "false" || typed == "0"), nil
	default:
		return true, errors.New("args.dry_run must be a boolean")
	}
}
func bind[I, O any](ops *[]app.Operation, spec opapi.Spec, handler func(context.Context, I) (O, error)) error {
	spec.AllowUnknownInput = true
	operation, err := app.NewOperation(spec, handler)
	if err == nil {
		*ops = append(*ops, operation)
	}
	return err
}

var getRequestSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{}}}`)
var deleteRequestSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"id":{}}}`)
var listRequestSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"kind":{},"source":{},"corr":{}}}`)
var collectRequestSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"older_than_days":{},"dry_run":{}}}`)

func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("artifact service provider required")
	}
	var ops []app.Operation
	bindings := []func() error{
		func() error {
			return bind(&ops, opapi.Spec{Name: "artifact_get", ReadOnly: true, InputSchema: getRequestSchema}, func(ctx context.Context, in GetRequestInput) (GetOutput, error) {
				ref, err := requiredText(in.Ref, "ref")
				if err != nil {
					return GetOutput{}, err
				}
				return provider(ctx).Get(ctx, GetInput{Ref: ref})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "artifact_list", ReadOnly: true, InputSchema: listRequestSchema}, func(ctx context.Context, in ListRequestInput) (ListOutput, error) {
				service := provider(ctx)
				if service.index == nil {
					return ListOutput{}, errors.New("artifact index unavailable")
				}
				kind, err := optionalString(in.Kind, "kind")
				if err != nil {
					return ListOutput{}, err
				}
				source, err := optionalString(in.Source, "source")
				if err != nil {
					return ListOutput{}, err
				}
				corr, err := optionalString(in.CorrelationID, "corr")
				if err != nil {
					return ListOutput{}, err
				}
				return service.List(ctx, ListInput{Kind: kind, Source: source, CorrelationID: corr})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "artifact_delete", InputSchema: deleteRequestSchema}, func(ctx context.Context, in DeleteRequestInput) (DeleteOutput, error) {
				id, err := requiredText(in.ID, "id")
				if err != nil {
					return DeleteOutput{}, err
				}
				return provider(ctx).Delete(ctx, DeleteInput{ID: id})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "artifact_collect", InputSchema: collectRequestSchema}, func(ctx context.Context, in CollectRequestInput) (CollectOutput, error) {
				service := provider(ctx)
				if service.index == nil {
					return CollectOutput{}, errors.New("artifact index unavailable")
				}
				dry, err := dryRun(in.DryRun)
				if err != nil {
					return CollectOutput{}, err
				}
				return service.Collect(ctx, CollectInput{OlderThanDays: days(in.OlderThanDays), DryRun: dry})
			})
		},
	}
	for _, binding := range bindings {
		if err := binding(); err != nil {
			return nil, err
		}
	}
	return ops, nil
}
