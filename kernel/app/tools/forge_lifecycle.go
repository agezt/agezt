// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/toolforge"
	"strings"
)

// ForgeWriter retains the kernel's durable lifecycle and script-test boundaries.
type ForgeWriter interface {
	DraftScriptTool(string, toolforge.ScriptTool) (toolforge.ScriptTool, error)
	UpdateScriptTool(string, string, func(*toolforge.ScriptTool)) (toolforge.ScriptTool, bool, error)
	TestScriptTool(context.Context, string, string, string) (toolforge.ScriptTool, string, error)
	PromoteScriptTool(string, string) (toolforge.ScriptTool, error)
	QuarantineScriptTool(string, string, string) (toolforge.ScriptTool, error)
	RemoveScriptTool(string, string) (bool, error)
}
type ForgeLifecycle struct{ writer ForgeWriter }

func NewForgeLifecycle(writer ForgeWriter) *ForgeLifecycle { return &ForgeLifecycle{writer: writer} }

type ForgeDraftInput struct {
	Tool          toolforge.ScriptTool
	CorrelationID string
}
type ForgeEditInput struct {
	Ref           string
	Tool          toolforge.ScriptTool
	CorrelationID string
}
type ForgeRefInput struct{ Ref, CorrelationID string }
type ForgeTestInput struct{ Ref, Sample, CorrelationID string }
type ForgeQuarantineInput struct{ Ref, Reason, CorrelationID string }
type ForgeMutationOutput struct {
	Tool ForgeItem `json:"tool"`
}
type ForgeTestOutput struct {
	Tool   ForgeItem `json:"tool"`
	OK     bool      `json:"ok"`
	Output string    `json:"output"`
}
type ForgeRemoveOutput struct {
	Removed bool `json:"removed"`
}

func forgeCorrelation(ctx context.Context, explicit string) string {
	if id := opapi.CorrelationFromContext(ctx); id != "" {
		return id
	}
	return explicit
}
func forgeLifecycleError(err error, ref string) error {
	if errors.Is(err, toolforge.ErrNotFound) {
		return fmt.Errorf("unknown script tool: %s", ref)
	}
	return err
}
func (s *ForgeLifecycle) Draft(ctx context.Context, in ForgeDraftInput) (ForgeMutationOutput, error) {
	if err := ctx.Err(); err != nil {
		return ForgeMutationOutput{}, err
	}
	saved, err := s.writer.DraftScriptTool(forgeCorrelation(ctx, in.CorrelationID), in.Tool)
	if err != nil {
		return ForgeMutationOutput{}, err
	}
	return ForgeMutationOutput{Tool: ForgeToolView(saved)}, nil
}
func (s *ForgeLifecycle) Edit(ctx context.Context, in ForgeEditInput) (ForgeMutationOutput, error) {
	if err := ctx.Err(); err != nil {
		return ForgeMutationOutput{}, err
	}
	st, found, err := s.writer.UpdateScriptTool(forgeCorrelation(ctx, in.CorrelationID), in.Ref, func(dst *toolforge.ScriptTool) {
		if strings.TrimSpace(in.Tool.Description) != "" {
			dst.Description = in.Tool.Description
		}
		if strings.TrimSpace(in.Tool.Language) != "" {
			dst.Language = in.Tool.Language
		}
		if in.Tool.Code != "" {
			dst.Code = in.Tool.Code
		}
		if strings.TrimSpace(in.Tool.InputSchema) != "" {
			dst.InputSchema = in.Tool.InputSchema
		}
	})
	if err != nil {
		return ForgeMutationOutput{}, err
	}
	if !found {
		return ForgeMutationOutput{}, fmt.Errorf("unknown script tool: %s", in.Ref)
	}
	return ForgeMutationOutput{Tool: ForgeToolView(st)}, nil
}
func (s *ForgeLifecycle) Test(ctx context.Context, in ForgeTestInput) (ForgeTestOutput, error) {
	if err := ctx.Err(); err != nil {
		return ForgeTestOutput{}, err
	}
	st, out, err := s.writer.TestScriptTool(ctx, forgeCorrelation(ctx, in.CorrelationID), in.Ref, in.Sample)
	if err != nil {
		return ForgeTestOutput{}, forgeLifecycleError(err, in.Ref)
	}
	return ForgeTestOutput{Tool: ForgeToolView(st), OK: st.TestedOK, Output: out}, nil
}
func (s *ForgeLifecycle) Promote(ctx context.Context, in ForgeRefInput) (ForgeMutationOutput, error) {
	if err := ctx.Err(); err != nil {
		return ForgeMutationOutput{}, err
	}
	st, err := s.writer.PromoteScriptTool(forgeCorrelation(ctx, in.CorrelationID), in.Ref)
	if err != nil {
		return ForgeMutationOutput{}, forgeLifecycleError(err, in.Ref)
	}
	return ForgeMutationOutput{Tool: ForgeToolView(st)}, nil
}
func (s *ForgeLifecycle) Quarantine(ctx context.Context, in ForgeQuarantineInput) (ForgeMutationOutput, error) {
	if err := ctx.Err(); err != nil {
		return ForgeMutationOutput{}, err
	}
	st, err := s.writer.QuarantineScriptTool(forgeCorrelation(ctx, in.CorrelationID), in.Ref, in.Reason)
	if err != nil {
		return ForgeMutationOutput{}, forgeLifecycleError(err, in.Ref)
	}
	return ForgeMutationOutput{Tool: ForgeToolView(st)}, nil
}
func (s *ForgeLifecycle) Remove(ctx context.Context, in ForgeRefInput) (ForgeRemoveOutput, error) {
	if err := ctx.Err(); err != nil {
		return ForgeRemoveOutput{}, err
	}
	removed, err := s.writer.RemoveScriptTool(forgeCorrelation(ctx, in.CorrelationID), in.Ref)
	if err != nil {
		return ForgeRemoveOutput{}, err
	}
	return ForgeRemoveOutput{Removed: removed}, nil
}
