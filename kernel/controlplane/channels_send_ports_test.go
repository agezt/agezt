// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"errors"
	"testing"

	appchannels "github.com/agezt/agezt/kernel/app/channels"
)

func TestChannelSendNativeSelectedSenderFreshnessAndServerIsolation(t *testing.T) {
	s, other := &Server{}, &Server{}
	selectedCalls, otherCalls := 0, 0
	sentinel := errors.New("owned replacement sender")
	s.SetChannelSender(func(context.Context, string, string, string) error { selectedCalls++; return nil })
	other.SetChannelSender(func(context.Context, string, string, string) error { otherCalls++; return nil })
	call := func(server *Server) error {
		var resultErr error
		replies := 0
		server.channelOutbound().Send(context.Background(), appchannels.SendInput{Channel: "slack", To: "owned", Text: "fixture"}, func(out appchannels.SendOutput, err error) { replies++; resultErr = err })
		if replies != 1 {
			t.Fatal(replies)
		}
		return resultErr
	}
	if err := call(s); err != nil || selectedCalls != 1 || otherCalls != 0 {
		t.Fatal(err, selectedCalls, otherCalls)
	}
	s.SetChannelSender(func(context.Context, string, string, string) error { selectedCalls++; return sentinel })
	if err := call(s); !errors.Is(err, sentinel) || selectedCalls != 2 || otherCalls != 0 {
		t.Fatal(err, selectedCalls, otherCalls)
	}
	s.SetChannelSender(nil)
	if err := call(s); err == nil || err.Error() != "no channels configured (set a channel token to enable send)" || selectedCalls != 2 || otherCalls != 0 {
		t.Fatal(err, selectedCalls, otherCalls)
	}
	if err := call(other); err != nil || selectedCalls != 2 || otherCalls != 1 {
		t.Fatal(err, selectedCalls, otherCalls)
	}
}
