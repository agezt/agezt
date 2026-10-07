// SPDX-License-Identifier: MIT
package workflow

import (
	"context"
	"errors"
	"fmt"
	graphs "github.com/agezt/agezt/kernel/workflow"
)

// Writer preserves runtime facade publication and core store identity/rollback.
type Writer interface {
	SaveWorkflow(string, graphs.Workflow) (graphs.Workflow, bool, error)
	RestoreWorkflow(string, graphs.Workflow, string) (graphs.Workflow, bool, error)
	SetWorkflowEnabled(string, string, bool) (graphs.Workflow, error)
	RemoveWorkflow(string, string) (bool, error)
}
type Lifecycle struct{ writer Writer }

func NewLifecycle(writer Writer) *Lifecycle { return &Lifecycle{writer: writer} }

type SaveInput struct {
	CorrelationID string
	Workflow      graphs.Workflow
}
type SaveOutput struct {
	Workflow FullRecord `json:"workflow"`
	Created  bool       `json:"created"`
}

func (s *Lifecycle) Save(ctx context.Context, in SaveInput) (SaveOutput, error) {
	if err := ctx.Err(); err != nil {
		return SaveOutput{}, err
	}
	w, created, err := s.writer.SaveWorkflow(operationCorrelation(ctx, in.CorrelationID, nil), in.Workflow)
	if err != nil {
		return SaveOutput{}, err
	}
	return SaveOutput{Workflow: ProjectFull(w), Created: created}, nil
}

type RestoreInput struct {
	CorrelationID string
	Workflow      graphs.Workflow
	Reason        string
}

func (s *Lifecycle) Restore(ctx context.Context, in RestoreInput) (SaveOutput, error) {
	if err := ctx.Err(); err != nil {
		return SaveOutput{}, err
	}
	w, created, err := s.writer.RestoreWorkflow(operationCorrelation(ctx, in.CorrelationID, nil), in.Workflow, in.Reason)
	if err != nil {
		return SaveOutput{}, err
	}
	return SaveOutput{Workflow: ProjectFull(w), Created: created}, nil
}

type EnableInput struct {
	CorrelationID string
	Ref           string
	Enabled       bool
}
type EnableOutput struct {
	Workflow Record `json:"workflow"`
}

// UnknownWorkflowError keeps the wire's raw reference and the original domain
// classification without adding text from an underlying sentinel.
type UnknownWorkflowError struct{ Ref string }

func (e UnknownWorkflowError) Error() string { return fmt.Sprintf("unknown workflow: %s", e.Ref) }
func (e UnknownWorkflowError) Unwrap() error { return graphs.ErrNotFound }
func (s *Lifecycle) SetEnabled(ctx context.Context, in EnableInput) (EnableOutput, error) {
	if err := ctx.Err(); err != nil {
		return EnableOutput{}, err
	}
	w, err := s.writer.SetWorkflowEnabled(operationCorrelation(ctx, in.CorrelationID, nil), in.Ref, in.Enabled)
	if err != nil {
		if errors.Is(err, graphs.ErrNotFound) {
			return EnableOutput{}, UnknownWorkflowError{Ref: in.Ref}
		}
		return EnableOutput{}, err
	}
	return EnableOutput{Workflow: Project(w)}, nil
}

type RemoveInput struct {
	CorrelationID string
	Ref           string
}
type RemoveOutput struct {
	Removed bool `json:"removed"`
}

func (s *Lifecycle) Remove(ctx context.Context, in RemoveInput) (RemoveOutput, error) {
	if err := ctx.Err(); err != nil {
		return RemoveOutput{}, err
	}
	removed, err := s.writer.RemoveWorkflow(operationCorrelation(ctx, in.CorrelationID, nil), in.Ref)
	if err != nil {
		return RemoveOutput{}, err
	}
	return RemoveOutput{Removed: removed}, nil
}
