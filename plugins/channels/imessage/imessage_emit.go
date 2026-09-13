// SPDX-License-Identifier: MIT
//
// iMessage channel: emitInbound + scrubURLError (the bus publisher +
// the URL-query redactor used by the send helpers).
// Extracted from imessage.go during the Day-202 god-file split.
// Public API unchanged.
package imessage

import (
	"errors"
	"net/url"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
)

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.cfg.Bus == nil {
		return
	}
	_, _ = c.cfg.Bus.Publish(event.Spec{
		Subject:       "channel.inbound.imessage",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-imessage",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "imessage", "channel_id": msg.ChannelID,
			"sender": msg.Sender, "text": msg.Text, "allowed": allowed,
		},
	})
}

// scrubURLError redacts the query string from a *url.Error so the BlueBubbles
// password (passed as ?password=… per the BlueBubbles API) never reaches logs
// or surfaced errors. Transport errors from net/http are *url.Error values
// whose .URL carries the full request URL including the query.
func scrubURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		if u, perr := url.Parse(ue.URL); perr == nil {
			u.RawQuery = ""
			ue.URL = u.String()
		}
	}
	return err
}

