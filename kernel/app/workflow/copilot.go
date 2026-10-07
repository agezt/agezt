// SPDX-License-Identifier: MIT
package workflow

import (
	"context"
	"errors"
	graphs "github.com/agezt/agezt/kernel/workflow"
	"strings"
	"time"
)

const CopilotTimeout = 3 * time.Minute

var ErrRefineBaseRequired = errors.New("args.workflow or args.ref required")

type Designer interface {
	DraftWorkflow(context.Context, string, string, string) (graphs.Workflow, error)
	RefineWorkflow(context.Context, string, graphs.Workflow, string) (graphs.Workflow, error)
}
type Copilot struct {
	reader      Reader
	designer    Designer
	correlation func() string
}

func NewCopilot(reader Reader, designer Designer, correlation func() string) *Copilot {
	return &Copilot{reader: reader, designer: designer, correlation: correlation}
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
	w, err := s.designer.DraftWorkflow(ctx, corr, in.Name, in.Description)
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
	w, err := r.copilot.designer.RefineWorkflow(ctx, corr, r.base, r.instruction)
	if err != nil {
		return CopilotOutput{}, err
	}
	return CopilotOutput{Workflow: ProjectFull(w), CorrelationID: corr}, nil
}
