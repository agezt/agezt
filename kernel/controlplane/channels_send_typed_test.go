// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestChannelSendNativeTypedMetadata(t *testing.T) {
	if len(channelSendOperations) != 1 {
		t.Fatal(channelSendOperations)
	}
	s := channelSendOperations[0].Spec()
	wire := commandRegistry[CmdSend]
	count := 0
	for _, op := range registeredAppOperations() {
		if op.Spec().Name == CmdSend {
			count++
		}
	}
	if count != 1 || !wire.AppOwned || wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Input != reflect.TypeFor[appchannels.SendRequest]() || s.Output != reflect.TypeFor[appchannels.SendOutput]() || s.HTTP.Method != "POST" || s.HTTP.Path != "/api/send" {
		t.Fatal(s, wire, count)
	}
}
func TestChannelSendNativeAuditAndCanceledBeforeSender(t *testing.T) {
	for _, gate := range []string{"closed-journal", "canceled"} {
		t.Run(gate, func(t *testing.T) {
			p := mock.New()
			k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: p})
			if err != nil {
				t.Fatal(err)
			}
			defer k.Close()
			s := NewServer(k, t.TempDir())
			s.token = "primary"
			sends := 0
			s.SetChannelSender(func(context.Context, string, string, string) error { sends++; return nil })
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
			defer a.Close()
			defer b.Close()
			a.SetDeadline(time.Now().Add(3 * time.Second))
			done := make(chan struct{})
			go func() { defer close(done); s.handleConn(ctx, b) }()
			raw, _ := json.Marshal(Request{ID: "owned", Cmd: CmdSend, Token: "primary", Args: map[string]any{"channel": "slack", "to": "owned", "text": "fixture"}})
			if _, err := a.Write(append(raw, 10)); err != nil {
				t.Fatal(err)
			}
			line, err := bufio.NewReader(a).ReadBytes(10)
			a.Close()
			<-done
			if err != nil {
				t.Fatal(err)
			}
			var reply Response
			json.Unmarshal(line, &reply)
			if reply.Type != RespError || sends != 0 || p.CallCount() != 0 || gate == "canceled" && reply.Error != "context canceled" {
				t.Fatal(reply, sends)
			}
		})
	}
}

type failedSendWriteConn struct {
	net.Conn
	panicWrite bool
}

func (c failedSendWriteConn) Write([]byte) (int, error) {
	if c.panicWrite {
		panic("owned terminal write panic")
	}
	return 0, errors.New("owned terminal write failure")
}
func TestChannelSendNativeContextReleasedOnTerminalWriteFailureOrPanic(t *testing.T) {
	for _, panics := range []bool{false, true} {
		s := &Server{}
		var call context.Context
		s.SetChannelSender(func(ctx context.Context, _, _, _ string) error { call = ctx; return nil })
		a, b := net.Pipe()
		func() {
			defer func() {
				value := recover()
				if panics && value != "owned terminal write panic" || !panics && value != nil {
					t.Error("write panic identity", value)
				}
			}()
			handleSendLifetimeOperation(s, failedSendWriteConn{Conn: b, panicWrite: panics}, Request{ID: "owned", Args: map[string]any{"channel": "slack", "to": "owned", "text": "fixture"}})
		}()
		a.Close()
		b.Close()
		if call == nil || !errors.Is(call.Err(), context.Canceled) {
			t.Fatal("terminal failure leaked sender context", call)
		}
	}
}
