// SPDX-License-Identifier: MIT

package mcp

// Provenance: the MCP REGISTRY SERVICE (W3.71). The lifecycle over the
//             registered servers and the bridge that folds an attached
//             server's tools into a run's tool map. Moved out of
//             kernel/runtime (the daemon used to hold the lifecycle, the
//             live connections and the bridge itself) into the extensions
//             module, which owns the MCP registry.
//
// The daemon keeps its own names as forwarders, so every caller outside the
// runtime is untouched. What moved is WHERE the answer comes from: the
// registry's OPERATIONS live with the registry's DATA now.

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/event"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
)

// Service is the extensions module's MCP registry: the durable registrations,
// the live attachments and the bridge that hands an attached server's tools
// to a run.
//
// It holds the host's PORTS (the registry's store, the two dialers, the
// publisher) and nothing else of the host's. A host that handed over no
// registry port answers "no registry" rather than calling into nothing.
type Service struct {
	registry   mcpapi.Registry
	dialer     mcpapi.Dialer
	httpDialer mcpapi.HTTPDialer
	publish    func(spec event.Spec)

	// mu guards conns — the LIVE attachments (server name → connection).
	mu    sync.Mutex
	conns map[string]mcpapi.Conn
}

// NewService builds the registry service over the host's dependencies.
//
// The service satisfies the module's api (extapi.MCPRegistryService); the
// constructor returns the concrete type so the host can hold either.
func NewService(deps extapi.MCPRegistryDependencies) *Service {
	return &Service{
		registry:   deps.Registry,
		dialer:     deps.Dialer,
		httpDialer: deps.HTTPDialer,
		publish:    deps.Publish,
		conns:      map[string]mcpapi.Conn{},
	}
}

// emit publishes one event on the host's bus, when the host has one.
func (s *Service) emit(spec event.Spec) {
	if s.publish == nil {
		return
	}
	s.publish(spec)
}

// Add validates and persists a new MCP server registration, journaling
// mcp.added. Registration alone spawns nothing — attach does.
func (s *Service) Add(corr string, srv mcpapi.Server) (mcpapi.Server, error) {
	if s.registry == nil {
		return mcpapi.Server{}, mcpapi.ErrNoRegistry
	}
	saved, err := s.registry.Add(srv)
	if err != nil {
		return mcpapi.Server{}, err
	}
	transport := "stdio"
	if saved.URL != "" {
		transport = "http"
	}
	s.emit(event.Spec{
		Subject: "mcp." + saved.Name, Kind: event.KindMCPAdded, Actor: "mcp",
		CorrelationID: corr,
		Payload:       map[string]any{"id": saved.ID, "name": saved.Name, "transport": transport, "command": saved.Command, "args": saved.Args, "url": saved.URL},
	})
	return saved, nil
}

// SetEnabled flips a server's auto-attach-at-start flag, journaling
// mcp.updated. It does not touch a live attachment.
func (s *Service) SetEnabled(corr, ref string, enabled bool) (mcpapi.Server, error) {
	if s.registry == nil {
		return mcpapi.Server{}, mcpapi.ErrNoRegistry
	}
	srv, err := s.registry.SetEnabled(ref, enabled)
	if err != nil {
		return mcpapi.Server{}, err
	}
	s.emit(event.Spec{
		Subject: "mcp." + srv.Name, Kind: event.KindMCPUpdated, Actor: "mcp",
		CorrelationID: corr,
		Payload:       map[string]any{"id": srv.ID, "name": srv.Name, "enabled": enabled},
	})
	return srv, nil
}

// Attach spawns a registered server, completes the MCP handshake,
// discovers its tools, and journals mcp.attached with the discovered tool
// names — from the next run on, every agent is offered them as
// mcp_<name>_<tool>. Attaching an already-attached server is an error
// (detach first).
func (s *Service) Attach(ctx context.Context, corr, ref string) (mcpapi.Server, []string, error) {
	if s.registry == nil {
		return mcpapi.Server{}, nil, mcpapi.ErrNoRegistry
	}
	srv, found := s.registry.Get(ref)
	if !found {
		return mcpapi.Server{}, nil, mcpapi.ErrNotFound
	}
	s.mu.Lock()
	if _, live := s.conns[srv.Name]; live {
		s.mu.Unlock()
		return mcpapi.Server{}, nil, fmt.Errorf("mcp: mcp server %s is already attached", srv.Name)
	}
	s.mu.Unlock()

	conn, err := s.dial(ctx, srv)
	if err != nil {
		return mcpapi.Server{}, nil, err
	}

	s.mu.Lock()
	if _, live := s.conns[srv.Name]; live { // raced a concurrent attach
		s.mu.Unlock()
		_ = conn.Close()
		return mcpapi.Server{}, nil, fmt.Errorf("mcp: mcp server %s is already attached", srv.Name)
	}
	s.conns[srv.Name] = conn
	s.mu.Unlock()

	names := make([]string, 0, len(conn.Tools()))
	for _, t := range conn.Tools() {
		names = append(names, mcpToolName(srv.Name, t.Name))
	}
	s.emit(event.Spec{
		Subject: "mcp." + srv.Name, Kind: event.KindMCPAttached, Actor: "mcp",
		CorrelationID: corr,
		Payload:       map[string]any{"id": srv.ID, "name": srv.Name, "tools": names},
	})
	return srv, names, nil
}

