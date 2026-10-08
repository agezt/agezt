// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	appupdate "github.com/agezt/agezt/kernel/app/update"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/runtime"
	core "github.com/agezt/agezt/kernel/update"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestUpdateNativeTwoTypedOperationsExit(t *testing.T) {
	expected := map[string][2]reflect.Type{CmdUpdateCheck: {reflect.TypeFor[appupdate.CheckRequest](), reflect.TypeFor[appupdate.CheckOutput]()}, CmdUpdateApply: {reflect.TypeFor[appupdate.ApplyRequest](), reflect.TypeFor[appupdate.ApplyOutput]()}}
	seen := map[string]int{}
	for _, op := range registeredAppOperations() {
		s := op.Spec()
		types, ok := expected[s.Name]
		if !ok {
			continue
		}
		wire, found := commandRegistry[s.Name]
		if !found || !wire.AppOwned || wire.ReadOnly != (s.Name == CmdUpdateCheck) || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Input != types[0] || s.Output != types[1] || !s.AllowUnknownInput || s.Stream != opapi.StreamNone || s.Emission != nil || len(s.EmissionSchema) != 0 || s.HTTP.Method != "" || s.HTTP.Path != "" {
			t.Fatal(s, wire)
		}
		seen[s.Name]++
	}
	if len(updateOperations) != 2 || len(seen) != 2 || seen[CmdUpdateCheck] != 1 || seen[CmdUpdateApply] != 1 {
		t.Fatal("update aggregate incomplete", seen)
	}
}

type closingUpdateBackend struct {
	ownedUpdateBackend
	close func() error
}

func (b *closingUpdateBackend) Apply(ctx context.Context, in *core.UpdateInfo, drain func(context.Context, time.Duration) core.DrainResult) error {
	if err := b.ownedUpdateBackend.Apply(ctx, in, drain); err != nil {
		return err
	}
	return b.close()
}

func TestUpdateNativeTerminalAuditFailureStillFinishesWriteBeforeRestart(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, t.TempDir())
	s.token = "primary"
	backend := &closingUpdateBackend{close: k.Journal().Close}
	s.updateSvc = backend
	a, b := net.Pipe()
	a.SetDeadline(time.Now().Add(3 * time.Second))
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.handleConn(context.Background(), pausedOAuthStatusConn{Conn: b, entered: entered, release: release})
	}()
	raw, _ := json.Marshal(Request{ID: "owned", Cmd: CmdUpdateApply, Token: "primary", Args: map[string]any{"version": "v", "sha256": "s", "url": "u"}})
	if _, err := a.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("no error writer")
	}
	if backend.call == nil || backend.call.Err() != nil {
		t.Fatal("context canceled before audit error write")
	}
	if _, err := os.Stat(filepath.Join(s.baseDir, "update.sentinel")); err != nil {
		t.Fatal("applied sentinel lost", err)
	}
	select {
	case <-s.shutdownCh:
		t.Fatal("restart before error writer return")
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	line, err := bufio.NewReader(a).ReadBytes(10)
	a.Close()
	b.Close()
	<-done
	if err != nil {
		t.Fatal(err)
	}
	var reply Response
	json.Unmarshal(line, &reply)
	if reply.Type != RespError || reply.Error == "" {
		t.Fatal("terminal audit failure hidden", reply)
	}
	if backend.call.Err() != context.Canceled {
		t.Fatal("context leaked after error writer")
	}
	select {
	case <-s.shutdownCh:
	case <-time.After(time.Second):
		t.Fatal("applied effect lost restart")
	}
}
func TestUpdateNativeCanceledAndAuditAdmissionBeforeBackend(t *testing.T) {
	for _, cmd := range []string{CmdUpdateCheck, CmdUpdateApply} {
		for _, gate := range []string{"canceled", "closed-journal"} {
			t.Run(cmd+"/"+gate, func(t *testing.T) {
				p := mock.New()
				k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: p})
				if err != nil {
					t.Fatal(err)
				}
				defer k.Close()
				s := NewServer(k, t.TempDir())
				s.token = "primary"
				backend := &ownedUpdateBackend{}
				s.updateSvc = backend
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if gate == "canceled" {
					cancel()
				} else {
					if err := k.Journal().Close(); err != nil {
						t.Fatal(err)
					}
				}
				a, b := net.Pipe()
				a.SetDeadline(time.Now().Add(3 * time.Second))
				done := make(chan struct{})
				go func() { defer close(done); s.handleConn(ctx, b) }()
				raw, _ := json.Marshal(Request{ID: "owned", Cmd: cmd, Token: "primary", Args: map[string]any{"version": "v", "sha256": "s", "url": "u"}})
				if _, err := a.Write(append(raw, 10)); err != nil {
					t.Fatal(err)
				}
				line, err := bufio.NewReader(a).ReadBytes(10)
				a.Close()
				b.Close()
				<-done
				if err != nil {
					t.Fatal(err)
				}
				var reply Response
				json.Unmarshal(line, &reply)
				admitted := gate == "closed-journal" && cmd == CmdUpdateCheck
				if admitted {
					if reply.Type != RespResult || backend.call == nil {
						t.Fatal("read requires audit", reply)
					}
				} else if reply.Type != RespError || backend.call != nil || gate == "canceled" && reply.Error != "context canceled" {
					t.Fatal("admission allowed backend", reply, backend.call)
				}
				if p.CallCount() != 0 {
					t.Fatal("provider invoked")
				}
			})
		}
	}
}
