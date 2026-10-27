// SPDX-License-Identifier: MIT

package channel

import (
	"fmt"

	"github.com/agezt/agezt/kernel/event"
)

// Publisher is the port the guard journals a recovered panic through.
//
// The channel package is a CONTRACT (layer 1) and the bus is PLATFORM
// (layer 2): a contract may not import the platform, so the guard takes a
// port and the bus satisfies it (W5.4d). The port is one verb wide, because
// the guard publishes one event.
//
// A publisher may be a NIL interface (no publisher is configured) — the guard
// then recovers the panic and journals nothing. It may also be a nil
// *bus.Bus handed to this parameter, which is a NON-nil interface: the
// publish would panic inside the very recover that is supposed to contain the
// damage. The journal helper below is therefore panic-safe on its own.
type Publisher interface {
	Publish(event.Spec) (*event.Event, error)
}

// Guard runs fn, recovering from a panic so a single malformed inbound message
// can't crash the daemon. Channels process untrusted external input on long-lived
// goroutines (poll loops, per-message goroutines); in Go an unrecovered panic in
// any goroutine takes down the whole process — every run and channel with it. Wrap
// per-message handling in Guard so a handler bug is contained to that one message.
//
// The panic is journalled as a channel.error event (when a publisher is supplied) with
// the channel name and the recovered value, so it stays diagnosable in
// `agt journal` rather than vanishing into a silent recover.
//
// It reports whether it **attempted** to journal the recovered panic. A daemon
// with no publisher configured has nowhere to publish to, and the guard does
// not pretend otherwise: it reports false, and the caller can see that the
// failure was not recorded. A publisher that is broken in its own right is
// still an attempt — the guard tried, and the publisher refused.
func Guard(p Publisher, channelName string, fn func()) (attempted bool) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if p == nil {
			// No publisher: there is nowhere to publish to, and a nil
			// publisher behind a non-nil interface would raise a panic of
			// its own on every error event.
			return
		}
		attempted = true
		journalChannelError(p, channelName, r)
	}()
	fn()
	return attempted
}

// journalChannelError publishes the recovered panic.
//
// The publish is panic-safe on its own: a publisher that is broken (a nil bus
// behind a non-nil interface) must not take the daemon with it — Guard exists
// precisely so that one bad message cannot end the process, and a crash in
// the error path would defeat it.
func journalChannelError(p Publisher, channelName string, r any) {
	defer func() { _ = recover() }()
	_, _ = p.Publish(event.Spec{
		Subject: "channel." + channelName + ".error",
		Kind:    event.KindChannelError,
		Actor:   "channel-" + channelName,
		Payload: map[string]any{
			"channel": channelName,
			"panic":   fmt.Sprint(r),
		},
	})
}
