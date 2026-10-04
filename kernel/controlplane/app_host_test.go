// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestAppHostSocketMutationAndStreaming(t *testing.T) {
	for _, mode := range []string{"read-stream", "mutate", "mutate-stream", "panic", "invalid-input", "audit-unavailable", "terminal-audit-error"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { k.Close() })
			s := NewServer(k, dir)
			s.token = "primary"
			const name = "zz_test_app_host"
			type input struct {
				Reason   string `json:"reason,omitempty"`
				Password string `json:"password,omitempty"`
			}
			type output struct {
				OK bool `json:"ok"`
			}
			calls := 0
			var handlerCorrelation string
			spec := opapi.Spec{Name: name, ReadOnly: mode == "read-stream"}
			if strings.Contains(mode, "stream") {
				spec.Stream = opapi.StreamEvents
			}
			spec.EmissionSchema = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"}},"required":["kind"],"additionalProperties":true}`)
			op, err := app.NewStreamingOperation(spec, func(ctx context.Context, in input, emit func(*event.Event) error) (output, error) {
				calls++
				if !spec.ReadOnly {
					host := ctx.Value(appHostKey{}).(appHost)
					handlerCorrelation = host.correlation
					if host.kernel != k || k.ActorFromCtx(ctx) != "operator" {
						t.Error("handler lost routed actor/kernel context")
					}
					invocations := 0
					_ = k.Journal().Range(func(e *event.Event) error {
						if e.Subject == "op."+name && e.Kind == event.KindOpInvoked {
							invocations++
						}
						return nil
					})
					if invocations != 1 {
						t.Errorf("effect entered without exactly one durable admission: %d", invocations)
					}
				}
				if spec.Stream != opapi.StreamNone {
					if err := emit(&event.Event{Kind: event.KindMarketInstallProgress, Subject: "market.install", Actor: "market", Payload: json.RawMessage(`{"step":"ready"}`)}); err != nil {
						return output{}, err
					}
				}
				if mode == "panic" {
					panic("handler failed")
				}
				if mode == "terminal-audit-error" {
					_ = k.Journal().Close()
				}
				return output{OK: true}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			// Exercise the same registration/factory/adapter as production operations.
			original := systemOperations
			systemOperations = []app.Operation{op}
			for _, op := range systemOperations {
				wire, err := appCommandSpec(op)
				if err != nil {
					t.Fatal(err)
				}
				register(wire)
			}
			t.Cleanup(func() { systemOperations = original; delete(commandRegistry, name) })
			if mode == "audit-unavailable" {
				_ = k.Journal().Close()
			}
			secret := strings.Join([]string{"fixture", "private", "argument"}, "-")
			args := map[string]any{"reason": "test", "password": secret}
			if mode == "invalid-input" {
				args["reason"] = true
			}
			responses := callAppHost(t, s, Request{ID: "fixture", Cmd: name, Token: "primary", Args: args})
			last := responses[len(responses)-1]
			wantError := mode == "panic" || mode == "invalid-input" || mode == "audit-unavailable" || mode == "terminal-audit-error"
			if wantError != (last.Type == RespError) {
				t.Fatalf("terminal frame=%+v error expected=%v", last, wantError)
			}
			if !wantError && last.Result["ok"] != true {
				t.Fatalf("typed terminal output=%+v", last)
			}
			wantCalls := 1
			if mode == "invalid-input" || mode == "audit-unavailable" {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Fatalf("effects=%d want=%d", calls, wantCalls)
			}
			if spec.Stream != opapi.StreamNone {
				if len(responses) != 2 || responses[0].Type != RespEvent || responses[0].Event == nil || responses[0].Event.Kind != event.KindMarketInstallProgress || string(responses[0].Event.Payload) != `{"step":"ready"}` {
					t.Fatalf("stream wire=%+v", responses)
				}
			}
			if mode == "audit-unavailable" || mode == "terminal-audit-error" {
				return
			}
			var events []*event.Event
			if err := k.Journal().Range(func(e *event.Event) error {
				if e.Subject == "op."+name {
					events = append(events, e)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if spec.ReadOnly || mode == "invalid-input" {
				if len(events) != 0 {
					t.Fatalf("read/rejected input journaled: %d", len(events))
				}
				return
			}
			terminalKind := event.KindOpCompleted
			if mode == "panic" {
				terminalKind = event.KindOpFailed
			}
			if len(events) != 2 || events[0].Kind != event.KindOpInvoked || events[1].Kind != terminalKind || events[0].CorrelationID == "" || events[0].CorrelationID != events[1].CorrelationID || events[0].CorrelationID != handlerCorrelation {
				t.Fatalf("audit arc=%v", events)
			}
			for _, e := range events {
				if strings.Contains(string(e.Payload), secret) {
					t.Fatal("private input reached journal")
				}
			}
			var payload map[string]any
			if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload["caller"] != "operator" || payload["args"].(map[string]any)["reason"] != "test" {
				t.Fatalf("audit metadata=%v", payload)
			}
		})
	}
}

func TestAppHostTenantAuditUsesRoutedJournal(t *testing.T) {
	dir := t.TempDir()
	primary, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { primary.Close() })
	s := NewServer(primary, dir)
	s.token = "primary"
	registry, err := tenant.New(filepath.Join(dir, "tenants"), func(_ string, baseDir string) (io.Closer, error) {
		return runtime.Open(runtime.Config{BaseDir: baseDir, Provider: mock.New()})
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.CloseAll() })
	s.SetTenants(registry)
	entry, err := registry.Acquire("acme", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	token, err := registry.Token("acme")
	if err != nil {
		t.Fatal(err)
	}
	const name = "zz_test_app_tenant"
	type input struct {
		Tenant string `json:"tenant"`
	}
	type output struct {
		OK bool `json:"ok"`
	}
	calls := 0
	op, err := app.NewOperation(opapi.Spec{Name: name, Authz: opapi.OwnTenant, Tenancy: opapi.CallerTenant}, func(ctx context.Context, in input) (output, error) {
		host := ctx.Value(appHostKey{}).(appHost)
		if host.kernel != entry.Kernel || host.kernel == primary || in.Tenant != "acme" {
			t.Error("tenant operation bound to another host")
		}
		calls++
		return output{true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	original := systemOperations
	systemOperations = []app.Operation{op}
	for _, op := range systemOperations {
		wire, err := appCommandSpec(op)
		if err != nil {
			t.Fatal(err)
		}
		register(wire)
	}
	t.Cleanup(func() { systemOperations = original; delete(commandRegistry, name) })
	for _, credential := range []string{token, "primary"} {
		responses := callAppHost(t, s, Request{ID: "tenant", Cmd: name, Token: credential, Args: map[string]any{"tenant": "acme"}})
		if last := responses[len(responses)-1]; last.Type != RespResult || last.Result["ok"] != true {
			t.Fatalf("tenant operation failed: %+v", last)
		}
	}
	responses := callAppHost(t, s, Request{ID: "foreign", Cmd: name, Token: token, Args: map[string]any{"tenant": "other"}})
	if responses[0].Type != RespError || calls != 2 {
		t.Fatalf("foreign tenant admitted: %+v calls=%d", responses, calls)
	}
	for _, check := range []struct {
		k    *runtime.Kernel
		want int
	}{{primary, 0}, {entry.Kernel.(*runtime.Kernel), 4}} {
		count := 0
		if err := check.k.Journal().Range(func(e *event.Event) error {
			if e.Subject == "op."+name {
				count++
				var p map[string]any
				if err := json.Unmarshal(e.Payload, &p); err != nil {
					return err
				}
				if e.Kind == event.KindOpInvoked && p["tenant"] != "acme" {
					t.Errorf("audit tenant=%v", p)
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if count != check.want {
			t.Fatalf("routed journal has %d operation events want=%d", count, check.want)
		}
	}
}

func TestAppHostEmitterRetainsCancellationAndWriteCause(t *testing.T) {
	client, server := net.Pipe()
	_ = client.Close()
	defer server.Close()
	emitter := appEmitter{server, "fixture"}
	if err := emitter.Emit(context.Background(), &event.Event{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write cause lost: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := emitter.Emit(ctx, &event.Event{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation cause lost: %v", err)
	}
	for _, invalid := range []any{nil, (*event.Event)(nil), "wrong"} {
		if err := emitter.Emit(context.Background(), invalid); err == nil {
			t.Fatalf("invalid native frame accepted: %v", invalid)
		}
	}
}

func TestAppHostLiveDisconnectSettlesOwnedAudit(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	s := NewServer(k, dir)
	s.token = "primary"
	const name = "zz_test_app_live"
	started := make(chan struct{})
	op, err := app.NewStreamingOperation(opapi.Spec{Name: name, Stream: opapi.StreamLive, EmissionSchema: json.RawMessage(`{"type":"object"}`)}, func(ctx context.Context, _ struct{}, _ func(*event.Event) error) (map[string]any, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	original := systemOperations
	systemOperations = []app.Operation{op}
	for _, op := range systemOperations {
		wire, err := appCommandSpec(op)
		if err != nil {
			t.Fatal(err)
		}
		register(wire)
	}
	t.Cleanup(func() { systemOperations = original; delete(commandRegistry, name) })
	client, server := net.Pipe()
	defer client.Close()
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(context.Background(), server) }()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	raw, err := json.Marshal(Request{ID: "live", Cmd: name, Token: "primary"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write(append(raw, '\n')); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("live operation did not enter")
	}
	_ = client.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("disconnect did not cancel and settle operation")
	}
	var events []*event.Event
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Subject == "op."+name {
			events = append(events, e)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Kind != event.KindOpInvoked || events[1].Kind != event.KindOpFailed || events[0].CorrelationID != events[1].CorrelationID {
		t.Fatalf("disconnect audit=%v", events)
	}
	if !strings.Contains(string(events[1].Payload), context.Canceled.Error()) {
		t.Fatalf("disconnect cause lost: %s", events[1].Payload)
	}
}

func TestAppHostBindingRejectsUnsupportedWireShapes(t *testing.T) {
	primitive, err := app.NewOperation(opapi.Spec{Name: "primitive"}, func(context.Context, struct{}) (string, error) { return "ok", nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := appCommandSpec(primitive); err == nil {
		t.Fatal("primitive terminal bound to native object result")
	}
	progress, err := app.NewStreamingOperation(opapi.Spec{Name: "progress", Stream: opapi.StreamEvents}, func(context.Context, struct{}, func(string) error) (map[string]any, error) { return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := appCommandSpec(progress); err == nil {
		t.Fatal("non-event progress bound to native Event envelope")
	}
}

func callAppHost(t *testing.T, s *Server, request Request) []Response {
	t.Helper()
	client, server := net.Pipe()
	defer client.Close()
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(context.Background(), server) }()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write(append(raw, '\n')); err != nil {
		t.Fatal(err)
	}
	var responses []Response
	reader := bufio.NewReader(client)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		var response Response
		if err := json.Unmarshal(line, &response); err != nil {
			t.Fatal(err)
		}
		if response.ID != request.ID {
			t.Fatalf("response identity=%s want=%s", response.ID, request.ID)
		}
		responses = append(responses, response)
		if response.Type != RespEvent {
			break
		}
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("adapter did not settle")
	}
	return responses
}