// dial attaches one registered server over the transport its registration
// implies: a remote endpoint (URL) speaks Streamable HTTP (M904), everything
// else is a spawned stdio process (M796).
//
// The DIALERS are the host's seams (W3.58): the implementations live in this
// module and the host may not name them, so the daemon injects them. A
// service with no dialer refuses the attach rather than calling into nothing
// — and the refusal is explicit, because a nil func value would panic at the
// call.
func (s *Service) dial(ctx context.Context, srv mcpapi.Server) (mcpapi.Conn, error) {
	if strings.TrimSpace(srv.URL) != "" {
		if s.httpDialer == nil {
			return nil, fmt.Errorf("mcp: no HTTP MCP dialer is configured")
		}
		return s.httpDialer(ctx, srv.URL, srv.Headers)
	}
	if s.dialer == nil {
		return nil, fmt.Errorf("mcp: no stdio MCP dialer is configured")
	}
	return s.dialer(ctx, srv.Command, srv.Args, srv.Env)
}

// Detach closes a live attachment — the kill switch: its tools vanish from
// the next run on. Journals mcp.detached.
func (s *Service) Detach(corr, ref string) error {
	// Resolve a registry ref (id or name) to the live-connection key, but
	// also accept the bare name of a connection whose registry row is gone.
	name := ref
	if s.registry != nil {
		if srv, found := s.registry.Get(ref); found {
			name = srv.Name
		}
	}
	s.mu.Lock()
	conn, live := s.conns[name]
	delete(s.conns, name)
	s.mu.Unlock()
	if !live {
		return fmt.Errorf("mcp: mcp server %s is not attached", name)
	}
	_ = conn.Close()
	s.emit(event.Spec{
		Subject: "mcp." + name, Kind: event.KindMCPDetached, Actor: "mcp",
		CorrelationID: corr,
		Payload:       map[string]any{"name": name},
	})
	return nil
}

// Remove deletes a registration (detaching it first when live), journaling
// mcp.removed. Returns whether it existed.
func (s *Service) Remove(corr, ref string) (bool, error) {
	if s.registry == nil {
		return false, mcpapi.ErrNoRegistry
	}
	if srv, found := s.registry.Get(ref); found {
		s.mu.Lock()
		_, live := s.conns[srv.Name]
		s.mu.Unlock()
		if live {
			_ = s.Detach(corr, srv.Name)
		}
	}
	gone, ok, err := s.registry.Remove(ref)
	if err != nil {
		return false, err
	}
	if ok {
		s.emit(event.Spec{
			Subject: "mcp." + gone.Name, Kind: event.KindMCPRemoved, Actor: "mcp",
			CorrelationID: corr,
			Payload:       map[string]any{"id": gone.ID, "name": gone.Name},
		})
	}
	return ok, nil
}

// AttachEnabled attaches every enabled registration — the daemon's boot
// path. Failures are per-server and reported, never fatal: one broken server
// must not take the others (or the daemon) down.
func (s *Service) AttachEnabled(ctx context.Context) (attached []string, failures map[string]error) {
	failures = map[string]error{}
	for _, srv := range s.registrations() {
		if !srv.Enabled {
			continue
		}
		if _, _, err := s.Attach(ctx, "", srv.Name); err != nil {
			failures[srv.Name] = err
			continue
		}
		attached = append(attached, srv.Name)
	}
	return attached, failures
}

// registrations returns every registered server — the nil-safe read.
func (s *Service) registrations() []mcpapi.Server {
	if s.registry == nil {
		return nil
	}
	return s.registry.List()
}

// Attached returns the names of currently-attached servers with their
// bridged tool counts — the status surface for `agt mcp list` and the
// console.
func (s *Service) Attached() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]int, len(s.conns))
	for name, conn := range s.conns {
		out[name] = len(conn.Tools())
	}
	return out
}

// Close detaches everything — the host's shutdown path.
func (s *Service) Close() {
	s.mu.Lock()
	conns := s.conns
	s.conns = map[string]mcpapi.Conn{}
	s.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}
