// SPDX-License-Identifier: MIT
package workflow

import (
	"context"
	"errors"
	"fmt"
	graphs "github.com/agezt/agezt/kernel/modules/workflows/workflow"
)

// Writer preserves runtime facade publication and core store identity/rollback.
//
// It is the MODULE's engine (W3.76u): the application's port speaks the
// contract's engine vocabulary, so the module's face binds here directly —
// there is no adapter in between. It is nil when the daemon holds no engine,
// and the service then refuses rather than calling into nothing.
type Writer interface {
	Save(string, graphs.Workflow) (graphs.Workflow, bool, error)
	Restore(string, graphs.Workflow, string) (graphs.Workflow, bool, error)
	SetEnabled(string, string, bool) (graphs.Workflow, error)
	Remove(string, string) (bool, error)
}
type Lifecycle struct{ writer Writer }

func NewLifecycle(writer Writer) *Lifecycle { return &Lifecycle{writer: writer} }

// engine returns the writer, or the refusal.
//
// The writer is the module's engine, and a daemon opened without one holds no
// engine at all — so the application says so instead of calling into nothing.
func (s *Lifecycle) engine() (Writer, error) {
	if s.writer == nil {
		return nil, graphs.ErrNoEngine
	}
	return s.writer, nil
}

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
	writer, err := s.engine()
	if err != nil {
		return SaveOutput{}, err
	}
	w, created, err := writer.Save(operationCorrelation(ctx, in.CorrelationID, nil), in.Workflow)
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
	writer, err := s.engine()
	if err != nil {
		return SaveOutput{}, err
	}
	w, created, err := writer.Restore(operationCorrelation(ctx, in.CorrelationID, nil), in.Workflow, in.Reason)
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
	writer, err := s.engine()
	if err != nil {
		return EnableOutput{}, err
	}
	w, err := writer.SetEnabled(operationCorrelation(ctx, in.CorrelationID, nil), in.Ref, in.Enabled)
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
	writer, err := s.engine()
	if err != nil {
		return RemoveOutput{}, err
	}
	removed, err := writer.Remove(operationCorrelation(ctx, in.CorrelationID, nil), in.Ref)
	if err != nil {
		return RemoveOutput{}, err
	}
	return RemoveOutput{Removed: removed}, nil
}
