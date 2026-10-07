// SPDX-License-Identifier: MIT
package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/toolbox"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
)

func toolboxUnknownBatch(t *testing.T) []any {
	t.Helper()
	name := "__agezt_owned_not_in_catalog__"
	for _, entry := range toolbox.Catalog {
		if entry.Name == name {
			t.Fatal("fixture would reach real manager")
		}
	}
	return []any{name, name, name}
}
func TestToolboxInstallNativeAuditFailureBlocksRequestedAndAttempts(t *testing.T) {
	k, s, _, p := pulseAppFixture(t)
	if err := k.Journal().Close(); err != nil {
		t.Fatal(err)
	}
	frames := callAppHost(t, s, Request{ID: "owned", Cmd: CmdToolboxInstall, Token: "primary", Args: map[string]any{"names": toolboxUnknownBatch(t)}})
	if len(frames) != 1 || frames[0].Type != RespError || !strings.Contains(frames[0].Error, "journal") || p.CallCount() != 0 {
		t.Fatalf("EXPECTED:audit failure emits only error ACTUAL:frames=%d terminal=%s", len(frames), frames[len(frames)-1].Type)
	}
}
func TestToolboxInstallNativeCanceledAdmissionHasNoLifecycleOrAudit(t *testing.T) {
	k, s, _, _ := pulseAppFixture(t)
	head, hash := k.Journal().Head()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := Request{ID: "owned", Cmd: CmdToolboxInstall, Token: "primary", Args: map[string]any{"names": toolboxUnknownBatch(t)}}
	conn := newToolboxRecordingConn(req)
	s.handleConn(ctx, conn)
	after, afterHash := k.Journal().Head()
	var reply Response
	if err := json.Unmarshal(bytes.TrimSpace(conn.output.Bytes()), &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Type != RespError || !strings.Contains(reply.Error, "context canceled") || head != after || hash != afterHash {
		t.Fatalf("EXPECTED:canceled admission without publication ACTUAL:type=%s seq=%d/%d", reply.Type, head, after)
	}
}

type toolboxFixtureAddr struct{}

func (toolboxFixtureAddr) Network() string { return "fixture" }
func (toolboxFixtureAddr) String() string  { return "fixture" }

type toolboxRecordingConn struct {
	input      *bytes.Reader
	output     bytes.Buffer
	failure    error
	writes     int
	afterWrite func()
}

func newToolboxRecordingConn(req Request) *toolboxRecordingConn {
	raw, _ := json.Marshal(req)
	return &toolboxRecordingConn{input: bytes.NewReader(append(raw, 10))}
}
func (c *toolboxRecordingConn) Read(p []byte) (int, error) { return c.input.Read(p) }
func (c *toolboxRecordingConn) Write(p []byte) (int, error) {
	c.writes++
	if c.failure != nil {
		return 0, c.failure
	}
	n, err := c.output.Write(p)
	if c.afterWrite != nil {
		c.afterWrite()
	}
	return n, err
}
func (*toolboxRecordingConn) Close() error                     { return nil }
func (*toolboxRecordingConn) LocalAddr() net.Addr              { return toolboxFixtureAddr{} }
func (*toolboxRecordingConn) RemoteAddr() net.Addr             { return toolboxFixtureAddr{} }
func (*toolboxRecordingConn) SetDeadline(time.Time) error      { return nil }
func (*toolboxRecordingConn) SetReadDeadline(time.Time) error  { return nil }
func (*toolboxRecordingConn) SetWriteDeadline(time.Time) error { return nil }
func TestToolboxInstallNativeBrokenStreamStopsLaterAttempts(t *testing.T) {
	k, s, _, p := pulseAppFixture(t)
	head, _ := k.Journal().Head()
	conn := newToolboxRecordingConn(Request{ID: "owned", Cmd: CmdToolboxInstall, Token: "primary", Args: map[string]any{"names": toolboxUnknownBatch(t)}})
	conn.failure = errors.New("owned stream write failure")
	s.handleConn(context.Background(), conn)
	rows, err := k.Journal().Tail(100)
	if err != nil {
		t.Fatal(err)
	}
	attempts, requested, invoked, failed := 0, 0, 0, 0
	for _, e := range rows {
		if e.Seq <= head {
			continue
		}
		switch e.Kind {
		case event.KindToolboxInstalled:
			attempts++
		case event.KindToolboxInstallRequested:
			requested++
		case event.KindOpInvoked:
			invoked++
		case event.KindOpFailed:
			failed++
		}
	}
	if attempts != 1 || requested != 1 || invoked != 1 || failed != 1 || p.CallCount() != 0 {
		t.Fatalf("EXPECTED:one attempted tool then failed audit ACTUAL:attempts=%d requested=%d invoked=%d failed=%d writes=%d", attempts, requested, invoked, failed, conn.writes)
	}
}
func TestToolboxInstallNativeDomainEventsShareOperationIdentity(t *testing.T) {
	k, s, _, _ := pulseAppFixture(t)
	head, _ := k.Journal().Head()
	frames := callAppHost(t, s, Request{ID: "owned", Cmd: CmdToolboxInstall, Token: "primary", Args: map[string]any{"names": toolboxUnknownBatch(t), "correlation_id": "poison", "tenant": "spoof"}})
	if len(frames) != 4 || frames[3].Type != RespResult {
		t.Fatal(frames)
	}
	rows, err := k.Journal().Tail(100)
	if err != nil {
		t.Fatal(err)
	}
	corr := ""
	domain, terminal := 0, 0
	for _, e := range rows {
		if e.Seq <= head {
			continue
		}
		if e.Kind == event.KindOpInvoked {
			corr = e.CorrelationID
		}
		if e.Kind == event.KindToolboxInstallRequested || e.Kind == event.KindToolboxInstalled {
			domain++
			if corr == "" || corr == "poison" || e.CorrelationID != corr {
				t.Fatalf("EXPECTED:domain owns audit identity ACTUAL:audit=%q domain=%q", corr, e.CorrelationID)
			}
		}
		if e.Kind == event.KindOpCompleted {
			terminal++
			if e.CorrelationID != corr {
				t.Fatal(e)
			}
		}
	}
	if domain != 4 || terminal != 1 {
		t.Fatal(domain, terminal)
	}
}

func TestToolboxNativeThreeOperationsPolicyStreamAndTypes(t *testing.T) {
	want := map[string]bool{CmdToolboxDetect: true, CmdToolboxOutdated: true, CmdToolboxInstall: false}
	seen := map[string]bool{}
	for _, op := range registeredAppOperations() {
		spec := op.Spec()
		read, known := want[spec.Name]
		if !known {
			continue
		}
		wire, ok := commandRegistry[spec.Name]
		stream := opapi.StreamNone
		if !read {
			stream = opapi.StreamEvents
		}
		if seen[spec.Name] || !ok || !wire.AppOwned || wire.ReadOnly != read || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamMode(stream) || spec.ReadOnly != read || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != stream || !spec.AllowUnknownInput {
			t.Fatal(spec, wire)
		}
		if !read && (spec.Input != reflect.TypeFor[apptools.ToolboxInstallRequest]() || spec.Output != reflect.TypeFor[apptools.ToolboxInstallOutput]() || spec.Emission != reflect.TypeFor[event.Event]()) {
			t.Fatal(spec)
		}
		seen[spec.Name] = true
	}
	if len(seen) != 3 || len(toolboxInstallOperations) != 1 || len(toolboxReadOperations) != 2 {
		t.Fatal(seen)
	}
}

func TestToolboxInstallNativePostAdmissionPublicationErrorStopsProgress(t *testing.T) {
	k, s, _, _ := pulseAppFixture(t)
	conn := newToolboxRecordingConn(Request{ID: "owned", Cmd: CmdToolboxInstall, Token: "primary", Args: map[string]any{"names": toolboxUnknownBatch(t)}})
	conn.afterWrite = func() {
		if conn.writes == 1 {
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	s.handleConn(context.Background(), conn)
	lines := bytes.Split(bytes.TrimSpace(conn.output.Bytes()), []byte{'\n'})
	if len(lines) != 2 {
		t.Fatalf("EXPECTED:one progress then publication failure ACTUAL:frames=%d", len(lines))
	}
	var first, last Response
	_ = json.Unmarshal(lines[0], &first)
	_ = json.Unmarshal(lines[1], &last)
	if first.Type != RespEvent || last.Type != RespError || !strings.Contains(last.Error, "journal") {
		t.Fatal(first, last)
	}
}
