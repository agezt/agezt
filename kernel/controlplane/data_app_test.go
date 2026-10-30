// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"github.com/agezt/agezt/kernel/app/binding"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/modules/artifacts/datalake"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestDataEditsThePrimaryLake drives the typed data lake operations through
// the native adapter against the primary kernel's lake.
//
// The lake is opened HERE and handed to the kernel as the contract's port:
// the lake's files are the composition root's business (W3.42), so the host
// speaks the port instead of the kernel opening a module's lake.
func TestDataEditsThePrimaryLake(t *testing.T) {
	lake, err := datalake.Open(t.TempDir(), func() int64 { return time.Now().UnixMilli() })
	if err != nil {
		t.Fatal(err)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Lake: lake})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(binding.ScopeFromKernel(k, t.TempDir()))
	s.token = "primary"
	call := func(cmd string, args map[string]any) (string, string) {
		t.Helper()
		resp := callAppHost(t, s, Request{ID: "d", Cmd: cmd, Token: "primary", Args: args})[0]
		raw, _ := json.Marshal(resp.Result)
		return string(raw), resp.Error
	}
	if _, msg := call(CmdDataCreateCollection, map[string]any{"collection": map[string]any{"name": "books"}}); msg != "" {
		t.Fatal(msg)
	}
	out, msg := call(CmdDataInsert, map[string]any{"collection": "books", "record": map[string]any{"title": "Dune"}})
	if msg != "" || !strings.Contains(out, `"created_by":"operator"`) {
		t.Fatal(out, msg)
	}
	var inserted struct {
		Record struct {
			ID string `json:"id"`
		} `json:"record"`
	}
	if err := json.Unmarshal([]byte(out), &inserted); err != nil {
		t.Fatal(err)
	}
	recs, err := k.DataLake().Query("books", datalake.Query{})
	if err != nil || len(recs) != 1 || recs[0].ID != inserted.Record.ID || recs[0].Fields["title"] != "Dune" {
		t.Fatal("the insert reached the primary lake", recs, err)
	}
	if out, msg := call(CmdDataRecords, map[string]any{"collection": "books"}); msg != "" || !strings.Contains(out, `"count":1`) {
		t.Fatal(out, msg)
	}
	if _, msg := call(CmdDataDropCollection, map[string]any{"name": "books"}); msg != "" {
		t.Fatal(msg)
	}
	if _, ok := k.DataLake().Schema("books"); ok {
		t.Fatal("the drop reached the primary lake")
	}
	for cmd, read := range map[string]bool{CmdDataCollections: true, CmdDataRecords: true, CmdDataInsert: false, CmdDataUpdate: false, CmdDataDelete: false, CmdDataCreateCollection: false, CmdDataDropCollection: false} {
		if wire, exists := commandRegistry[cmd]; !exists || !wire.AppOwned || wire.ReadOnly != read || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire %+v", cmd, wire)
		}
	}
}

// The lake is the DAEMON's file, and a daemon that was given none holds none.
//
// The surface must REFUSE then, and not read from nothing: the port hands over
// no lake, the service says the lake is unavailable, and the operator is told
// so instead of being handed an empty list that reads like "there is no data".
func TestTheDataSurfaceRefusesWhenTheDaemonHoldsNoLake(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(binding.ScopeFromKernel(k, t.TempDir()))
	s.token = "primary"
	call := func(cmd string, args map[string]any) (string, string) {
		t.Helper()
		resp := callAppHost(t, s, Request{ID: "d", Cmd: cmd, Token: "primary", Args: args})[0]
		raw, _ := json.Marshal(resp.Result)
		return string(raw), resp.Error
	}
	out, msg := call(CmdDataCollections, nil)
	if msg == "" {
		t.Fatal("the data surface answered a daemon with no lake", out)
	}
	if !strings.Contains(msg, "data lake unavailable") {
		t.Fatal("the data surface refused for the wrong reason", msg)
	}
}
