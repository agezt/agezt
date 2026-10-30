// SPDX-License-Identifier: MIT

package mcp

// The MCP REGISTRY SERVICE's own tests (W3.71).
//
// The daemon used to hold the registry's lifecycle — the store AND the
// operations over it, the live attachments and the bridge. These tests pin
// the behaviour where it now lives: the module answers the lifecycle, the
// daemon keeps the names.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/event"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
)

// The compile-time assertion: the service IS the module's face.
var _ extapi.MCPRegistryService = (*Service)(nil)

// recorder collects the events the service publishes.
type recorder struct{ specs []event.Spec }

func (r *recorder) publish(spec event.Spec) { r.specs = append(r.specs, spec) }

// kinds returns the published event kinds, in order.
func (r *recorder) kinds() []string {
	out := make([]string, 0, len(r.specs))
	for _, s := range r.specs {
		out = append(out, string(s.Kind))
	}
	return out
}

// fakeConn is a canned MCP attachment: fixed tools, recorded calls.
type fakeConn struct {
	tools  []mcpapi.ToolDef
	calls  []string
	out    string
	isErr  bool
	err    error
	closed bool
}

func (c *fakeConn) Tools() []mcpapi.ToolDef { return c.tools }

func (c *fakeConn) Call(_ context.Context, tool string, _ json.RawMessage) (string, bool, error) {
	c.calls = append(c.calls, tool)
	return c.out, c.isErr, c.err
}

func (c *fakeConn) Close() error { c.closed = true; return nil }

// newService builds the service over a real store (the module's own) and the
// given dialers.
func newService(t *testing.T, rec *recorder, dialer mcpapi.Dialer, httpDialer mcpapi.HTTPDialer) (*Service, *Store) {
	t.Helper()
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("open the MCP registry: %v", err)
	}
	deps := extapi.MCPRegistryDependencies{Registry: store, Dialer: dialer, HTTPDialer: httpDialer}
	if rec != nil {
		deps.Publish = rec.publish
	}
	return NewService(deps), store
}

// TestTheRegistryRefusesWithoutAStore: a service with no registry port is a
// daemon with no MCP servers — the writes must SAY so rather than call into
// nothing.
func TestTheRegistryRefusesWithoutAStore(t *testing.T) {
	svc := NewService(extapi.MCPRegistryDependencies{})
	if _, err := svc.Add("corr", mcpapi.Server{Name: "owned", Command: "never-executed"}); !errors.Is(err, mcpapi.ErrNoRegistry) {
		t.Fatalf("add without a registry must refuse, got %v", err)
	}
	if _, err := svc.SetEnabled("corr", "owned", true); !errors.Is(err, mcpapi.ErrNoRegistry) {
		t.Fatalf("set-enabled without a registry must refuse, got %v", err)
	}
	if _, _, err := svc.Attach(context.Background(), "corr", "owned"); !errors.Is(err, mcpapi.ErrNoRegistry) {
		t.Fatalf("attach without a registry must refuse, got %v", err)
	}
	if _, err := svc.Remove("corr", "owned"); !errors.Is(err, mcpapi.ErrNoRegistry) {
		t.Fatalf("remove without a registry must refuse, got %v", err)
	}
	if rows := svc.registrations(); rows != nil {
		t.Fatalf("a service with no registry must list no servers, got %v", rows)
	}
}

