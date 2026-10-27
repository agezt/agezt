// SPDX-License-Identifier: MIT
package controlplane

import (
	"testing"

	core "github.com/agezt/agezt/kernel/contract/updateapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestTheUpdateOperationsReachTheContractPort: with the port wired, the
// update operations answer with the PORT's rows — the daemon's engine is
// never consulted directly.
//
// The wiring goes through the daemon's DEPS, because that is the door the
// daemon comes through: a setter alone would not prove the deps reach the
// server.
func TestTheUpdateOperationsReachTheContractPort(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	backend := &ownedUpdateBackend{}

	s := NewServerWithDeps(k, dir, Deps{UpdateSvc: backend})
	s.token = "primary"

	frames := callAppHost(t, s, Request{ID: "owned", Cmd: CmdUpdateCheck, Token: "primary"})
	last := frames[len(frames)-1]
	if last.Type != RespResult || last.Result["current"] != "owned-current" {
		t.Fatalf("EXPECTED:the port's answer ACTUAL:%+v", last)
	}
}

// TestTheUpdateOperationsWithoutAPortReportDisabled: a daemon with no
// update engine answers "update is disabled" — it does not dereference nil.
func TestTheUpdateOperationsWithoutAPortReportDisabled(t *testing.T) {
	_, s, _, _ := pulseAppFixture(t)
	s.SetUpdateService(nil)

	frames := callAppHost(t, s, Request{ID: "owned", Cmd: CmdUpdateCheck, Token: "primary"})
	last := frames[len(frames)-1]
	if last.Type != RespResult || last.Result["status"] != "update is disabled" {
		t.Fatalf("EXPECTED:update is disabled ACTUAL:%+v", last)
	}
	if last.Result["current"] == "" {
		t.Fatalf("EXPECTED:the running version in the answer ACTUAL:%+v", last.Result)
	}
}

// TestTheUpdatePortIsTheContractPort: the port the server holds is the
// contract's — the module's service is one implementation of it.
func TestTheUpdatePortIsTheContractPort(t *testing.T) {
	_, s, _, _ := pulseAppFixture(t)
	var port core.Backend = &ownedUpdateBackend{}
	s.SetUpdateService(port)
	frames := callAppHost(t, s, Request{ID: "owned", Cmd: CmdUpdateCheck, Token: "primary"})
	if last := frames[len(frames)-1]; last.Type != RespResult {
		t.Fatalf("EXPECTED:the wired port answers ACTUAL:%+v", last)
	}
}
