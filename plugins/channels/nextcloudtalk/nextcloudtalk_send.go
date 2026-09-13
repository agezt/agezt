// SPDX-License-Identifier: MIT

package nextcloudtalk

// Nextcloud Talk outbound Send path: Send + send + emitInbound +
// emitOutbound. Carved out of nextcloudtalk.go during the Day 194
// god-file split so the main file can stay focused on types +
// lifecycle + inbound handling + verify/sign and the helpers file
// can stay focused on parseActivity + randomHex + sha256Sum +
// seenBefore.
// Public API unchanged.

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)

func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	token := strings.TrimSpace(out.ChannelID)
	text := strings.TrimSpace(out.Text)
	if token == "" {
		return fmt.Errorf("nextcloudtalk: send requires a conversation token")
	}
	if text == "" {
		return nil
	}
	return c.send(ctx, token, text, "chan-"+ulid.New())
}

func (c *Channel) send(ctx context.Context, token, text, corr string) error {
	if c.server == "" {
		return fmt.Errorf("nextcloudtalk: no ServerURL configured")
	}
	if c.secret == "" {
		return fmt.Errorf("nextcloudtalk: no secret configured")
	}
	endpoint := c.server + "/ocs/v2.php/apps/spreed/api/v1/bot/" + url.PathEscape(token) + "/message"
	for _, chunk := range channel.SplitText(text, maxChars) {
		random, err := randomHex()
		if err != nil {
			return err
		}
		sig := c.sign(random, []byte(chunk))
		form := url.Values{}
		form.Set("message", chunk)
		// A stable referenceId lets Nextcloud de-duplicate retried sends.
		form.Set("referenceId", hex.EncodeToString(sha256Sum(random+chunk)))
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("OCS-APIRequest", "true")
		req.Header.Set("X-Nextcloud-Talk-Bot-Random", random)
		req.Header.Set("X-Nextcloud-Talk-Bot-Signature", sig)
		resp, err := c.client.Do(req)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("nextcloudtalk: send returned status %d", resp.StatusCode)
		}
	}
	c.emitOutbound(channel.Outbound{ChannelID: token, Text: text, Priority: channel.PriorityNotify}, corr)
	return nil
}

// --- events ---------------------------------------------------------------

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.nextcloudtalk",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-nextcloudtalk",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": msg.ChannelKind,
			"channel_id":   msg.ChannelID,
			"sender":       msg.Sender,
			"text":         msg.Text,
			"allowed":      allowed,
		},
	})
}

func (c *Channel) emitOutbound(out channel.Outbound, corr string) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.outbound.nextcloudtalk",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-nextcloudtalk",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_id": out.ChannelID,
			"text":       out.Text,
			"priority":   string(out.Priority),
		},
	})
}

// --- wire shapes ----------------------------------------------------------

// parseActivity reads an Activity-Streams 2.0 bot event. Only "Create" message
// events are kept; the message text is pulled from object.content's JSON.
