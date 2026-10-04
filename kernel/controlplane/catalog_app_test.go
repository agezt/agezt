// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/agezt/agezt/kernel/app/catalog"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestCatalogCommandMetadataComesFromAppSpecs(t *testing.T) {
	if len(catalogOperations) != 3 {
		t.Fatalf("catalog operations=%d", len(catalogOperations))
	}
	for _, operation := range catalogOperations {
		spec := operation.Spec()
		wire := commandRegistry[spec.Name]
		if !wire.AppOwned || wire.ReadOnly != spec.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary {
			t.Fatalf("catalog metadata=%+v %+v", spec, wire)
		}
		if spec.Name == CmdCatalogSync && spec.Output != reflect.TypeFor[catalog.SyncOutput]() {
			t.Fatal("sync output is opaque")
		}
		if spec.Name == CmdCatalogList && spec.Output != reflect.TypeFor[catalog.ListOutput]() {
			t.Fatal("list output is opaque")
		}
		if spec.Name == CmdCatalogDiscover && spec.Output != reflect.TypeFor[catalog.DiscoverOutput]() {
			t.Fatal("discover output is opaque")
		}
	}
}

func TestCatalogAppSocketAdmissionAuditAndWire(t *testing.T) {
	for _, mode := range []string{"sync", "discover", "bad-input", "audit-unavailable", "fetch-error"} {
		t.Run(mode, func(t *testing.T) {
			var fetches, reloads atomic.Int32
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fetches.Add(1)
				if mode == "fetch-error" {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				if r.URL.Path == "/api/tags" {
					_, _ = w.Write([]byte(`{"models":[{"model":"fixture-model"}]}`))
					return
				}
				_, _ = w.Write([]byte(`{"fixture":{"id":"fixture","models":{"fixture-model":{"id":"fixture-model"}}}}`))
			}))
			defer ts.Close()
			dir := t.TempDir()
			k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), OnReload: func() error { reloads.Add(1); return nil }})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { k.Close() })
			s := NewServer(k, dir)
			s.token = "primary"
			command := CmdCatalogSync
			args := map[string]any{"url": ts.URL, "ignored": "legacy-compatible", "tenant": "ignored-primary-scope"}
			if mode == "discover" {
				command = CmdCatalogDiscover
				args = map[string]any{"endpoint": ts.URL, "ignored": true}
			}
			if mode == "bad-input" {
				args["url"] = 42
			}
			if mode == "audit-unavailable" {
				_ = k.Journal().Close()
			}
			responses := callAppHost(t, s, Request{ID: "catalog", Cmd: command, Token: "primary", Args: args})
			last := responses[len(responses)-1]
			wantError := mode == "bad-input" || mode == "audit-unavailable" || mode == "fetch-error"
			if (last.Type == RespError) != wantError {
				t.Fatalf("mode=%s terminal=%+v", mode, last)
			}
			if mode == "bad-input" || mode == "audit-unavailable" {
				if fetches.Load() != 0 || reloads.Load() != 0 {
					t.Fatalf("rejected catalog request reached effects: %d/%d", fetches.Load(), reloads.Load())
				}
				if mode == "bad-input" {
					count := 0
					if err := k.Journal().Range(func(e *event.Event) error {
						if e.Subject == "op."+command {
							count++
						}
						return nil
					}); err != nil {
						t.Fatal(err)
					}
					if count != 0 {
						t.Fatalf("invalid input entered operation audit: %d", count)
					}
				}
				return
			}
			var events []*event.Event
			var domainEvents []*event.Event
			if err := k.Journal().Range(func(e *event.Event) error {
				if e.Subject == "op."+command {
					events = append(events, e)
				}
				if e.Subject == "catalog.sync" || e.Subject == "catalog.discovery" {
					domainEvents = append(domainEvents, e)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			terminalKind := event.KindOpCompleted
			if mode == "fetch-error" {
				terminalKind = event.KindOpFailed
			}
			if len(events) != 2 || events[0].Kind != event.KindOpInvoked || events[1].Kind != terminalKind || events[0].CorrelationID == "" || events[0].CorrelationID != events[1].CorrelationID {
				t.Fatalf("catalog audit=%v", events)
			}
			if len(domainEvents) != 1 || domainEvents[0].CorrelationID != events[0].CorrelationID {
				t.Fatalf("catalog domain event lost operation identity: %v", domainEvents)
			}
			var admission map[string]any
			if err := json.Unmarshal(events[0].Payload, &admission); err != nil {
				t.Fatal(err)
			}
			if _, exists := admission["tenant"]; exists {
				t.Fatalf("primary catalog operation audited as caller-selected tenant: %v", admission)
			}
			if mode == "fetch-error" {
				if reloads.Load() != 0 {
					t.Fatal("failed fetch reloaded providers")
				}
				return
			}
			if last.Result["model_count"] != float64(1) || last.Result["providers_reloaded"] != true || fetches.Load() != 1 || reloads.Load() != 1 {
				t.Fatalf("catalog result=%v fetch/reload=%d/%d", last.Result, fetches.Load(), reloads.Load())
			}
			before, _ := k.Journal().Head()
			listed := callAppHost(t, s, Request{ID: "list", Cmd: CmdCatalogList, Token: "primary", Args: map[string]any{"ignored": true}})
			after, _ := k.Journal().Head()
			if listed[0].Type != RespResult || listed[0].Result["provider_count"] != float64(1) || before != after {
				t.Fatalf("list wire/audit=%v journal=%d->%d", listed, before, after)
			}
		})
	}
}
