// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"github.com/agezt/agezt/kernel/contract/toolboxapi"
)

// ToolboxReader exposes host discovery while keeping process execution in the host.
//
// The rows are the CONTRACT's (W5.4b): the operation schema is built from
// them, and a schema may not name a module.
type ToolboxReader interface {
	Detect(context.Context) toolboxapi.Inventory
	Outdated(context.Context) map[string]bool
}
type ToolboxReads struct{ reader ToolboxReader }

func NewToolboxReads(reader ToolboxReader) *ToolboxReads { return &ToolboxReads{reader: reader} }

type ToolboxDetectInput struct{}
type ToolboxOutdatedInput struct{}
type ToolboxOutdatedOutput struct {
	Outdated []string `json:"outdated"`
	Count    int      `json:"count"`
}

// Detect retains the host's exact snapshot, including optional fields and nil slices.
//
// A daemon with no toolbox port answers "the toolbox is not available on this
// daemon" — it does not answer with an empty inventory, because "this daemon
// cannot inspect the machine" and "this machine has no tools" are different
// facts.
func (s *ToolboxReads) Detect(ctx context.Context, _ ToolboxDetectInput) (toolboxapi.Inventory, error) {
	if s.reader == nil {
		return toolboxapi.Inventory{}, toolboxapi.ErrUnavailable
	}
	return s.reader.Detect(ctx), nil
}

// Outdated preserves legacy map-key membership and unspecified iteration order.
func (s *ToolboxReads) Outdated(ctx context.Context, _ ToolboxOutdatedInput) (ToolboxOutdatedOutput, error) {
	if s.reader == nil {
		return ToolboxOutdatedOutput{}, toolboxapi.ErrUnavailable
	}
	out := s.reader.Outdated(ctx)
	names := make([]string, 0, len(out))
	for name := range out {
		names = append(names, name)
	}
	return ToolboxOutdatedOutput{Outdated: names, Count: len(names)}, nil
}
