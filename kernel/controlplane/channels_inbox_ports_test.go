// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"encoding/json"
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
)

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
	out, err := s.channelInbox().List(context.Background(), appchannels.InboxInput{})
	raw, _ := json.Marshal(out)
	if err != nil || out.Count != 1 || !containsInboxBytes(raw, []byte(`"correlation_id":"selected"`)) || containsInboxBytes(raw, []byte(`"correlation_id":"other"`)) {
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