// TestAddRegistersTheServerAndJournalsIt: the registration lands in the
// registry and the lifecycle transition is published — the subject is the
// server's own and the kind is the registry's.
func TestAddRegistersTheServerAndJournalsIt(t *testing.T) {
	rec := &recorder{}
	svc, store := newService(t, rec, nil, nil)

	saved, err := svc.Add("corr-1", mcpapi.Server{Name: "owned", Command: "never-executed"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if saved.ID == "" || saved.Name != "owned" {
		t.Fatalf("the registered server is not the one that was added: %+v", saved)
	}
	if rows := store.List(); len(rows) != 1 || rows[0].ID != saved.ID {
		t.Fatalf("the registry does not hold the added server: %v", rows)
	}
	if got := rec.kinds(); len(got) != 1 || got[0] != string(event.KindMCPAdded) {
		t.Fatalf("the add must be journaled as mcp.added, got %v", got)
	}
	spec := rec.specs[0]
	if spec.Subject != "mcp.owned" || spec.Actor != "mcp" || spec.CorrelationID != "corr-1" {
		t.Fatalf("the journaled event is not the registry's own: %+v", spec)
	}
	payload, _ := spec.Payload.(map[string]any)
	if payload["name"] != "owned" || payload["transport"] != "stdio" {
		t.Fatalf("the journaled payload does not name the server: %+v", payload)
	}
}

// TestARemoteRegistrationIsJournalledAsHTTP: the transport in the payload
// follows the row's transport.
func TestARemoteRegistrationIsJournalledAsHTTP(t *testing.T) {
	rec := &recorder{}
	svc, _ := newService(t, rec, nil, nil)
	if _, err := svc.Add("corr", mcpapi.Server{Name: "remote", URL: "https://mcp.example.com/v1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if rec.specs[0].Payload.(map[string]any)["transport"] != "http" {
		t.Fatalf("a remote server must be journalled as http, got %+v", rec.specs[0].Payload)
	}
}

// TestSetEnabledFlipsTheFlagAndJournalsIt: the flag is the registry's row and
// the transition is journaled.
func TestSetEnabledFlipsTheFlagAndJournalsIt(t *testing.T) {
	rec := &recorder{}
	svc, store := newService(t, rec, nil, nil)
	saved, err := svc.Add("corr", mcpapi.Server{Name: "owned", Command: "never-executed"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	flipped, err := svc.SetEnabled("corr-2", saved.ID, false)
	if err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if flipped.Enabled {
		t.Fatal("the server is still enabled")
	}
	if row, ok := store.Get(saved.ID); !ok || row.Enabled {
		t.Fatalf("the registry does not hold the flipped flag: %+v", row)
	}
	if got := rec.kinds(); len(got) != 2 || got[1] != string(event.KindMCPUpdated) {
		t.Fatalf("the flip must be journaled as mcp.updated, got %v", got)
	}
	if rec.specs[1].Payload.(map[string]any)["enabled"] != false {
		t.Fatalf("the journaled payload does not carry the flag: %+v", rec.specs[1].Payload)
	}
}

// TestAttachDialsDiscoversAndJournals: the attach spawns the server through
// the host's seam, discovers its tools and journals them — the agent is
// offered them as mcp_<server>_<tool> from the next run on.
func TestAttachDialsDiscoversAndJournals(t *testing.T) {
	rec := &recorder{}
	conn := &fakeConn{tools: []mcpapi.ToolDef{{Name: "greet"}}}
	dials := 0
	dialer := func(context.Context, string, []string, map[string]string) (mcpapi.Conn, error) {
		dials++
		return conn, nil
	}
	svc, _ := newService(t, rec, dialer, nil)
	if _, err := svc.Add("corr", mcpapi.Server{Name: "fake", Command: "never-executed"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	srv, names, err := svc.Attach(context.Background(), "corr-2", "fake")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if dials != 1 {
		t.Fatalf("the attach must reach the host's dialer once, got %d", dials)
	}
	if srv.Name != "fake" {
		t.Fatalf("the attach answered about another server: %+v", srv)
	}
	if len(names) != 1 || names[0] != "mcp_fake_greet" {
		t.Fatalf("the discovered tools are not the callable names: %v", names)
	}
	if got := rec.kinds(); len(got) != 2 || got[1] != string(event.KindMCPAttached) {
		t.Fatalf("the attach must be journaled as mcp.attached, got %v", got)
	}
	tools, _ := rec.specs[1].Payload.(map[string]any)["tools"].([]string)
	if len(tools) != 1 || tools[0] != "mcp_fake_greet" {
		t.Fatalf("the journaled attach does not carry the tools: %+v", rec.specs[1].Payload)
	}
}

// TestAttachRefusesAnUnknownServer: the registry does not know it, so the
// attach must not dial at all.
func TestAttachRefusesAnUnknownServer(t *testing.T) {
	dials := 0
	dialer := func(context.Context, string, []string, map[string]string) (mcpapi.Conn, error) {
		dials++
		return &fakeConn{}, nil
	}
	svc, _ := newService(t, nil, dialer, nil)
	if _, _, err := svc.Attach(context.Background(), "corr", "ghost"); !errors.Is(err, mcpapi.ErrNotFound) {
		t.Fatalf("attaching an unknown server must refuse, got %v", err)
	}
	if dials != 0 {
		t.Fatalf("the attach must not dial for an unknown server, got %d dials", dials)
	}
}

// TestAttachRefusesADoubleAttach: attaching a live server again is an error —
// detach first. And it must not even DIAL: the dial spawns a child process,
// and a refused attach must not leave one behind.
func TestAttachRefusesADoubleAttach(t *testing.T) {
	dials := 0
	dialer := func(context.Context, string, []string, map[string]string) (mcpapi.Conn, error) {
		dials++
		return &fakeConn{}, nil
	}
	svc, _ := newService(t, nil, dialer, nil)
	if _, err := svc.Add("corr", mcpapi.Server{Name: "fake", Command: "never-executed"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, _, err := svc.Attach(context.Background(), "corr", "fake"); err != nil {
		t.Fatalf("the first attach must succeed: %v", err)
	}
	_, _, err := svc.Attach(context.Background(), "corr", "fake")
	if err == nil || !strings.Contains(err.Error(), "already attached") {
		t.Fatalf("the second attach must refuse, got %v", err)
	}
	if dials != 1 {
		t.Fatalf("the refused attach must not dial again, got %d dials", dials)
	}
}

// TestTheRemoteRegistrationDialsOverHTTP: a row with a URL speaks Streamable
// HTTP, everything else a spawned stdio process.
func TestTheRemoteRegistrationDialsOverHTTP(t *testing.T) {
	stdioDials, httpDials := 0, 0
	dialer := func(context.Context, string, []string, map[string]string) (mcpapi.Conn, error) {
		stdioDials++
		return &fakeConn{}, nil
	}
	httpDialer := func(context.Context, string, map[string]string) (mcpapi.Conn, error) {
		httpDials++
		return &fakeConn{}, nil
	}
	svc, _ := newService(t, nil, dialer, httpDialer)
	if _, err := svc.Add("corr", mcpapi.Server{Name: "local", Command: "never-executed"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := svc.Add("corr", mcpapi.Server{Name: "remote", URL: "https://mcp.example.com/v1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, _, err := svc.Attach(context.Background(), "corr", "local"); err != nil {
		t.Fatalf("the stdio attach: %v", err)
	}
	if _, _, err := svc.Attach(context.Background(), "corr", "remote"); err != nil {
		t.Fatalf("the http attach: %v", err)
	}
	if stdioDials != 1 || httpDials != 1 {
		t.Fatalf("the transports were mixed up: %d stdio, %d http", stdioDials, httpDials)
	}
}

// TestTheAttachRefusesWithoutADialer: the dialers are the host's seams, and a
// missing one must refuse with a readable error instead of calling a nil
// func value.
func TestTheAttachRefusesWithoutADialer(t *testing.T) {
	svc, _ := newService(t, nil, nil, nil)
	if _, err := svc.Add("corr", mcpapi.Server{Name: "local", Command: "never-executed"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := svc.Add("corr", mcpapi.Server{Name: "remote", URL: "https://mcp.example.com/v1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, _, err := svc.Attach(context.Background(), "corr", "local"); err == nil ||
		!strings.Contains(err.Error(), "no stdio MCP dialer is configured") {
		t.Fatalf("the stdio attach must refuse with the dialer's absence, got %v", err)
	}
	if _, _, err := svc.Attach(context.Background(), "corr", "remote"); err == nil ||
		!strings.Contains(err.Error(), "no HTTP MCP dialer is configured") {
		t.Fatalf("the http attach must refuse with the dialer's absence, got %v", err)
	}
}

// TestDetachIsTheKillSwitch: the connection is closed and the transition is
// journaled; a second detach has nothing to close.
func TestDetachIsTheKillSwitch(t *testing.T) {
	rec := &recorder{}
	conn := &fakeConn{}
	dialer := func(context.Context, string, []string, map[string]string) (mcpapi.Conn, error) {
		return conn, nil
	}
	svc, _ := newService(t, rec, dialer, nil)
	if _, err := svc.Add("corr", mcpapi.Server{Name: "fake", Command: "never-executed"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, _, err := svc.Attach(context.Background(), "corr", "fake"); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if err := svc.Detach("corr-2", "fake"); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	if !conn.closed {
		t.Fatal("the detach did not close the connection")
	}
	if got := rec.kinds(); len(got) != 3 || got[2] != string(event.KindMCPDetached) {
		t.Fatalf("the detach must be journaled as mcp.detached, got %v", got)
	}
	if err := svc.Detach("corr-3", "fake"); err == nil || !strings.Contains(err.Error(), "is not attached") {
		t.Fatalf("the second detach must refuse, got %v", err)
	}
}

// TestRemoveDetachesALiveServerFirst: the row and the attachment are one
// thing to the operator — removing the row must take the live server with it.
func TestRemoveDetachesALiveServerFirst(t *testing.T) {
	rec := &recorder{}
	conn := &fakeConn{}
	dialer := func(context.Context, string, []string, map[string]string) (mcpapi.Conn, error) {
		return conn, nil
	}
	svc, store := newService(t, rec, dialer, nil)
	if _, err := svc.Add("corr", mcpapi.Server{Name: "fake", Command: "never-executed"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, _, err := svc.Attach(context.Background(), "corr", "fake"); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	removed, err := svc.Remove("corr-2", "fake")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !removed {
		t.Fatal("the remove must report that the row was there")
	}
	if !conn.closed {
		t.Fatal("the remove did not detach the live server")
	}
	if rows := store.List(); len(rows) != 0 {
		t.Fatalf("the row is still in the registry: %v", rows)
	}
	if got := rec.kinds(); len(got) != 4 || got[3] != string(event.KindMCPRemoved) {
		t.Fatalf("the remove must be journaled as mcp.removed, got %v", got)
	}
}

// TestAttachEnabledSkipsTheDisabledAndReportsFailures: the boot path must
// not take the daemon down for one broken server.
func TestAttachEnabledSkipsTheDisabledAndReportsFailures(t *testing.T) {
	dialer := func(_ context.Context, command string, _ []string, _ map[string]string) (mcpapi.Conn, error) {
		if command == "broken" {
			return nil, errors.New("the server refused to start")
		}
		return &fakeConn{}, nil
	}
	svc, _ := newService(t, nil, dialer, nil)
	if _, err := svc.Add("corr", mcpapi.Server{Name: "good", Command: "good"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := svc.Add("corr", mcpapi.Server{Name: "broken", Command: "broken"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := svc.Add("corr", mcpapi.Server{Name: "off", Command: "good"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := svc.SetEnabled("corr", "off", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}

	attached, failures := svc.AttachEnabled(context.Background())
	if len(attached) != 1 || attached[0] != "good" {
		t.Fatalf("the boot path attached the wrong servers: %v", attached)
	}
	if len(failures) != 1 || failures["broken"] == nil {
		t.Fatalf("the broken server was not reported: %v", failures)
	}
}

// TestAttachedCountsTheTools: the status surface answers the live servers and
// their tool counts.
func TestAttachedCountsTheTools(t *testing.T) {
	conn := &fakeConn{tools: []mcpapi.ToolDef{{Name: "a"}, {Name: "b"}}}
	dialer := func(context.Context, string, []string, map[string]string) (mcpapi.Conn, error) {
		return conn, nil
	}
	svc, _ := newService(t, nil, dialer, nil)
	if _, err := svc.Add("corr", mcpapi.Server{Name: "fake", Command: "never-executed"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got := svc.Attached(); len(got) != 0 {
		t.Fatalf("nothing is attached yet, got %v", got)
	}
	if _, _, err := svc.Attach(context.Background(), "corr", "fake"); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	got := svc.Attached()
	if len(got) != 1 || got["fake"] != 2 {
		t.Fatalf("the attached surface does not count the tools: %v", got)
	}
}

// TestCloseDetachesEverything: the host's shutdown path kills the children.
func TestCloseDetachesEverything(t *testing.T) {
	conn := &fakeConn{}
	dialer := func(context.Context, string, []string, map[string]string) (mcpapi.Conn, error) {
		return conn, nil
	}
	svc, _ := newService(t, nil, dialer, nil)
	if _, err := svc.Add("corr", mcpapi.Server{Name: "fake", Command: "never-executed"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, _, err := svc.Attach(context.Background(), "corr", "fake"); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	svc.Close()
	if !conn.closed {
		t.Fatal("the close did not detach the live server")
	}
	if got := svc.Attached(); len(got) != 0 {
		t.Fatalf("the close left an attachment behind: %v", got)
	}
}
