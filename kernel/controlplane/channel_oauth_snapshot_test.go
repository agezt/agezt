// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/app"
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"
)

func oauthSnapshotResponse(t *testing.T, s *Server) Response {
	t.Helper()
	a, b := net.Pipe()
	defer a.Close()
	a.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer b.Close()
		handleOAuthSnapshotOperation(s, b, Request{ID: "owned", Args: map[string]any{"state": "owned"}})
	}()
	line, err := bufio.NewReader(a).ReadBytes(10)
	if err != nil {
		t.Fatal(err)
	}
	a.Close()
	<-done
	var reply Response
	if err := json.Unmarshal(line, &reply); err != nil {
		t.Fatal(err)
	}
	return reply
}

func TestChannelOAuthStatusConcurrentSnapshot(t *testing.T) {
	s := oauthSnapshotServer(appchannels.OAuthFlow{Kind: "slack", Label: "work", Status: "pending", Created: time.Now()})
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 10000; i++ {
			s.channelOAuthState.SetStatus("owned", "done", "done-message")
			runtime.Gosched()
			s.channelOAuthState.SetStatus("owned", "error", "error-message")
			runtime.Gosched()
		}
	}()
	close(start)
	for i := 0; i < 1000; i++ {
		reply := oauthSnapshotResponse(t, s)
		if reply.Type != RespResult || len(reply.Result) != 4 || reply.Result["kind"] != "slack" || reply.Result["label"] != "work" {
			t.Fatal(reply)
		}
		status, msg := reply.Result["status"], reply.Result["error"]
		if !(status == "pending" && msg == "" || status == "done" && msg == "done-message" || status == "error" && msg == "error-message") {
			t.Errorf("EXPECTED:atomic status/error snapshot ACTUAL:status=%v error=%v", status, msg)
		}
	}
	wg.Wait()
}

type pausedOAuthStatusConn struct {
	net.Conn
	entered, release chan struct{}
}

func (c pausedOAuthStatusConn) Write(p []byte) (int, error) {
	close(c.entered)
	<-c.release
	return c.Conn.Write(p)
}

func TestChannelOAuthStatusSnapshotDoesNotHoldMutexDuringWrite(t *testing.T) {
	s := oauthSnapshotServer(appchannels.OAuthFlow{Kind: "slack", Label: "work", Status: "pending"})
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	a.SetDeadline(time.Now().Add(3 * time.Second))
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleOAuthSnapshotOperation(s, pausedOAuthStatusConn{Conn: b, entered: entered, release: release}, Request{ID: "owned", Args: map[string]any{"state": "owned"}})
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("status did not reach writer")
	}
	updated := make(chan struct{})
	go func() { s.channelOAuthState.SetStatus("owned", "done", "completed"); close(updated) }()
	select {
	case <-updated:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("status held state mutex across socket write")
	}
	close(release)
	line, err := bufio.NewReader(a).ReadBytes(10)
	if err != nil {
		t.Fatal(err)
	}
	a.Close()
	<-done
	var reply Response
	if err := json.Unmarshal(line, &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Result["status"] != "pending" || reply.Result["error"] != "" {
		t.Fatal("captured snapshot changed during write", reply.Result)
	}
}

func TestChannelOAuthStatusKnownUnknownRawShape(t *testing.T) {
	s := oauthSnapshotServer(appchannels.OAuthFlow{Kind: " raw kind ", Label: " raw label ", Status: "error", Error: " raw error "})
	reply := oauthSnapshotResponse(t, s)
	if reply.ID != "owned" || len(reply.Result) != 4 || reply.Result["kind"] != " raw kind " || reply.Result["label"] != " raw label " || reply.Result["error"] != " raw error " {
		t.Fatal(reply)
	}
	reply = oauthSnapshotResponse(t, &Server{})
	if len(reply.Result) != 1 || reply.Result["status"] != "unknown" {
		t.Fatal(reply)
	}
}

func oauthSnapshotServer(flow appchannels.OAuthFlow) *Server {
	s := &Server{}
	_ = s.channelOAuth()
	s.channelOAuthState.Put("owned", flow, time.Now())
	return s
}

// Snapshot-only host avoids runtime stores; native admission has separate tests.
type oauthSnapshotAuth struct{}

func (oauthSnapshotAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return opapi.Principal{Kind: opapi.Operator}, nil
}

type oauthSnapshotRoute struct{ server *Server }

func (r oauthSnapshotRoute) Route(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) {
	return context.WithValue(ctx, systemHostKey{}, r.server), nil
}
func handleOAuthSnapshotOperation(s *Server, conn net.Conn, req Request) {
	s.operationOnce.Do(func() {
		s.operations, s.operationErr = app.NewDispatcher(channelOAuthOperations, app.Dependencies{Auth: oauthSnapshotAuth{}, Router: oauthSnapshotRoute{s}})
	})
	req.Cmd = CmdChannelOAuthStatus
	handleAppOperation(&DispatchCtx{S: s, Conn: conn, Req: req, Ctx: context.Background()})
}
