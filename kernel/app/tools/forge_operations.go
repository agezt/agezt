// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"strings"
)

type ForgeShowRequest struct {
	Ref json.RawMessage `json:"ref,omitempty"`
}

var forgeShowSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{}}}`)

func forgeRequiredRef(raw json.RawMessage) (string, error) {
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
func ForgeReadOperations(provider func(context.Context) *ForgeCatalog) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("forge read provider required")
	}
	spec := func(name string) opapi.Spec {
		return opapi.Spec{Name: name, ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}
	}
	list, err := app.NewOperation(spec("toolforge_list"), func(ctx context.Context, in ForgeListInput) (ForgeListOutput, error) {
		return provider(ctx).List(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	showSpec := spec("toolforge_show")
	showSpec.InputSchema = forgeShowSchema
	show, err := app.NewOperation(showSpec, func(ctx context.Context, in ForgeShowRequest) (ForgeShowOutput, error) {
		ref, err := forgeRequiredRef(in.Ref)
		if err != nil {
			return ForgeShowOutput{}, err
		}
		return provider(ctx).Show(ctx, ForgeShowInput{Ref: ref})
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{list, show}, nil
}
