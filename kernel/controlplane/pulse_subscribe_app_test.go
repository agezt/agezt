// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"io"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPulseFourteenNativeOperationsAndStreamProjectionMetadata(t *testing.T) {
	want := map[string]bool{CmdPulseStatus: true, CmdPulseAsks: true, CmdPulseSubscribe: true, CmdPulseAskResolve: false, CmdPulsePause: false, CmdPulseResume: false, CmdPulseBeat: false, CmdPulseCadence: false, CmdPulseDial: false, CmdPulseQuiet: false, CmdPulseFlush: false, CmdPulseWatch: false, CmdPulseProbe: false, CmdPulseUnwatch: false}
	seen := map[string]bool{}
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if !strings.HasPrefix(spec.Name, "pulse_") {
			continue
		}
		read, known := want[spec.Name]
		wire, exists := commandRegistry[spec.Name]
		if !known || seen[spec.Name] || !exists || !wire.AppOwned || wire.ReadOnly != read || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || spec.ReadOnly != read || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary {
			t.Fatal(spec, wire)
		}
		if spec.Name == CmdPulseSubscribe {
			if spec.Stream != opapi.StreamLive || spec.Emission != reflect.TypeFor[*event.Event]() || len(spec.EmissionSchema) == 0 || reflect.ValueOf(wire.Handler).Pointer() != reflect.ValueOf(handlePulseAppStream).Pointer() {
				t.Fatal(spec, wire)
			}
		} else if spec.Stream != opapi.StreamNone {
			t.Fatal(spec)
		}
		seen[spec.Name] = true
	}
	if len(seen) != 14 || len(pulseSubscribeOperations) != 1 {
		t.Fatal(seen, len(pulseSubscribeOperations))
	}
	for _, mode := range []string{"unary", "write", "tenant"} {
		spec := opapi.Spec{Name: CmdPulseSubscribe, ReadOnly: true, Stream: opapi.StreamLive, EmissionSchema: json.RawMessage(`{"type":"object"}`)}
		switch mode {
		case "unary":
			spec.Stream = opapi.StreamNone
		case "write":
			spec.ReadOnly = false
		case "tenant":
			spec.Authz = opapi.OwnTenant
			spec.Tenancy = opapi.CallerTenant
		}
		operation, err := app.NewStreamingOperation(spec, func(context.Context, struct{}, func(*event.Event) error) (struct{}, error) { return struct{}{}, nil })
		if err != nil {
			t.Fatal(err)
		}
		if _, err := appCommandSpec(operation); err == nil {
			t.Fatal("incompatible pulse native contract accepted", mode)
		}
	}
}
func TestPulseSubscribeNativeBoundedWireHasEventsThenEOFAndNoAudit(t *testing.T) {
	k, s, _, provider := pulseAppFixture(t)
	ev, err := k.Bus().Publish(event.Spec{Subject: "owned.one", Kind: event.KindWorkflowStarted, Actor: "fixture", Payload: map[string]any{"raw": []any{false, float64(0), nil}}})
	if err != nil {
		t.Fatal(err)
	}
	client, conn := net.Pipe()
	defer client.Close()
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(context.Background(), conn) }()
	client.SetDeadline(time.Now().Add(3 * time.Second))
	req := Request{ID: "bounded", Cmd: CmdPulseSubscribe, Token: "primary", Args: map[string]any{"pattern": "owned.*", "since": float64(0), "until": float64(ev.Seq + 1)}}
	raw, _ := json.Marshal(req)
	if _, err := client.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(client)
	var frames []Response
	for scanner.Scan() {
		var frame Response
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, frame)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	<-done
	if len(frames) != 1 || frames[0].ID != "bounded" || frames[0].Type != RespEvent || !reflect.DeepEqual(frames[0].Event, ev) || provider.CallCount() != 0 {
		t.Fatal(frames, ev, provider.CallCount())
	}
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Subject == "op.pulse_subscribe" {
			t.Fatal("read subscription audited", e)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

type pulseWatchTimeout struct{}

func (pulseWatchTimeout) Error() string   { return "idle" }
func (pulseWatchTimeout) Timeout() bool   { return true }
func (pulseWatchTimeout) Temporary() bool { return true }

type pulseWatchConn struct {
	net.Conn
	reads     int
	deadlines []time.Time
	cancel    context.CancelFunc
	stop      bool
}

func (c *pulseWatchConn) SetReadDeadline(deadline time.Time) error {
	c.deadlines = append(c.deadlines, deadline)
	return nil
}
func (c *pulseWatchConn) Read([]byte) (int, error) {
	c.reads++
	switch c.reads {
	case 1:
		if c.stop {
			c.cancel()
		}
		return 0, pulseWatchTimeout{}
	case 2:
		return 1, nil
	default:
		return 0, io.EOF
	}
}
func TestPulseSubscribeNativeSingleWatcherTreatsTimeoutAndStrayByteAsIdle(t *testing.T) {
	for _, stop := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		conn := &pulseWatchConn{cancel: cancel, stop: stop}
		gone := pulseClientGone(ctx, conn)
		select {
		case <-gone:
		case <-time.After(time.Second):
			t.Fatal("watcher did not end")
		}
		cancel()
		want := 3
		if stop {
			want = 1
		}
		if conn.reads != want || len(conn.deadlines) != want {
			t.Fatal(conn.reads, conn.deadlines)
		}
		for _, deadline := range conn.deadlines {
			remaining := time.Until(deadline)
			if remaining <= 0 || remaining > 500*time.Millisecond {
				t.Fatal(deadline, remaining)
			}
		}
	}
}
func TestPulseSubscribeNativeUnlimitedEmitterRetainsSocketOwnedCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client, conn := net.Pipe()
	defer client.Close()
	defer conn.Close()
	client.SetDeadline(time.Now().Add(time.Second))
	done := make(chan error, 1)
	ev := &event.Event{Subject: "owned", Kind: event.KindWorkflowStarted}
	go func() { done <- pulseNativeEmitter{appEmitter{conn, "owned"}}.Emit(ctx, ev) }()
	line, err := bufio.NewReader(client).ReadBytes(10)
	if err != nil {
		t.Fatal(err)
	}
	var frame Response
	if err := json.Unmarshal(line, &frame); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil || frame.Type != RespEvent || frame.ID != "owned" || !reflect.DeepEqual(frame.Event, ev) {
		t.Fatal(err, frame)
	}
}

func TestPulseSubscribeNativeCanceledAdmissionDoesNotReplay(t *testing.T) {
	k, s, _, provider := pulseAppFixture(t)
	_, err := k.Bus().Publish(event.Spec{Subject: "owned.one", Kind: event.KindWorkflowStarted, Actor: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client, conn := net.Pipe()
	defer client.Close()
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(ctx, conn) }()
	client.SetDeadline(time.Now().Add(time.Second))
	req := Request{ID: "canceled", Cmd: CmdPulseSubscribe, Token: "primary", Args: map[string]any{"since": float64(0), "until": float64(1)}}
	raw, _ := json.Marshal(req)
	if _, err := client.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(client)
	var frames []Response
	for scanner.Scan() {
		var frame Response
		_ = json.Unmarshal(scanner.Bytes(), &frame)
		frames = append(frames, frame)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	<-done
	if len(frames) != 1 || frames[0].Type != RespError || frames[0].Error != "context canceled" || provider.CallCount() != 0 {
		t.Fatal(frames, provider.CallCount())
	}
}
