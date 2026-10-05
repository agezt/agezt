// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	store "github.com/agezt/agezt/kernel/memory"
)

type EmptyInput struct{}
type MaintenanceInput struct {
	DryRun json.RawMessage `json:"dry_run,omitempty"`
}
type PruneRequestInput struct {
	MaintenanceInput
	OlderThanDays any `json:"older_than_days,omitempty"`
}
type LogRequestInput struct {
	Op      string `json:"op,omitempty"`
	Limit   any    `json:"limit,omitempty"`
	SinceMS any    `json:"since_ms,omitempty"`
	Cursor  any    `json:"cursor,omitempty"`
}

func admittedDryRun(raw json.RawMessage) (bool, error) {
	if len(raw) == 0 {
		return true, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return true, err
	}
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		return !(v == "false" || v == "0"), nil
	default:
		return true, errors.New("args.dry_run must be a boolean")
	}
}
func admittedDays(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n := 0
		for _, r := range v {
			if r < '0' || r > '9' {
				return 0
			}
			n = n*10 + int(r-'0')
		}
		return n
	default:
		return 0
	}
}
func admittedLog(in LogRequestInput) LogInput {
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
	return LogInput{Limit: limit, CutoffMS: cutoff, Cursor: in.Cursor, OpFilter: in.Op}
}

func rememberSchema(supersede bool) json.RawMessage {
	schema := `{"type":"object","additionalProperties":true,"properties":{"content":{"type":"string"},"subject":{"type":"string"},"type":{"type":"string"},"confidence":{"type":"number"},"evidence":{"type":"string"},"half_life_ms":{"type":"number"},"tags":{"type":"object","additionalProperties":{"type":"string"}}},"required":["content"]}`
	if supersede {
		schema = strings.Replace(schema, `"content":{"type":"string"}`, `"old_id":{"type":"string"},"content":{"type":"string"}`, 1)
		schema = strings.Replace(schema, `["content"]`, `["old_id","content"]`, 1)
	}
	return json.RawMessage(schema)
}
func maintenanceSchema(prune bool) json.RawMessage {
	schema := `{"type":"object","additionalProperties":true,"properties":{"dry_run":{"type":["boolean","string"]}}}`
	if prune {
		schema = strings.Replace(schema, `"dry_run":`, `"older_than_days":{},"dry_run":`, 1)
	}
	return json.RawMessage(schema)
}

func bind[I, O any](operations *[]app.Operation, spec opapi.Spec, handler func(context.Context, I) (O, error)) error {
	spec.AllowUnknownInput = true
	operation, err := app.NewOperation(spec, handler)
	if err == nil {
		*operations = append(*operations, operation)
	}
	return err
}

