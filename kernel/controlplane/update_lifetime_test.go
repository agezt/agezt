// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	core "github.com/agezt/agezt/kernel/update"
)

type ownedUpdateBackend struct {
	call      context.Context
	cause     error
	panicCall bool
	info      *core.UpdateInfo
}

func (u *ownedUpdateBackend) Check(ctx context.Context) (*core.CheckResult, error) {
	u.call = ctx
	if u.panicCall {
		panic("owned updater panic")
	}
	return &core.CheckResult{Current: "owned-current"}, u.cause
}
func (u *ownedUpdateBackend) Apply(ctx context.Context, info *core.UpdateInfo, _ func(context.Context, time.Duration) core.DrainResult) error {
	u.call = ctx
	u.info = info
	if u.panicCall {
		panic("owned updater panic")
	}
	return u.cause
}
func handleUpdateLifetime(s *Server, conn net.Conn, cmd string) {
	req := Request{ID: "owned", Cmd: cmd, Args: map[string]any{"version": " raw-version ", "sha256": "raw-sha", "url": "raw-url", "notes": " raw-notes "}}
	if cmd == CmdUpdateCheck {
		s.handleUpdateCheck(conn, req)
	} else {
		s.handleUpdateApply(conn, req)
	}
}

func TestUpdateNativeContextAndRestartFollowTerminalWrite(t *testing.T) {
	for _, cmd := range []string{CmdUpdateCheck, CmdUpdateApply} {
		for _, cause := range []error{nil, errors.New("owned update error")} {
			t.Run(cmd+"/"+func() string {
				if cause == nil {
					return "success"
				}
				return "error"
			}(), func(t *testing.T) {
				backend := &ownedUpdateBackend{cause: cause}
				s := &Server{updateSvc: backend, baseDir: t.TempDir(), shutdownCh: make(chan struct{})}
				a, b := net.Pipe()
				a.SetDeadline(time.Now().Add(3 * time.Second))
				entered, release := make(chan struct{}), make(chan struct{})
				done := make(chan struct{})
				go func() {
					defer close(done)
					handleUpdateLifetime(s, pausedOAuthStatusConn{Conn: b, entered: entered, release: release}, cmd)
				}()
				select {
				case <-entered:
				case <-time.After(time.Second):
					t.Fatal("no terminal writer")
				}
				if backend.call == nil || backend.call.Err() != nil {
					close(release)
					a.Close()
					b.Close()
					<-done
					t.Fatal("EXPECTED: updater context alive through write; ACTUAL: canceled")
				}
				deadline, has := backend.call.Deadline()
				if cmd == CmdUpdateCheck && (!has || time.Until(deadline) < 59*time.Second || time.Until(deadline) > 60*time.Second) || cmd == CmdUpdateApply && has {
					t.Fatal("context deadline", deadline, has)
				}
				if cmd == CmdUpdateApply && cause == nil {
					raw, err := os.ReadFile(filepath.Join(s.baseDir, "update.sentinel"))
					if err != nil {
						t.Fatal("sentinel must precede response", err)
					}
					if _, err := time.Parse(time.RFC3339, string(raw[:len(raw)-1])); err != nil {
						t.Fatal(err)
					}
					if backend.info.Version != " raw-version " || backend.info.Notes != " raw-notes " || backend.info.Provenance != core.ProvenanceUnverified || backend.info.Signature != "" {
						t.Fatal("manifest changed", backend.info)
					}
					select {
					case <-s.shutdownCh:
						t.Fatal("restart before response")
					case <-time.After(150 * time.Millisecond):
					}
				}
				returned := time.Now()
				close(release)
				if _, err := bufio.NewReader(a).ReadBytes(10); err != nil {
					t.Fatal(err)
				}
				a.Close()
				b.Close()
				<-done
				if !errors.Is(backend.call.Err(), context.Canceled) {
					t.Fatal("context leaked after terminal", backend.call.Err())
				}
				if cmd == CmdUpdateApply && cause == nil {
					select {
					case <-s.shutdownCh:
						if time.Since(returned) < 90*time.Millisecond {
							t.Fatal("restart delay lost")
						}
					case <-time.After(time.Second):
						t.Fatal("restart missing")
					}
				} else {
					select {
					case <-s.shutdownCh:
						t.Fatal("failed/check operation restarted")
					default:
					}
				}
			})
		}
	}
}

