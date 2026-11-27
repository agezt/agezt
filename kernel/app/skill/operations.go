// SPDX-License-Identifier: MIT

package skill

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"strings"
)

type ImportRequestInput struct {
	Name          any               `json:"name,omitempty"`
	Body          any               `json:"body,omitempty"`
	Description   any               `json:"description,omitempty"`
	Agent         any               `json:"agent,omitempty"`
	Triggers      []string          `json:"triggers,omitempty"`
	ToolsRequired []string          `json:"tools_required,omitempty"`
	Resources     map[string]string `json:"resources,omitempty"`
}
type HygieneRequestInput struct {
	IdleDays any `json:"idle_days,omitempty"`
}

func importText(raw any) string { value, _ := raw.(string); return strings.TrimSpace(value) }
func importStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}
func idleDays(raw any) int {
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
	default:
		return 0
	}
}
func required(value, key string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("args." + key + " required")
	}
	return nil
}
func bind[I, O any](operations *[]app.Operation, spec opapi.Spec, handler func(context.Context, I) (O, error)) error {
	spec.AllowUnknownInput = true
	op, err := app.NewOperation(spec, handler)
	if err == nil {
		*operations = append(*operations, op)
	}
	return err
}

// hygieneSchema is the hygiene read's request contract: the idle window is a
// number of days.
var hygieneSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"idle_days":{"type":["number","string"]}}}`)

func importSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"name":{"type":"string"},"body":{"type":"string"},"description":{"type":"string"},"agent":{"type":"string"},"triggers":{"type":"array","items":{"type":"string"}},"tools_required":{"type":"array","items":{"type":"string"}},"resources":{"type":["object","null"],"additionalProperties":{"type":"string"}}}}`)
}
func Operations(reads func(context.Context) *Service, lifecycle func(context.Context) *Lifecycle, curation func(context.Context) *Curation, observations func(context.Context) *Observations) ([]app.Operation, error) {
	if reads == nil || lifecycle == nil || curation == nil || observations == nil {
		return nil, errors.New("skill service providers required")
	}
	var operations []app.Operation
	bindings := []func() error{
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.SkillList, ReadOnly: true}, func(ctx context.Context, in ListInput) (ListOutput, error) { return reads(ctx).List(ctx, in) })
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.SkillGet, ReadOnly: true}, func(ctx context.Context, in GetInput) (GetOutput, error) {
				if err := required(in.ID, "id"); err != nil {
					return GetOutput{}, err
				}
				return reads(ctx).Get(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.SkillHistory, ReadOnly: true}, func(ctx context.Context, in GetInput) (HistoryOutput, error) {
				if err := required(in.ID, "id"); err != nil {
					return HistoryOutput{}, err
				}
				return observations(ctx).History(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.SkillFiles, ReadOnly: true}, func(ctx context.Context, in GetInput) (FilesOutput, error) {
				if err := required(in.ID, "id"); err != nil {
					return FilesOutput{}, err
				}
				return observations(ctx).Files(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.SkillReadFile, ReadOnly: true}, func(ctx context.Context, in ReadFileInput) (ReadFileOutput, error) {
				if err := required(in.ID, "id"); err != nil {
					return ReadFileOutput{}, err
				}
				if err := required(in.Path, "path"); err != nil {
					return ReadFileOutput{}, err
				}
				return observations(ctx).ReadFile(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.SkillHygiene, ReadOnly: true, InputSchema: hygieneSchema}, func(ctx context.Context, in HygieneRequestInput) (HygieneOutput, error) {
				return observations(ctx).Hygiene(ctx, HygieneInput{IdleDays: idleDays(in.IdleDays)})
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.SkillPromote}, func(ctx context.Context, in GetInput) (StatusOutput, error) {
				if err := required(in.ID, "id"); err != nil {
					return StatusOutput{}, err
				}
				return lifecycle(ctx).Promote(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.SkillQuarantine}, func(ctx context.Context, in ReasonInput) (StatusOutput, error) {
				if err := required(in.ID, "id"); err != nil {
					return StatusOutput{}, err
				}
				return lifecycle(ctx).Quarantine(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.SkillArchive}, func(ctx context.Context, in ReasonInput) (ArchiveOutput, error) {
				if err := required(in.ID, "id"); err != nil {
					return ArchiveOutput{}, err
				}
				return lifecycle(ctx).Archive(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.SkillRevert}, func(ctx context.Context, in GetInput) (RevertOutput, error) {
				if err := required(in.ID, "id"); err != nil {
					return RevertOutput{}, err
				}
				return lifecycle(ctx).Revert(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.SkillRestore}, func(ctx context.Context, in opapi.SkillRestoreRequest) (RestoreOutput, error) {
				if err := required(in.ID, "id"); err != nil {
					return RestoreOutput{}, err
				}
				if err := required(string(in.Status), "status"); err != nil {
					return RestoreOutput{}, err
				}
				return lifecycle(ctx).Restore(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.SkillShare}, func(ctx context.Context, in GetInput) (ShareOutput, error) {
				if err := required(in.ID, "id"); err != nil {
					return ShareOutput{}, err
				}
				return curation(ctx).Share(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.SkillReassign}, func(ctx context.Context, in ReassignInput) (ReassignOutput, error) {
				if err := required(in.ID, "id"); err != nil {
					return ReassignOutput{}, err
				}
				return curation(ctx).Reassign(ctx, in)
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: opapi.SkillImport, InputSchema: importSchema()}, func(ctx context.Context, in ImportRequestInput) (ImportOutput, error) {
				name, body := importText(in.Name), importText(in.Body)
				if name == "" || body == "" {
					return ImportOutput{}, errors.New("args.name and args.body required")
				}
				var resources map[string][]byte
				if len(in.Resources) > 0 {
					resources = make(map[string][]byte, len(in.Resources))
					for path, content := range in.Resources {
						resources[path] = []byte(content)
					}
				}
				return curation(ctx).Import(ctx, ImportInput{Name: name, Body: body, Description: importText(in.Description), Agent: importText(in.Agent), Triggers: importStrings(in.Triggers), ToolsRequired: importStrings(in.ToolsRequired), Resources: resources})
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
