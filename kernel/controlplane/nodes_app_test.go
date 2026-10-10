// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type nodesRemoteRunTool struct{}

func (nodesRemoteRunTool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{Name: "remote_run", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (nodesRemoteRunTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	return toolapi.Result{}, nil
}

// TestNodeRegistryBindsTheServer: the registry reads the primary kernel's
// model and remote_run tool, and the peer list from the environment before the
// server's settings store, probing each peer.
func TestNodeRegistryBindsTheServer(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","version":"peer-1","model_count":2}`))
	}))
	defer peer.Close()
	t.Setenv("AGEZT_PEERS", "")
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Model: "binding-model", Tools: map[string]toolapi.Tool{"remote_run": nodesRemoteRunTool{}}})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	serverDir := t.TempDir()
	store := settings.NewStore(serverDir)
	store.Set("AGEZT_PEERS", "stored="+peer.URL+"|tok")
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, serverDir)
	s.token = "primary"
	call := func() string {
		t.Helper()
		resp := callAppHost(t, s, Request{ID: "n", Cmd: CmdNodeRegistry, Token: "primary"})[0]
		if resp.Type != RespResult {
			t.Fatal(resp.Error)
		}
		raw, _ := json.Marshal(resp.Result)
		return string(raw)
	}
	got := call()
	for _, want := range []string{
		`"capabilities":["controlplane","webui","agent-runtime","remote-run"]`,
		`"model":"binding-model"`,
		`{"auth":"token","id":"peer:stored","local":false,"model_count":2,"name":"stored","reachable":true,"status":"ok","url":"` + peer.URL + `","version":"peer-1"}`,
		`"count":2,"nodes":`,
		`"peer_count":1,"remote_run_registered":true}`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
	if strings.Contains(got, "tok\"") {
		t.Fatal("the token leaked", got)
	}
	t.Setenv("AGEZT_PEERS", "env="+peer.URL)
	if got := call(); !strings.Contains(got, `"id":"peer:env"`) || strings.Contains(got, "peer:stored") {
		t.Fatal("the environment wins", got)
	}
	if wire, exists := commandRegistry[CmdNodeRegistry]; !exists || !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
		t.Fatalf("native wire %+v", wire)
	}
}
