// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
)

func TestChannelInboxNativeCodecPreservesFloatOnlyLimitAndDelayedCursorError(t *testing.T) {
	for _, value := range []any{nil, "5", false, 5, json.Number("5"), map[string]any{}} {
		in := inboxInput(Request{Args: map[string]any{"limit": value, "channel": false, "cursor": 5}})
		if in.Limit != nil || in.Channel != "" || in.Cursor != "" || in.CursorError == nil || in.CursorError.Error() != "args.cursor must be a string" {
			t.Fatal(value, in)
		}
	}
	in := inboxInput(Request{Args: map[string]any{"limit": 5.9, "channel": " Raw Channel ", "cursor": " 100:z "}})
	if in.Limit == nil || *in.Limit != 5 || in.Channel != " Raw Channel " || in.Cursor != " 100:z " || in.CursorError != nil {
		t.Fatal(in)
	}
	in = inboxInput(Request{})
	if in.Limit != nil || in.Channel != "" || in.Cursor != "" || in.CursorError != nil {
		t.Fatal(in)
	}
}

func TestChannelInboxNativeSelectedKernelJournalIsolation(t *testing.T) {
	p := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: p})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	other, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	for _, fixture := range []struct {
		kernel *runtime.Kernel
		corr   string
	}{{k, "selected"}, {other, "other"}} {
		if _, err := fixture.kernel.Journal().Append(event.Spec{Subject: "owned.channel", Kind: event.KindChannelInbound, Actor: "owned", CorrelationID: fixture.corr, Payload: map[string]any{"text": fixture.corr}}); err != nil {
			t.Fatal(err)
		}
	}
	s := NewServer(k, t.TempDir())
	head, hash := k.Journal().Head()
	otherHead, otherHash := other.Journal().Head()
	out, err := s.channelInbox().List(context.Background(), inboxInput(Request{Args: map[string]any{"tenant": "other"}}))
	raw, _ := json.Marshal(out)
	if err != nil || out["count"] != 1 || !containsInboxBytes(raw, []byte(`"correlation_id":"selected"`)) || containsInboxBytes(raw, []byte(`"correlation_id":"other"`)) {
		t.Fatal(string(raw), err)
	}
	after, afterHash := k.Journal().Head()
	otherAfter, otherAfterHash := other.Journal().Head()
	if head != after || hash != afterHash || otherHead != otherAfter || otherHash != otherAfterHash || p.CallCount() != 0 {
		t.Fatal("read changed journals/provider")
	}
}

func containsInboxBytes(data, pattern []byte) bool {
	return strings.Contains(string(data), string(pattern))
}
