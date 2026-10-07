// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"github.com/agezt/agezt/kernel/toolbox"
)

// ToolboxReader exposes host discovery while keeping process execution in the host.
type ToolboxReader interface {
	Detect(context.Context) toolbox.Inventory
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
func (s *ToolboxReads) Detect(ctx context.Context, _ ToolboxDetectInput) (toolbox.Inventory, error) {
	return s.reader.Detect(ctx), nil
}

// Outdated preserves legacy map-key membership and unspecified iteration order.
func (s *ToolboxReads) Outdated(ctx context.Context, _ ToolboxOutdatedInput) (ToolboxOutdatedOutput, error) {
	out := s.reader.Outdated(ctx)
	names := make([]string, 0, len(out))
	for name := range out {
		names = append(names, name)
	}
	return ToolboxOutdatedOutput{Outdated: names, Count: len(names)}, nil
}
