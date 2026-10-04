// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"time"
	"unicode/utf8"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)

// appHost binds the routed kernel and one correlation before audit admission.
// Migrated handlers can use the kernel's actor/correlation context for domain events.
type appHostKey struct{}
type appHost struct {
	kernel      *runtime.Kernel
	correlation string
}

type appAuditor struct{}

func (appAuditor) Begin(ctx context.Context, record opapi.AuditRecord) (opapi.AuditSpan, error) {
	host, ok := ctx.Value(appHostKey{}).(appHost)
	if !ok || host.kernel == nil || host.kernel.Bus() == nil || host.correlation == "" {
		return nil, errors.New("operation audit host unavailable")
	}
	var args map[string]any
	if err := json.Unmarshal(record.Input, &args); err != nil {
		return nil, err
	}
	span := &appAuditSpan{host: host, operation: record.Operation, start: time.Now()}
	payload := map[string]any{"op": record.Operation, "caller": string(record.Principal.Kind)}
	if record.Principal.Tenant != "" {
		payload["tenant"] = record.Principal.Tenant
	}
	if redacted := auditArgs(args); redacted != nil {
		payload["args"] = redacted
	}
	if _, err := host.kernel.Bus().Publish(event.Spec{
		Subject: "op." + record.Operation, Kind: event.KindOpInvoked, Actor: "controlplane",
		CorrelationID: host.correlation, Payload: payload,
	}); err != nil {
		return nil, err
	}
	return span, nil
}

type appAuditSpan struct {
	host      appHost
	operation string
	start     time.Time
}

func (s *appAuditSpan) End(_ context.Context, cause error) error {
	kind := event.KindOpCompleted
	payload := map[string]any{"op": s.operation, "duration_ms": time.Since(s.start).Milliseconds()}
	if cause != nil {
		kind = event.KindOpFailed
		message := cause.Error()
		if utf8.RuneCountInString(message) > auditValueMax {
			message = string([]rune(message)[:auditValueMax]) + "…"
		}
		payload["error"] = message
	}
	_, err := s.host.kernel.Bus().Publish(event.Spec{
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
