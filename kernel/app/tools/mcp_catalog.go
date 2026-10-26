// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"sort"
)

// MCPRegistrationReader is the catalog's read port for the registry — the
// CONTRACT's reader port (W3.58), so the catalog names the contract and not
// a shape of its own.
type MCPRegistrationReader = mcpapi.RegistrationReader

type MCPAttachmentReader interface{ MCPAttached() map[string]int }
type MCPCatalog struct {
	registrations MCPRegistrationReader
	attached      MCPAttachmentReader
}

func NewMCPCatalog(registrations MCPRegistrationReader, attached MCPAttachmentReader) *MCPCatalog {
	return &MCPCatalog{registrations: registrations, attached: attached}
}

type MCPListInput struct{}
type MCPListOutput struct {
	Servers       []MCPServerView `json:"servers"`
	Count         int             `json:"count"`
	AttachedCount int             `json:"attached_count"`
}

// List keeps the original registration order and attachment-read cadence:
// one count snapshot plus one fresh status snapshot per registration view.
//
// A catalog with NO reader is a daemon with no registry: the answer is "no
// servers", not a crash. The kernel's own reader is nil-safe, so this is the
// belt to that braces — and the catalog must survive a host that hands it
// nothing.
func (s *MCPCatalog) List(_ context.Context, _ MCPListInput) (MCPListOutput, error) {
	var servers []mcpapi.Server
	if s.registrations != nil {
		servers = s.registrations.List()
	}
	attached := s.attachmentSnapshot()
	out := make([]MCPServerView, 0, len(servers))
	for _, srv := range servers {
		out = append(out, s.View(srv))
	}
	return MCPListOutput{Servers: out, Count: len(out), AttachedCount: len(attached)}, nil
}

// attachmentSnapshot reads the attachment status — the nil-safe answer for a
// host with no attachment reader ("nothing is attached").
func (s *MCPCatalog) attachmentSnapshot() map[string]int {
	if s.attached == nil {
		return nil
	}
	return s.attached.MCPAttached()
}

// View projects only public registration fields, copying collection values so
// callers cannot mutate the selected registration through its read projection.
func (s *MCPCatalog) View(srv mcpapi.Server) MCPServerView {
	row := MCPServerView{ID: srv.ID, Name: srv.Name, Command: srv.Command, Args: append([]string(nil), srv.Args...), URL: srv.URL, Enabled: srv.Enabled, Description: srv.Description, Lazy: srv.Lazy, ToolAllow: append([]string(nil), srv.ToolAllow...), CreatedMS: srv.CreatedMS, UpdatedMS: srv.UpdatedMS, Transport: "stdio"}
	if len(srv.Env) > 0 {
		row.EnvKeys = make([]string, 0, len(srv.Env))
		for key := range srv.Env {
			row.EnvKeys = append(row.EnvKeys, key)
		}
		sort.Strings(row.EnvKeys)
	}
	if len(srv.Headers) > 0 {
		row.HeaderKeys = make([]string, 0, len(srv.Headers))
		for key := range srv.Headers {
			row.HeaderKeys = append(row.HeaderKeys, key)
		}
		sort.Strings(row.HeaderKeys)
	}
	if srv.URL != "" {
		row.Transport = "http"
	}
	attached := s.attachmentSnapshot()
	if count, live := attached[srv.Name]; live {
		row.Attached = true
		row.ToolCount = &count
	}
	return row
}
