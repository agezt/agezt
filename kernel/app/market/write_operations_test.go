// SPDX-License-Identifier: MIT
package market

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	core "github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/platform/schema"
	"reflect"
	"strings"
	"testing"
)

type writeAudit struct {
	begins, ends       int
	err, endErr, cause error
	cancel             context.CancelFunc
}

func (a *writeAudit) Begin(context.Context, opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.begins++
	if a.err != nil {
		return nil, a.err
	}
	if a.cancel != nil {
		a.cancel()
	}
	return a, nil
}
func (a *writeAudit) End(_ context.Context, err error) error {
	a.ends++
	a.cause = err
	return a.endErr
}

type writeEmitter struct {
	frames []event.Event
	err    error
}

func (e *writeEmitter) Emit(_ context.Context, value any) error {
	frame, ok := value.(event.Event)
	if !ok {
		return errors.New("wrong progress type")
	}
	e.frames = append(e.frames, frame)
	return e.err
}
func writeRaw(name string) json.RawMessage {
	if name == "market_add_source" {
		return json.RawMessage(`{"name":" raw-name ","url":" raw-url ","pubkey":" raw-key ","unknown":true}`)
	}
	return json.RawMessage(`{"name":" owned ","marketplace":" raw-market ","version":" raw-version ","correlation_id":"poison","unknown":true}`)
}
func writeDispatcher(t *testing.T, p *writeProbe, a *writeAudit, factory *int, seen *context.Context, kind opapi.PrincipalKind) (*app.Dispatcher, []app.Operation) {
	t.Helper()
	ops, err := WriteOperations(func(ctx context.Context) *Writes { *factory++; *seen = ctx; return NewWrites(p, nil) })
	if err != nil {
		t.Fatal(err)
	}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: readAuth{kind}, Router: readRouter{}, Audit: a})
	if err != nil {
		t.Fatal(err)
	}
	return d, ops
}
func TestWriteOperationsCanonicalTypesPoliciesSchemasAuditAndProgress(t *testing.T) {
	if _, err := WriteOperations(nil); err == nil {
		t.Fatal("nil factory")
	}
	p := &writeProbe{record: core.InstalledPack{Name: "owned", InstalledMS: 9007199254740993}, source: core.Source{Name: "owned", AddedMS: 9007199254740993}, progress: []core.Event{{Stage: "owned", OK: false}}}
	a := &writeAudit{}
	f := 0
	var seen context.Context
	d, ops := writeDispatcher(t, p, a, &f, &seen, opapi.Operator)
	names := []string{"market_install", "market_uninstall", "market_add_source", "market_remove_source", "market_sync"}
	inputs := []reflect.Type{reflect.TypeFor[InstallRequest](), reflect.TypeFor[WriteNameRequest](), reflect.TypeFor[AddSourceRequest](), reflect.TypeFor[WriteNameRequest](), reflect.TypeFor[WriteNameRequest]()}
	outputs := []reflect.Type{reflect.TypeFor[InstallOutput](), reflect.TypeFor[UninstallOutput](), reflect.TypeFor[AddSourceOutput](), reflect.TypeFor[RemoveSourceOutput](), reflect.TypeFor[SyncOutput]()}
	ctx := opapi.WithCorrelation(context.Background(), "owned-operation")
	if len(ops) != 5 {
		t.Fatal(ops)
	}
	for i, op := range ops {
		sp := op.Spec()
		stream := opapi.StreamNone
		if i < 2 {
			stream = opapi.StreamEvents
		}
		if sp.Name != names[i] || sp.ReadOnly || sp.Authz != opapi.PrimaryOnly || sp.Tenancy != opapi.Primary || sp.Stream != stream || !sp.AllowUnknownInput || sp.Input != inputs[i] || sp.Output != outputs[i] {
			t.Fatal(sp)
		}
		emitter := &writeEmitter{}
		out, err := d.Dispatch(ctx, opapi.Caller{}, sp.Name, writeRaw(sp.Name), emitter)
		raw, _ := json.Marshal(out)
		if err != nil || schema.ValidateJSON(sp.OutputSchema, raw) != nil || f != i+1 || seen != ctx || a.begins != i+1 || a.ends != i+1 || a.cause != nil {
			t.Fatal(i, out, err, a, p)
		}
		if i < 2 {
			if sp.Emission != reflect.TypeFor[event.Event]() || len(emitter.frames) != 1 || p.ctx != ctx || p.corr != "owned-operation" || p.name != "owned" {
				t.Fatal(sp, emitter, p)
			}
			frame := emitter.frames[0]
			want := event.KindMarketInstallProgress
			subject := "market.install"
			if i == 1 {
				want = event.KindMarketUninstallProgress
				subject = "market.uninstall"
			}
			if frame.Kind != want || frame.Subject != subject || frame.Actor != "market" || frame.CorrelationID != "" {
				t.Fatal(frame)
			}
			encoded, _ := json.Marshal(frame)
			if schema.ValidateJSON(sp.EmissionSchema, encoded) != nil {
				t.Fatal(string(encoded))
			}
			var payload map[string]json.RawMessage
			json.Unmarshal(frame.Payload, &payload)
			if len(payload) != 2 || string(payload["ok"]) != "false" {
				t.Fatal(string(frame.Payload))
			}
		} else if sp.Emission != nil || len(sp.EmissionSchema) != 0 || len(emitter.frames) != 0 {
			t.Fatal(sp, emitter)
		}
	}
}
func TestWriteOperationsAdmissionBlocksEveryFactoryAndWriter(t *testing.T) {
	for _, mode := range []string{"audit", "tenant", "canceled", "after-audit"} {
		for _, name := range []string{"market_install", "market_uninstall", "market_add_source", "market_remove_source", "market_sync"} {
			p := &writeProbe{}
			a := &writeAudit{}
			f := 0
			var seen context.Context
			kind := opapi.Operator
			ctx, cancel := context.WithCancel(context.Background())
			owned := errors.New("owned audit admission failure")
			switch mode {
			case "audit":
				a.err = owned
			case "tenant":
				kind = opapi.Tenant
			case "canceled":
				cancel()
			case "after-audit":
				a.cancel = cancel
			}
			d, _ := writeDispatcher(t, p, a, &f, &seen, kind)
			_, err := d.Dispatch(ctx, opapi.Caller{}, name, writeRaw(name), &writeEmitter{})
			cancel()
			if err == nil || f != 0 || len(p.calls) != 0 {
				t.Fatal(mode, name, err, f, p)
			}
			if mode == "audit" && !errors.Is(err, owned) || mode == "canceled" && !errors.Is(err, context.Canceled) || mode == "after-audit" && (a.begins != 1 || a.ends != 1 || a.cause != context.Canceled) {
				t.Fatal(mode, err, a)
			}
		}
	}
}
func TestWriteOperationsSinkSchemaAndSettlementErrors(t *testing.T) {
	sink := errors.New("owned sink failure")
	settlement := errors.New("owned settlement failure")
	for _, name := range []string{"market_install", "market_uninstall"} {
		p := &writeProbe{progress: []core.Event{{Stage: "one"}, {Stage: "two"}}}
		a := &writeAudit{endErr: settlement}
		f := 0
		var seen context.Context
		d, ops := writeDispatcher(t, p, a, &f, &seen, opapi.Operator)
		e := &writeEmitter{err: sink}
		_, err := d.Dispatch(context.Background(), opapi.Caller{}, name, writeRaw(name), e)
		if !errors.Is(err, sink) || !errors.Is(err, settlement) || a.begins != 1 || a.ends != 1 || len(e.frames) != 1 {
			t.Fatal(name, err, a, e)
		}
		sp := ops[0].Spec()
		raw := []byte(`{"id":"","seq":0,"ts_unix_ms":0,"prev_hash":"","subject":"market.install","actor":"market","kind":"market.install.progress","payload":{"stage":"owned","ok":false}}`)
		if schema.ValidateJSON(sp.EmissionSchema, raw) != nil {
			t.Fatal("valid progress rejected")
		}
		var frame map[string]any
		json.Unmarshal(raw, &frame)
		delete(frame, "payload")
		encoded, _ := json.Marshal(frame)
		if schema.ValidateJSON(sp.EmissionSchema, encoded) == nil {
			t.Fatal("missing payload")
		}
		json.Unmarshal(raw, &frame)
		delete(frame["payload"].(map[string]any), "stage")
		encoded, _ = json.Marshal(frame)
		if schema.ValidateJSON(sp.EmissionSchema, encoded) == nil {
			t.Fatal("missing stage")
		}
		json.Unmarshal(raw, &frame)
		delete(frame["payload"].(map[string]any), "ok")
		encoded, _ = json.Marshal(frame)
		if schema.ValidateJSON(sp.EmissionSchema, encoded) == nil {
			t.Fatal("missing ok")
		}
	}
}
func TestWriteOperationsRequiredTerminalRootsAndPartialErrorPresence(t *testing.T) {
	ops, err := WriteOperations(func(context.Context) *Writes { return nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		sp := op.Spec()
		var out any
		var keys []string
		switch sp.Name {
		case "market_install":
			out = InstallOutput{}
			keys = []string{"name", "version", "marketplace", "installed_ms"}
		case "market_uninstall":
			out = UninstallOutput{}
			keys = []string{"uninstalled"}
		case "market_add_source":
			out = AddSourceOutput{}
			keys = []string{"name", "url"}
		case "market_remove_source":
			out = RemoveSourceOutput{}
			keys = []string{"name", "removed"}
		case "market_sync":
			out = SyncOutput{}
			keys = []string{"results", "synced", "packs"}
		}
		var decl map[string]any
		json.Unmarshal(sp.OutputSchema, &decl)
		required := decl["required"].([]any)
		if len(required) != len(keys) {
			t.Fatal(sp.Name, decl)
		}
		value := readJSON(t, out)
		for _, key := range keys {
			clone := readJSON(t, value)
			delete(clone, key)
			raw, _ := json.Marshal(clone)
			if schema.ValidateJSON(sp.OutputSchema, raw) == nil {
				t.Fatal(sp.Name, "missing required", key)
			}
		}
	}
	empty := ""
	raw, _ := json.Marshal(SyncOutput{Results: []core.SyncResult{}, PartialError: &empty})
	if schema.ValidateJSON(ops[4].Spec().OutputSchema, raw) != nil || !strings.Contains(string(raw), `"partial_error":""`) {
		t.Fatal(string(raw))
	}
}
