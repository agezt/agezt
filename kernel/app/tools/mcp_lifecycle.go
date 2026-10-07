// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/mcp"
)

// MCPLifecycleWriter retains the runtime's durable registration and peer lifecycle.
type MCPLifecycleWriter interface {
	AddMCPServer(string, mcp.Server) (mcp.Server, error)
	AttachMCPServer(context.Context, string, string) (mcp.Server, []string, error)
	DetachMCPServer(string, string) error
	SetMCPServerEnabled(string, string, bool) (mcp.Server, error)
	RemoveMCPServer(string, string) (bool, error)
}
type MCPLifecycle struct {
	writer  MCPLifecycleWriter
	catalog *MCPCatalog
}

func NewMCPLifecycle(writer MCPLifecycleWriter, attached MCPAttachmentReader) *MCPLifecycle {
	return &MCPLifecycle{writer: writer, catalog: NewMCPCatalog(nil, attached)}
}

type MCPAddInput struct {
	Server        mcp.Server
	CorrelationID string
}
type MCPRefInput struct{ Ref, CorrelationID string }
type MCPSetEnabledInput struct {
	Ref, CorrelationID string
	Enabled            bool
}
type MCPServerOutput struct {
	Server MCPServerView `json:"server"`
}
type MCPAttachOutput struct {
	Server MCPServerView `json:"server"`
	Tools  []string      `json:"tools"`
}
type MCPDetachOutput struct {
	Detached bool `json:"detached"`
}
type MCPRemoveOutput struct {
	Removed bool `json:"removed"`
}

func mcpLifecycleCorrelation(ctx context.Context, explicit string) string {
	if owned := opapi.CorrelationFromContext(ctx); owned != "" {
		return owned
	}
	return explicit
}
func mcpLifecycleError(err error, ref string) error {
	if errors.Is(err, mcp.ErrNotFound) {
		return fmt.Errorf("unknown mcp server: %s", ref)
	}
	return err
}
func (s *MCPLifecycle) Add(ctx context.Context, in MCPAddInput) (MCPServerOutput, error) {
	if err := ctx.Err(); err != nil {
		return MCPServerOutput{}, err
	}
	srv, err := s.writer.AddMCPServer(mcpLifecycleCorrelation(ctx, in.CorrelationID), in.Server)
	if err != nil {
		return MCPServerOutput{}, err
	}
	return MCPServerOutput{Server: s.catalog.View(srv)}, nil
}
func (s *MCPLifecycle) Attach(ctx context.Context, in MCPRefInput) (MCPAttachOutput, error) {
	if err := ctx.Err(); err != nil {
		return MCPAttachOutput{}, err
	}
	srv, tools, err := s.writer.AttachMCPServer(ctx, mcpLifecycleCorrelation(ctx, in.CorrelationID), in.Ref)
	if err != nil {
		return MCPAttachOutput{}, mcpLifecycleError(err, in.Ref)
	}
	return MCPAttachOutput{Server: s.catalog.View(srv), Tools: tools}, nil
}
func (s *MCPLifecycle) Detach(ctx context.Context, in MCPRefInput) (MCPDetachOutput, error) {
	if err := ctx.Err(); err != nil {
		return MCPDetachOutput{}, err
	}
	if err := s.writer.DetachMCPServer(mcpLifecycleCorrelation(ctx, in.CorrelationID), in.Ref); err != nil {
		return MCPDetachOutput{}, err
	}
	return MCPDetachOutput{Detached: true}, nil
}
func (s *MCPLifecycle) SetEnabled(ctx context.Context, in MCPSetEnabledInput) (MCPServerOutput, error) {
	if err := ctx.Err(); err != nil {
		return MCPServerOutput{}, err
	}
	srv, err := s.writer.SetMCPServerEnabled(mcpLifecycleCorrelation(ctx, in.CorrelationID), in.Ref, in.Enabled)
	if err != nil {
		return MCPServerOutput{}, mcpLifecycleError(err, in.Ref)
	}
	return MCPServerOutput{Server: s.catalog.View(srv)}, nil
}
func (s *MCPLifecycle) Remove(ctx context.Context, in MCPRefInput) (MCPRemoveOutput, error) {
	if err := ctx.Err(); err != nil {
		return MCPRemoveOutput{}, err
	}
	removed, err := s.writer.RemoveMCPServer(mcpLifecycleCorrelation(ctx, in.CorrelationID), in.Ref)
	if err != nil {
		return MCPRemoveOutput{}, err
	}
	return MCPRemoveOutput{Removed: removed}, nil
}