func TestUpdateNativeWriteFailureVersusPanicRestart(t *testing.T) {
	for _, panics := range []bool{false, true} {
		backend := &ownedUpdateBackend{}
		s := &Server{updateSvc: backend, baseDir: t.TempDir(), shutdownCh: make(chan struct{})}
		a, b := net.Pipe()
		func() {
			defer func() {
				value := recover()
				if panics && value != "owned terminal write panic" || !panics && value != nil {
					t.Error(value)
				}
			}()
			handleUpdateLifetime(s, failedSendWriteConn{Conn: b, panicWrite: panics}, CmdUpdateApply)
		}()
		a.Close()
		b.Close()
		if backend.call == nil || !errors.Is(backend.call.Err(), context.Canceled) {
			t.Fatal("write failure leaked context")
		}
		if panics {
			select {
			case <-s.shutdownCh:
				t.Fatal("panic restarted")
			case <-time.After(150 * time.Millisecond):
			}
		} else {
			select {
			case <-s.shutdownCh:
			case <-time.After(time.Second):
				t.Fatal("returned failed write must still schedule restart")
			}
		}
	}
}

func TestUpdateNativeSenderPanicCancelsImmediately(t *testing.T) {
	for _, cmd := range []string{CmdUpdateCheck, CmdUpdateApply} {
		backend := &ownedUpdateBackend{panicCall: true}
		s := &Server{updateSvc: backend, shutdownCh: make(chan struct{})}
		a, b := net.Pipe()
		func() {
			defer func() {
				if value := recover(); value != "owned updater panic" {
					t.Error(value)
				}
			}()
			handleUpdateLifetime(s, b, cmd)
		}()
		a.Close()
		b.Close()
		if backend.call == nil || !errors.Is(backend.call.Err(), context.Canceled) {
			t.Fatal("backend panic leaked context")
		}
		select {
		case <-s.shutdownCh:
			t.Fatal("backend panic restarted")
		default:
		}
	}
}

func TestUpdateNativeSuccessfulRestartWaitsForWriteReturn(t *testing.T) {
	backend := &ownedUpdateBackend{}
	s := &Server{updateSvc: backend, baseDir: t.TempDir(), shutdownCh: make(chan struct{})}
	a, b := net.Pipe()
	a.SetDeadline(time.Now().Add(3 * time.Second))
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleUpdateLifetime(s, pausedOAuthStatusConn{Conn: b, entered: entered, release: release}, CmdUpdateApply)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("no writer")
	}
	select {
	case <-s.shutdownCh:
		t.Error("EXPECTED: restart only after writer returns; ACTUAL: shutdown while response blocked")
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	if _, err := bufio.NewReader(a).ReadBytes(10); err != nil {
		t.Fatal(err)
	}
	a.Close()
	b.Close()
	<-done
	select {
	case <-s.shutdownCh:
	case <-time.After(time.Second):
		t.Fatal("restart missing")
	}
}

func TestUpdateNativeNilSetterReportsDisabledBeforeCodec(t *testing.T) {
	s := &Server{}
	var backend *core.Service
	s.SetUpdateService(backend)
	for _, cmd := range []string{CmdUpdateCheck, CmdUpdateApply} {
		a, b := net.Pipe()
		a.SetDeadline(time.Now().Add(time.Second))
		done := make(chan struct{})
		go func() {
			defer close(done)
			req := Request{ID: "owned", Args: map[string]any{"version": false}}
			defer s.recoverConn(b, &req)
			if cmd == CmdUpdateCheck {
				s.handleUpdateCheck(b, req)
			} else {
				s.handleUpdateApply(b, req)
			}
		}()
		line, err := bufio.NewReader(a).ReadBytes(10)
		a.Close()
		b.Close()
		<-done
		if err != nil {
			t.Fatal(err)
		}
		var reply Response
		json.Unmarshal(line, &reply)
		if cmd == CmdUpdateCheck {
			if reply.Type != RespResult || reply.Result["status"] != "update is disabled" || reply.Result["current"] != core.CurrentVersion || reply.Result["up_to_date"] != true || reply.Result["update"] != nil || len(reply.Result) != 4 {
				t.Fatal(reply)
			}
		} else if reply.Type != RespError || reply.Error != "update is disabled" {
			t.Fatal(reply)
		}
	}
}
