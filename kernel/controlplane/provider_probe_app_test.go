// SPDX-License-Identifier: MIT

package controlplane

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestProviderProbeMetadataComesFromPrimaryAppSpec(t *testing.T) {
	if len(probeOperations) != 1 {
		t.Fatalf("operations = %d", len(probeOperations))
	}
	spec := probeOperations[0].Spec()
	wire := commandRegistry[CmdProviderProbe]
	if !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary {
		t.Fatalf("primary metadata = %+v %+v", spec, wire)
	}
}

func TestProviderProbeAppSocketAdmissionFailureShapeAndReadOnly(t *testing.T) {
	var requests atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/models" || r.Header.Get("Authorization") != "Bearer fixture" {
			t.Errorf("probe lost path/key: %s %v", r.URL, r.Header)
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(endpoint.Close)
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, dir)
	s.token = "primary"
	call := func(args map[string]any) Response {
		responses := callAppHost(t, s, Request{ID: "probe", Cmd: CmdProviderProbe, Token: "primary", Args: args})
		return responses[len(responses)-1]
	}
	response := call(map[string]any{"url": endpoint.URL + "/", "key": " fixture ", "tenant": "ignored", "ignored": true})
	if response.Type != RespResult || response.Result["ok"] != true || response.Result["reachable"] != true || response.Result["authorized"] != false || response.Result["http_status"] != float64(401) || response.Result["models"] != float64(0) || len(response.Result) != 5 {
		t.Fatalf("unauthorized endpoint response = %+v", response)
	}
	for _, args := range []map[string]any{{"url": 42}, {"url": endpoint.URL, "key": 42}, nil} {
		before := requests.Load()
		if response := call(args); response.Type != RespError || requests.Load() != before {
			t.Fatalf("invalid args entered endpoint: %+v requests=%d/%d", response, requests.Load(), before)
		}
	}
	endpoint.Close()
	response = call(map[string]any{"url": endpoint.URL, "key": "fixture"})
	failure, _ := response.Result["error"].(string)
	if response.Type != RespResult || response.Result["ok"] != false || failure == "" || len(response.Result) != 2 {
		t.Fatalf("unreachable endpoint response = %+v", response)
	}
	operations := 0
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindOpInvoked || e.Kind == event.KindOpCompleted || e.Kind == event.KindOpFailed {
			operations++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if operations != 0 {
		t.Fatalf("read-only probe wrote operation audit: %d", operations)
	}
}
