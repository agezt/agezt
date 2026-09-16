// SPDX-License-Identifier: MIT

// signal_helpers.go: send/getJSON/authorize/scrubToken/emit* split off from
// signal.go during the Day 211 god-file refactor (#129). Public API unchanged.
package signal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
)


// send POSTs /v2/send (chunked to the platform limit) and journals
// channel.outbound under corr.
func (c *Channel) send(ctx context.Context, out channel.Outbound, corr string) error {
	if strings.TrimSpace(out.Text) == "" {
		return nil // empty/whitespace is a no-op, not a failed send
	}
	for _, chunk := range channel.SplitText(out.Text, signalMaxChars) {
		body, _ := json.Marshal(map[string]any{
			"message":    chunk,
			"number":     c.number,
			"recipients": []string{out.ChannelID},
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v2/send", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		c.authorize(req)
		resp, err := c.client.Do(req)
		if err != nil {
			return c.scrubToken(err)
		}
		err = func() error {
			defer resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				return fmt.Errorf("signal send: status %d", resp.StatusCode)
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
			return nil
		}()
		if err != nil {
			return err
		}
	}
	c.emitOutbound(out, corr)
	return nil
}

// getJSON issues a GET and decodes a size-bounded JSON body.
func (c *Channel) getJSON(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	c.authorize(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return c.scrubToken(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, signalReceiveMaxBytes)).Decode(v)
}

// authorize attaches the optional bearer token. signal-cli-rest-api itself is
// unauthenticated; the token is for an operator's fronting reverse proxy.
func (c *Channel) authorize(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

// scrubToken removes the bearer token from an error message — defense in depth in
// case a transport error ever embeds it.
func (c *Channel) scrubToken(err error) error {
	if err == nil || c.token == "" {
		return err
	}
	if msg := err.Error(); strings.Contains(msg, c.token) {
		return errors.New(strings.ReplaceAll(msg, c.token, "<redacted>"))
	}
	return err
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.signal",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-signal",
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
		Subject:       "channel.outbound.signal",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-signal",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "signal",
			"channel_id":   out.ChannelID,
			"text":         out.Text,
			"priority":     string(out.Priority),
		},
	})
}