func Operations(service func(context.Context) *Service, distillation func(context.Context) *Distillation, logs func(context.Context) *LogService) ([]app.Operation, error) {
	if service == nil || distillation == nil || logs == nil {
		return nil, errors.New("memory service providers required")
	}
	var operations []app.Operation
	bindings := []func() error{
		func() error {
			return bind(&operations, opapi.Spec{Name: "memory_get", ReadOnly: true}, func(ctx context.Context, in GetInput) (GetOutput, error) {
				if strings.TrimSpace(in.ID) == "" {
					return GetOutput{}, errors.New("args.id required")
				}
				return service(ctx).Get(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: "memory_list", ReadOnly: true, HTTP: opapi.HTTP{Method: "GET", Path: "/api/memory"}}, func(ctx context.Context, in ListInput) (ListOutput, error) {
				prepared, err := service(ctx).PrepareList(ctx)
				if err != nil {
					return ListOutput{}, err
				}
				return prepared.Page(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: "memory_search", ReadOnly: true}, func(ctx context.Context, in SearchInput) (SearchOutput, error) {
				if strings.TrimSpace(in.Query) == "" {
					return SearchOutput{}, errors.New("args.query required")
				}
				return service(ctx).Search(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: "memory_find_related", ReadOnly: true}, func(ctx context.Context, in RelatedInput) (SearchOutput, error) {
				if strings.TrimSpace(in.ID) == "" {
					return SearchOutput{}, errors.New("args.id required")
				}
				return service(ctx).FindRelated(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: "memory_add", InputSchema: rememberSchema(false), HTTP: opapi.HTTP{Method: "POST", Path: "/api/memory/add"}}, func(ctx context.Context, in RememberInput) (RememberOutput, error) {
				if in.Content == "" {
					return RememberOutput{}, errors.New("args.content required")
				}
				return service(ctx).Remember(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: "memory_supersede", InputSchema: rememberSchema(true), HTTP: opapi.HTTP{Method: "POST", Path: "/api/memory/supersede"}}, func(ctx context.Context, in SupersedeInput) (SupersedeOutput, error) {
				if in.OldID == "" {
					return SupersedeOutput{}, errors.New("args.old_id required")
				}
				if in.Content == "" {
					return SupersedeOutput{}, errors.New("args.content required")
				}
				return service(ctx).Supersede(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: "memory_forget", HTTP: opapi.HTTP{Method: "POST", Path: "/api/memory/forget"}}, func(ctx context.Context, in GetInput) (ForgetOutput, error) {
				if strings.TrimSpace(in.ID) == "" {
					return ForgetOutput{}, errors.New("args.id required")
				}
				return service(ctx).Forget(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: "memory_promote", HTTP: opapi.HTTP{Method: "POST", Path: "/api/memory/promote"}}, func(ctx context.Context, in GetInput) (PromoteOutput, error) {
				if strings.TrimSpace(in.ID) == "" {
					return PromoteOutput{}, errors.New("args.id required")
				}
				return service(ctx).Promote(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: "memory_bulk_forget", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ids":{"type":"array","items":{"type":"string"}}},"required":["ids"]}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/memory/bulk_forget"}}, func(ctx context.Context, in BulkForgetInput) (BulkForgetOutput, error) {
				ids := make([]string, 0, len(in.IDs))
				for _, id := range in.IDs {
					if id = strings.TrimSpace(id); id != "" {
						ids = append(ids, id)
					}
				}
				return service(ctx).BulkForget(ctx, BulkForgetInput{IDs: ids})
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: "memory_prune", InputSchema: maintenanceSchema(true), HTTP: opapi.HTTP{Method: "POST", Path: "/api/memory/prune"}}, func(ctx context.Context, in PruneRequestInput) (PruneOutput, error) {
				dry, err := admittedDryRun(in.DryRun)
				if err != nil {
					return PruneOutput{}, err
				}
				return service(ctx).Prune(ctx, PruneInput{OlderThanDays: admittedDays(in.OlderThanDays), DryRun: dry})
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: "memory_tidy", InputSchema: maintenanceSchema(false), HTTP: opapi.HTTP{Method: "POST", Path: "/api/memory/tidy"}}, func(ctx context.Context, in MaintenanceInput) (TidyOutput, error) {
				dry, err := admittedDryRun(in.DryRun)
				if err != nil {
					return TidyOutput{}, err
				}
				return service(ctx).Tidy(ctx, HygieneInput{DryRun: dry})
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: "memory_audit", ReadOnly: true, Authz: opapi.OwnTenant, Tenancy: opapi.CallerTenant, HTTP: opapi.HTTP{Method: "GET", Path: "/api/memory/audit"}}, func(ctx context.Context, _ EmptyInput) (store.AuditReport, error) {
				return service(ctx).Audit(ctx, struct{}{})
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: "memory_clean", Authz: opapi.OwnTenant, Tenancy: opapi.CallerTenant, InputSchema: maintenanceSchema(false), HTTP: opapi.HTTP{Method: "POST", Path: "/api/memory/clean"}}, func(ctx context.Context, in MaintenanceInput) (store.CleanReport, error) {
				dry, err := admittedDryRun(in.DryRun)
				if err != nil {
					return store.CleanReport{}, err
				}
				return service(ctx).Clean(ctx, HygieneInput{DryRun: dry})
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: "memory_log", ReadOnly: true, Authz: opapi.OwnTenant, Tenancy: opapi.CallerTenant, HTTP: opapi.HTTP{Method: "GET", Path: "/api/memory_log"}}, func(ctx context.Context, in LogRequestInput) (LogOutput, error) {
				return logs(ctx).Log(ctx, admittedLog(in))
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: "memory_consolidate"}, func(ctx context.Context, in DistillInput) (ConsolidateOutput, error) {
				return distillation(ctx).Consolidate(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: "profile_rebuild", HTTP: opapi.HTTP{Method: "POST", Path: "/api/profile/rebuild"}}, func(ctx context.Context, in DistillInput) (ProfileOutput, error) {
				return distillation(ctx).RebuildProfile(ctx, in)
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
