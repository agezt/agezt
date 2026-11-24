// SPDX-License-Identifier: MIT

package world

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

type ResolveRequestInput struct {
	Query string  `json:"query"`
	Limit float64 `json:"limit,omitempty"`
}
type LogRequestInput struct {
	Kind    string `json:"kind,omitempty"`
	Limit   any    `json:"limit,omitempty"`
	SinceMS any    `json:"since_ms,omitempty"`
	Cursor  any    `json:"cursor,omitempty"`
}

func aliases(in []string) []string {
	out := make([]string, 0, len(in))
	for _, value := range in {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}
func curationSchema(edit bool) json.RawMessage {
	schema := `{"type":"object","additionalProperties":true,"properties":{"name":{"type":"string"},"kind":{"type":"string"},"aliases":{"type":"array","items":{"type":"string"}},"attrs":{"type":"object","additionalProperties":{"type":"string"}}},"required":["name"]}`
	if edit {
		schema = `{"type":"object","additionalProperties":true,"properties":{"id":{"type":"string"},"aliases":{"type":"array","items":{"type":"string"}},"attrs":{"type":"object","additionalProperties":{"type":"string"}}},"required":["id"]}`
	}
	return json.RawMessage(schema)
}

// The log read's contract names every field the adapter below reads: the kind
// filter is text, the page size and the window are numbers, and the cursor is
// the text the journal encodes it as.
var logSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"kind":{"type":"string"},"limit":{"type":"number"},"since_ms":{"type":"number"},"cursor":{"type":"string"}}}`)

func logPage(in LogRequestInput) LogInput {
	limit := 20
	if in.Limit != nil {
		switch v := in.Limit.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		case int64:
			limit = int(v)
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
	}
	var since int64
	switch v := in.SinceMS.(type) {
	case float64:
		since = int64(v)
	case int:
		since = int64(v)
	case int64:
		since = v
	}
	var cutoff int64
	if since > 0 {
		cutoff = time.Now().UnixMilli() - since
	}
	return LogInput{Limit: limit, CutoffMS: cutoff, Cursor: in.Cursor, KindFilter: in.Kind}
}
func bind[I, O any](operations *[]app.Operation, spec opapi.Spec, handler func(context.Context, I) (O, error)) error {
	spec.AllowUnknownInput = true
	operation, err := app.NewOperation(spec, handler)
	if err == nil {
		*operations = append(*operations, operation)
	}
	return err
}

func Operations(service func(context.Context) *Service, logs func(context.Context) *LogService) ([]app.Operation, error) {
	if service == nil || logs == nil {
		return nil, errors.New("world service providers required")
	}
	var operations []app.Operation
	bindings := []func() error{
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.WorldAdd, InputSchema: curationSchema(false)}, func(ctx context.Context, in AddInput) (AddOutput, error) {
				if strings.TrimSpace(in.Name) == "" {
					return AddOutput{}, errors.New("args.name required")
				}
				in.Aliases = aliases(in.Aliases)
				return service(ctx).Add(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.WorldEdit, InputSchema: curationSchema(true)}, func(ctx context.Context, in EditInput) (EditOutput, error) {
				if strings.TrimSpace(in.ID) == "" {
					return EditOutput{}, errors.New("args.id required")
				}
				in.Aliases = aliases(in.Aliases)
				return service(ctx).Edit(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.WorldRelate}, func(ctx context.Context, in RelateInput) (RelateOutput, error) {
				if in.From == "" || in.To == "" {
					return RelateOutput{}, errors.New("args.from and args.to required")
				}
				return service(ctx).Relate(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.WorldForget}, func(ctx context.Context, in GetInput) (ForgetOutput, error) {
				if strings.TrimSpace(in.ID) == "" {
					return ForgetOutput{}, errors.New("args.id required")
				}
				return service(ctx).Forget(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.WorldResolve, ReadOnly: true}, func(ctx context.Context, in ResolveRequestInput) (ResolveOutput, error) {
				if strings.TrimSpace(in.Query) == "" {
					return ResolveOutput{}, errors.New("args.query required")
				}
				limit := 10
				if in.Limit > 0 {
					limit = int(in.Limit)
				}
				if limit > 100 {
					limit = 100
				}
				return service(ctx).Resolve(ctx, ResolveInput{Query: in.Query, Limit: limit})
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.WorldNeighbors, ReadOnly: true}, func(ctx context.Context, in QueryInput) (NeighborsOutput, error) {
				if strings.TrimSpace(in.Query) == "" {
					return NeighborsOutput{}, errors.New("args.query required")
				}
				return service(ctx).Neighbors(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.WorldList, ReadOnly: true}, func(ctx context.Context, in ListInput) (ListOutput, error) { return service(ctx).List(ctx, in) })
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.WorldGet, ReadOnly: true}, func(ctx context.Context, in GetInput) (GetOutput, error) {
				if strings.TrimSpace(in.ID) == "" {
					return GetOutput{}, errors.New("args.id required")
				}
				return service(ctx).Get(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.WorldLog, ReadOnly: true, Authz: opapi.OwnTenant, Tenancy: opapi.CallerTenant, InputSchema: logSchema}, func(ctx context.Context, in LogRequestInput) (LogOutput, error) {
				return logs(ctx).Log(ctx, logPage(in))
			})
		},
	}
	for _, binding := range bindings {
		if err := binding(); err != nil {
			return nil, err
		}
	}
	return operations, nil
}
