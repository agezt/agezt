// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
)

type inboxBindingJournal struct {
	events []*event.Event
	err    error
	calls  int
}

func (j *inboxBindingJournal) Range(fn func(*event.Event) error) error {
	j.calls++
	if j.err != nil {
		return j.err
	}
	for _, e := range j.events {
		if err := fn(e); err != nil {
			return err
		}
	}
	return nil
}
func inboxBindingWire(t *testing.T, s *Server, ctx context.Context, args map[string]any) []byte {
	t.Helper()
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	a.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(ctx, b) }()
	raw, _ := json.Marshal(Request{ID: "owned", Cmd: CmdInbox, Token: "primary", Args: args})
	if _, err := a.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(a).ReadBytes(10)
	a.Close()
	<-done
	if err != nil {
		t.Fatal(err)
	}
	return line
}
func TestChannelInboxNativeTypedMetadata(t *testing.T) {
	if len(channelInboxOperations) != 1 {
		t.Fatal(channelInboxOperations)
	}
	s := channelInboxOperations[0].Spec()
	wire := commandRegistry[CmdInbox]
	count := 0
	for _, op := range registeredAppOperations() {
		if op.Spec().Name == CmdInbox {
			count++
		}
	}
	if count != 1 || !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Input != reflect.TypeFor[appchannels.InboxRequest]() || s.Output != reflect.TypeFor[appchannels.InboxOutput]() || s.HTTP.Method != "GET" || s.HTTP.Path != "/api/inbox" {
		t.Fatal(s, wire, count)
	}
}
func TestChannelInboxNativeWireMemberOrderExactIntegersAndCanceledRange(t *testing.T) {
	p := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: p})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, t.TempDir())
	s.token = "primary"
	j := &inboxBindingJournal{events: []*event.Event{{ID: "owned", CorrelationID: "owned", Kind: event.KindChannelInbound, TSUnixMS: 9223372036854775807, Payload: json.RawMessage(`{"text":"text"}`)}}}
	providers := 0
	ops, err := appchannels.InboxOperations(func(ctx context.Context) *appchannels.Inbox {
		providers++
		if ctx.Value(systemHostKey{}).(*Server) != s {
			t.Fatal("selected server lost")
		}
		return appchannels.NewInbox(j)
	})
	if err != nil {
		t.Fatal(err)
	}
	s.operationOnce.Do(func() {
		s.operations, s.operationErr = app.NewDispatcher(ops, app.Dependencies{Auth: appAuthenticator{s}, Router: appTenantRouter{s}})
	})
	head, hash := k.Journal().Head()
	line := inboxBindingWire(t, s, context.Background(), map[string]any{"unknown": true})
	expected := `"threads":[{"correlation_id":"owned","channel_kind":"","channel_id":"","messages":[{"direction":"in","text":"text","ts_unix_ms":9223372036854775807,"event_id":"owned"}],"last_ts_unix_ms":9223372036854775807}]`
	if !strings.Contains(string(line), expected) || providers != 1 || j.calls != 1 {
		t.Fatal("legacy nested declaration order/int64 erased", string(line), providers, j.calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	line = inboxBindingWire(t, s, ctx, map[string]any{"cursor": false})
	var reply Response
	json.Unmarshal(line, &reply)
	if reply.Type != RespError || reply.Error != "context canceled" || providers != 1 || j.calls != 1 {
		t.Fatal("canceled reader entered", reply, providers, j.calls)
	}
	j.err = errors.New("owned range failure")
	line = inboxBindingWire(t, s, context.Background(), map[string]any{"cursor": false})
	json.Unmarshal(line, &reply)
	if reply.Type != RespError || reply.Error != "owned range failure" || j.calls != 2 {
		t.Fatal("range error precedence changed", reply, j.calls)
	}
	j.err = nil
	line = inboxBindingWire(t, s, context.Background(), map[string]any{"cursor": nil})
	json.Unmarshal(line, &reply)
	if reply.Type != RespError || reply.Error != "args.cursor must be a string" || j.calls != 3 {
		t.Fatal("cursor error skipped range", reply, j.calls)
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || p.CallCount() != 0 {
		t.Fatal("read audited/model invoked")
	}
}
