// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestChannelSendValidationPrecedesUnavailableSenderAndEffect(t *testing.T) {
	for _, in := range []SendInput{{}, {Channel: " "}, {Channel: "slack", To: "owned"}, {Channel: "slack", Text: "owned"}, {To: "owned", Text: "owned"}, {Channel: "slack", To: " ", Text: "owned"}} {
		for _, available := range []bool{false, true} {
			calls, replies := 0, 0
			var sender Sender
			if available {
				sender = func(context.Context, string, string, string) error { calls++; return nil }
			}
			NewOutbound(sender).Send(context.Background(), in, func(out SendOutput, err error) {
				replies++
				if out != nil || err == nil || err.Error() != "send requires channel, to, and text" {
					t.Fatal(out, err)
				}
			})
			if replies != 1 || calls != 0 {
				t.Fatal(replies, calls)
			}
		}
	}
	replies := 0
	NewOutbound(nil).Send(context.Background(), SendInput{Channel: "slack", To: "owned", Text: "fixture"}, func(out SendOutput, err error) {
		replies++
		if out != nil || err == nil || err.Error() != "no channels configured (set a channel token to enable send)" {
			t.Fatal(out, err)
		}
	})
	if replies != 1 {
		t.Fatal(replies)
	}
}

func TestChannelSendSelectedArgumentsDeadlineCauseAndTerminalLifetime(t *testing.T) {
	type ownedKey struct{}
	for _, senderError := range []error{nil, errors.New("owned sender failure")} {
		ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ownedKey{}, "caller value"))
		cancel()
		var callContext context.Context
		calls, replies := 0, 0
		before := time.Now()
		NewOutbound(func(call context.Context, kind, to, text string) error {
			callContext = call
			calls++
			deadline, ok := call.Deadline()
			if !ok || deadline.Before(before.Add(29*time.Second)) || deadline.After(time.Now().Add(30*time.Second)) || call.Err() != nil || call.Value(ownedKey{}) != nil || kind != "slack" || to != "owned target" || text != "line1\nline2" {
				t.Fatal("selected sender arguments/context", kind, to, text)
			}
			return senderError
		}).Send(ctx, SendInput{Channel: " SLACK ", To: " owned target ", Text: " \tline1\nline2\t "}, func(out SendOutput, err error) {
			replies++
			if callContext == nil || callContext.Err() != nil {
				t.Fatal("context released before terminal callback")
			}
			if senderError != nil {
				if out != nil || !errors.Is(err, senderError) {
					t.Fatal(out, err)
				}
			} else if err != nil || !reflect.DeepEqual(out, SendOutput{"sent": true, "channel": "slack", "to": "owned target"}) {
				t.Fatal(out, err)
			}
		})
		if calls != 1 || replies != 1 || !errors.Is(callContext.Err(), context.Canceled) {
			t.Fatal(calls, replies, callContext.Err())
		}
	}
}

func TestChannelSendContextReleasedOnSenderOrTerminalPanic(t *testing.T) {
	for _, panicInSender := range []bool{false, true} {
		var callContext context.Context
		terminalCalls := 0
		func() {
			defer func() {
				if recover() != "owned panic" {
					t.Error("panic identity changed")
				}
			}()
			NewOutbound(func(ctx context.Context, _, _, _ string) error {
				callContext = ctx
				if panicInSender {
					panic("owned panic")
				}
				return nil
			}).Send(context.Background(), SendInput{Channel: "slack", To: "owned", Text: "fixture"}, func(SendOutput, error) { terminalCalls++; panic("owned panic") })
		}()
		want := 1
		if panicInSender {
			want = 0
		}
		if terminalCalls != want || callContext == nil || !errors.Is(callContext.Err(), context.Canceled) {
			t.Fatal(terminalCalls, callContext)
		}
	}
}
