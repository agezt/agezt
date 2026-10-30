// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"time"

	"github.com/agezt/agezt/kernel/app/binding"
	appeventbus "github.com/agezt/agezt/kernel/app/eventbus"
	"github.com/agezt/agezt/kernel/app/opaudit"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
)

// appHost binds the routed kernel and one correlation before audit admission.
// Migrated handlers can use the kernel's actor/correlation context for domain events.
type appHostKey struct{}

// appConnKey carries the transport connection into an operation that has to
// observe the client's disconnect — the run cancels itself when the operator
// goes away, instead of spending into a closed connection.
type appConnKey struct{}

// The transport request's id lives in the context under the app layer's key:
// an operation that runs a governed tool uses it as the tool call's id, as
// the native handlers did, and an operation that names a run after the call
// reads it the same way.

type appHost struct {
	// scope is the SCOPE of the request: the families' ports, filled for the
	// daemon the request is routed to. The host holds no handle — it can
	// reach each family's capabilities and nothing behind them.
	scope       binding.Scope
	correlation string
}

type appAuditor struct{}

func (appAuditor) Begin(ctx context.Context, record opapi.AuditRecord) (opapi.AuditSpan, error) {
	host, ok := ctx.Value(appHostKey{}).(appHost)
	if !ok || host.correlation == "" || !requestBusPorts(ctx).Ready() {
		return nil, errors.New("operation audit host unavailable")
	}
	var args map[string]any
	if err := json.Unmarshal(record.Input, &args); err != nil {
		return nil, err
	}
	span := &appAuditSpan{host: host, ports: requestBusPorts(ctx), operation: record.Operation, start: time.Now()}
	payload := map[string]any{"op": record.Operation, "caller": string(record.Principal.Kind)}
	if record.Principal.Tenant != "" {
		payload["tenant"] = record.Principal.Tenant
	}
	if redacted := opaudit.SummarizeArgs(args); redacted != nil {
		payload["args"] = redacted
	}
	if _, err := requestBusPorts(ctx).Publish(event.Spec{
		Subject: "op." + record.Operation, Kind: event.KindOpInvoked, Actor: "controlplane",
		CorrelationID: host.correlation, Payload: payload,
	}); err != nil {
		return nil, err
	}
	return span, nil
}

type appAuditSpan struct {
	host      appHost
	ports     appeventbus.Ports
	operation string
	start     time.Time
}

func (s *appAuditSpan) End(_ context.Context, cause error) error {
	kind := event.KindOpCompleted
	payload := map[string]any{"op": s.operation, "duration_ms": time.Since(s.start).Milliseconds()}
	if cause != nil {
		kind = event.KindOpFailed
		payload["error"] = opaudit.TruncateValue(cause.Error())
	}
	_, err := s.ports.Publish(event.Spec{
		Subject: "op." + s.operation, Kind: kind, Actor: "controlplane",
		CorrelationID: s.host.correlation, Payload: payload,
	})
	return err
}

// appEmitter preserves the native event envelope and returns write failures to
// app.Dispatch instead of dropping them. App validates its owned frame schema first.
type appEmitter struct {
	conn      net.Conn
	requestID string
}

func (e appEmitter) Emit(ctx context.Context, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var frame *event.Event
	switch value := value.(type) {
	case *event.Event:
		frame = value
	case event.Event:
		frame = &value
	default:
		return errors.New("control-plane streams require kernel event frames")
	}
	if frame == nil {
		return errors.New("control-plane stream event is nil")
	}
	return writeResp(e.conn, Response{ID: e.requestID, Type: RespEvent, Event: frame})
}
