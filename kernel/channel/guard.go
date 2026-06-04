// SPDX-License-Identifier: MIT

package channel

import (
	"fmt"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
)

// Guard runs fn, recovering from a panic so a single malformed inbound message
// can't crash the daemon. Channels process untrusted external input on long-lived
// goroutines (poll loops, per-message goroutines); in Go an unrecovered panic in
// any goroutine takes down the whole process — every run and channel with it. Wrap
// per-message handling in Guard so a handler bug is contained to that one message.
//
// The panic is journalled as a channel.error event (when a bus is supplied) with
// the channel name and the recovered value, so it stays diagnosable in
// `agt journal` rather than vanishing into a silent recover.
func Guard(b *bus.Bus, channelName string, fn func()) {
	defer func() {
		if r := recover(); r != nil && b != nil {
			_, _ = b.Publish(event.Spec{
				Subject: "channel." + channelName + ".error",
				Kind:    event.KindChannelError,
				Actor:   "channel-" + channelName,
				Payload: map[string]any{
					"channel": channelName,
					"panic":   fmt.Sprint(r),
				},
			})
		}
	}()
	fn()
}
