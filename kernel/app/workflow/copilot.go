// SPDX-License-Identifier: MIT
package workflow

import (
	"context"
	"errors"
	graphs "github.com/agezt/agezt/kernel/modules/workflows/workflow"
	"strings"
	"time"
)

const CopilotTimeout = 3 * time.Minute

var ErrRefineBaseRequired = errors.New("args.workflow or args.ref required")

// Designer is the copilot's face over the module's engine (W3.76u): it
// speaks the contract's engine vocabulary, so the module's face binds here
// directly. It is nil when the daemon holds no engine, and the copilot then
// refuses rather than calling into nothing.
type Designer interface {
	Draft(context.Context, string, string, string) (graphs.Workflow, error)
	Refine(context.Context, string, graphs.Workflow, string) (graphs.Workflow, error)
}
type Copilot struct {
	reader      Reader
	designer    Designer
	correlation func() string
}

func NewCopilot(reader Reader, designer Designer, correlation func() string) *Copilot {
	return &Copilot{reader: reader, designer: designer, correlation: correlation}
}

// designer2 returns the designer, or the refusal.
//
// The designer is the module's engine, and a daemon opened without one holds
// no engine at all — so the copilot says so instead of calling into nothing.
func (s *Copilot) designer2() (Designer, error) {
	if s.designer == nil {
		return nil, graphs.ErrNoEngine
	}
	return s.designer, nil
}

type DraftInput struct{ Name, Description string }
type CopilotOutput struct {
	Workflow      FullRecord `json:"workflow"`
	CorrelationID string     `json:"correlation_id"`
}

// Blocking copilot work inherits caller cancellation/values/deadline and the
// owned operation identity, bounded by the existing copilot ceiling.
func (s *Copilot) Draft(parent context.Context, in DraftInput) (CopilotOutput, error) {
	if err := parent.Err(); err != nil {
		return CopilotOutput{}, err
	}
	corr := operationCorrelation(parent, "", s.correlation)
	ctx, cancel := context.WithTimeout(parent, CopilotTimeout)
	defer cancel()
	designer, err := s.designer2()
	if err != nil {
		return CopilotOutput{}, err
	}
	w, err := designer.Draft(ctx, corr, in.Name, in.Description)
	if err != nil {
		return CopilotOutput{}, err
	}
	return CopilotOutput{Workflow: ProjectFull(w), CorrelationID: corr}, nil
}

type RefineInput struct {
	Posted           *graphs.Workflow
	Ref, Instruction string
}
type Refinement struct {
	copilot     *Copilot
	base        graphs.Workflow
	instruction string
}

// Posted graph wins without a store lookup; absent/null graph resolves the
// trimmed reference before correlation allocation or designer calls.
func (s *Copilot) PrepareRefine(in RefineInput) (Refinement, error) {
	var base graphs.Workflow
	if in.Posted != nil {
		base = *in.Posted
	} else {
		if s.reader == nil {
			return Refinement{}, errNoWorkflows
		}
		ref := strings.TrimSpace(in.Ref)
		if ref == "" {
			return Refinement{}, ErrRefineBaseRequired
		}
		w, found := s.reader.Get(ref)
		if !found {
			return Refinement{}, UnknownWorkflowError{Ref: in.Ref}
		}
		base = w
	}
	return Refinement{copilot: s, base: base, instruction: in.Instruction}, nil
}
func (r Refinement) Refine(parent context.Context) (CopilotOutput, error) {
	if err := parent.Err(); err != nil {
		return CopilotOutput{}, err
	}
	corr := operationCorrelation(parent, "", r.copilot.correlation)
	ctx, cancel := context.WithTimeout(parent, CopilotTimeout)
	defer cancel()
	designer, err := r.copilot.designer2()
	if err != nil {
		return CopilotOutput{}, err
	}
	w, err := designer.Refine(ctx, corr, r.base, r.instruction)
	if err != nil {
		return CopilotOutput{}, err
	}
	return CopilotOutput{Workflow: ProjectFull(w), CorrelationID: corr}, nil
}
