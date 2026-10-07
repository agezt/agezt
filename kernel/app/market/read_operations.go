// SPDX-License-Identifier: MIT
package market

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"strings"
)

type ListRequest struct {
	Query json.RawMessage `json:"query,omitempty"`
}
type ShowRequest struct {
	Name        json.RawMessage `json:"name,omitempty"`
	Marketplace json.RawMessage `json:"marketplace,omitempty"`
}

var readInputSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"query":{},"name":{},"marketplace":{}}}`)

// Permissive native fields: missing, null and nonstrings select empty text.
func readText(raw json.RawMessage) string {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}
func ReadOperations(provider func(context.Context) *Reads) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("market read provider required")
	}
	spec := func(name string) opapi.Spec {
		return opapi.Spec{Name: name, ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: readInputSchema}
	}
	list, err := app.NewOperation(spec("market_list"), func(ctx context.Context, in ListRequest) (ListOutput, error) {
		return provider(ctx).List(ctx, ListInput{Query: readText(in.Query)})
	})
	if err != nil {
		return nil, err
	}
	show, err := app.NewOperation(spec("market_show"), func(ctx context.Context, in ShowRequest) (ShowOutput, error) {
		return provider(ctx).Show(ctx, ShowInput{Marketplace: readText(in.Marketplace), Name: readText(in.Name)})
	})
	if err != nil {
		return nil, err
	}
	sources, err := app.NewOperation(spec("market_sources"), func(ctx context.Context, in SourcesInput) (SourcesOutput, error) {
		return provider(ctx).Sources(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{list, show, sources}, nil
}
