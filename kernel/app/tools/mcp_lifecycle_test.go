// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/mcp"
	"reflect"
	"strings"
	"testing"
)

type mcpLifecyclePort struct {
	calls     []string
	corr, ref string
	enabled   bool
	input     mcp.Server
	output    mcp.Server
	names     []string
	ctx       context.Context
	err       error
	removed   bool
	views     int
}

func (p *mcpLifecyclePort) AddMCPServer(corr string, srv mcp.Server) (mcp.Server, error) {
	p.calls = append(p.calls, "add")
	p.corr = corr
	p.input = srv
	return p.output, p.err
}
func (p *mcpLifecyclePort) AttachMCPServer(ctx context.Context, corr, ref string) (mcp.Server, []string, error) {
	p.calls = append(p.calls, "attach")
	p.ctx = ctx
	p.corr = corr
	p.ref = ref
	return p.output, p.names, p.err
}
func (p *mcpLifecyclePort) DetachMCPServer(corr, ref string) error {
	p.calls = append(p.calls, "detach")
	p.corr = corr
	p.ref = ref
	return p.err
}
func (p *mcpLifecyclePort) SetMCPServerEnabled(corr, ref string, enabled bool) (mcp.Server, error) {
	p.calls = append(p.calls, "enabled")
	p.corr = corr
	p.ref = ref
	p.enabled = enabled
	return p.output, p.err
}
func (p *mcpLifecyclePort) RemoveMCPServer(corr, ref string) (bool, error) {
	p.calls = append(p.calls, "remove")
	p.corr = corr
	p.ref = ref
	return p.removed, p.err
}
func (p *mcpLifecyclePort) MCPAttached() map[string]int {
	p.views++
	return map[string]int{p.output.Name: 0}
}
func TestMCPLifecycleFoundationProjectionAndWriterArguments(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "owned")
	row := mcp.Server{ID: "id", Name: "raw-name", Command: "never-executed", Args: []string{" raw "}, URL: " raw-url ", Enabled: true, Description: " raw description ", Env: map[string]string{"Z": "owned-private-env", "A": "owned-private-env"}, Headers: map[string]string{"X": "owned-private-header"}, Lazy: true, ToolAllow: []string{"owned"}, CreatedMS: 9007199254740993, UpdatedMS: 9223372036854775807}
	for _, op := range []string{"add", "attach", "detach", "enabled", "remove"} {
		t.Run(op, func(t *testing.T) {
			p := &mcpLifecyclePort{output: row, names: []string{" second ", "first"}, removed: true}
			s := NewMCPLifecycle(p, p)
			var out any
			var err error
			switch op {
			case "add":
				out, err = s.Add(ctx, MCPAddInput{Server: row, CorrelationID: "corr"})
			case "attach":
				out, err = s.Attach(ctx, MCPRefInput{Ref: " raw-ref ", CorrelationID: "corr"})
			case "detach":
				out, err = s.Detach(ctx, MCPRefInput{Ref: " raw-ref ", CorrelationID: "corr"})
			case "enabled":
				out, err = s.SetEnabled(ctx, MCPSetEnabledInput{Ref: " raw-ref ", CorrelationID: "corr", Enabled: true})
			case "remove":
				out, err = s.Remove(ctx, MCPRefInput{Ref: " raw-ref ", CorrelationID: "corr"})
			}
			if err != nil || !reflect.DeepEqual(p.calls, []string{op}) || p.corr != "corr" {
				t.Fatal(out, err, p)
			}
			if op == "add" {
				if !reflect.DeepEqual(p.input, row) {
					t.Fatal(p.input)
				}
			} else if p.ref != " raw-ref " {
				t.Fatal(p.ref)
			}
			switch op {
			case "add", "enabled":
				v := out.(MCPServerOutput)
				if !reflect.DeepEqual(v.Server, NewMCPCatalog(nil, &mcpLifecyclePort{output: row}).View(row)) {
					t.Fatal(v)
				}
			case "attach":
				v := out.(MCPAttachOutput)
				if p.ctx != ctx || !reflect.DeepEqual(v.Tools, p.names) || !v.Server.Attached || v.Server.ToolCount == nil || *v.Server.ToolCount != 0 {
					t.Fatal(v, p.ctx)
				}
			case "detach":
				if !out.(MCPDetachOutput).Detached || p.views != 0 {
					t.Fatal(out, p.views)
				}
			case "remove":
				if !out.(MCPRemoveOutput).Removed || p.views != 0 {
					t.Fatal(out, p.views)
				}
			}
			if op == "enabled" && !p.enabled {
				t.Fatal("enabled flag lost")
			}
			raw, _ := json.Marshal(out)
			if strings.Contains(string(raw), "owned-private-") {
				t.Fatal("private registration values leaked")
			}
			if op == "add" || op == "attach" || op == "enabled" {
				if !strings.Contains(string(raw), `"created_ms":9007199254740993`) || !strings.Contains(string(raw), `"updated_ms":9223372036854775807`) || p.views != 1 {
					t.Fatal(string(raw), p.views)
				}
			}
		})
	}
}
func TestMCPLifecycleFoundationErrorsAndAbsentRemove(t *testing.T) {
	owned := errors.New("owned failure")
	for _, op := range []string{"add", "attach", "detach", "enabled", "remove"} {
		for _, cause := range []error{owned, fmt.Errorf("wrapped: %w", mcp.ErrNotFound)} {
			p := &mcpLifecyclePort{err: cause}
			s := NewMCPLifecycle(p, p)
			var err error
			switch op {
			case "add":
				_, err = s.Add(context.Background(), MCPAddInput{})
			case "attach":
				_, err = s.Attach(context.Background(), MCPRefInput{Ref: " raw "})
			case "detach":
				_, err = s.Detach(context.Background(), MCPRefInput{Ref: " raw "})
			case "enabled":
				_, err = s.SetEnabled(context.Background(), MCPSetEnabledInput{Ref: " raw "})
			case "remove":
				_, err = s.Remove(context.Background(), MCPRefInput{Ref: " raw "})
			}
			if cause != owned && (op == "attach" || op == "enabled") {
				if err == nil || err.Error() != "unknown mcp server:  raw " {
					t.Fatal(op, err)
				}
			} else if err != cause {
				t.Fatal(op, err)
			}
			if p.views != 0 || !reflect.DeepEqual(p.calls, []string{op}) {
				t.Fatal(p)
			}
		}
	}
	p := &mcpLifecyclePort{}
	out, err := NewMCPLifecycle(p, p).Remove(context.Background(), MCPRefInput{Ref: "absent"})
	if err != nil || out.Removed || p.views != 0 {
		t.Fatal(out, err)
	}
}
