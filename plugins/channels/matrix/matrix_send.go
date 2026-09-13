// SPDX-License-Identifier: MIT

// Matrix channel: media attachment uploader (sendMedia).
// Code extracted from matrix.go during the Day-111 god-file split.
// Public API unchanged.
package matrix


import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)

func (c *Channel) sendMedia(ctx context.Context, roomID string, att channel.Attachment) error {
	if len(att.Data) == 0 {
		return nil
	}
	fn := att.Filename
	if fn == "" {
		fn = "file"
	}
	// 1) upload bytes → mxc:// URI.
	up := fmt.Sprintf("%s/_matrix/media/v3/upload?filename=%s", c.base, url.QueryEscape(fn))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, up, bytes.NewReader(att.Data))
	if err != nil {
		return err
	}
	if att.MIME != "" {
		req.Header.Set("Content-Type", att.MIME)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return c.scrubToken(err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("matrix media upload: status %d", resp.StatusCode)
	}
	var ur struct {
		ContentURI string `json:"content_uri"`
	}
	if err := json.Unmarshal(body, &ur); err != nil || ur.ContentURI == "" {
		return fmt.Errorf("matrix media upload: no content_uri")
	}
	// 2) post the typed message event.
	msgtype := "m.file"
	switch att.Kind {
	case "audio":
		msgtype = "m.audio"
	case "image":
		msgtype = "m.image"
	}
	txn := ulid.New()
	endpoint := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
		c.base, url.PathEscape(roomID), url.PathEscape(txn))
	ev, _ := json.Marshal(map[string]any{
		"msgtype": msgtype,
		"body":    fn,
		"url":     ur.ContentURI,
		"info":    map[string]any{"mimetype": att.MIME, "size": len(att.Data)},
	})
	ereq, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(ev))
	if err != nil {
		return err
	}
	ereq.Header.Set("Content-Type", "application/json")
	ereq.Header.Set("Authorization", "Bearer "+c.token)
	eresp, err := c.client.Do(ereq)
	if err != nil {
		return c.scrubToken(err)
	}
	defer eresp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(eresp.Body, 4<<10))
	if eresp.StatusCode/100 != 2 {
		return fmt.Errorf("matrix send media event: status %d", eresp.StatusCode)
	}
	return nil
}

// getJSON issues an authenticated GET and decodes a size-bounded JSON body.
func (c *Channel) getJSON(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return c.scrubToken(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, matrixSyncMaxBytes)).Decode(v)
}

// scrubToken removes the access token from an error message — defense in depth in
// case a transport error ever embeds it (the token rides an Authorization header,
// not the URL, but redaction is cheap and the cost of a leaked token is not).
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
		Subject:       "channel.inbound.matrix",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-matrix",
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
		Subject:       "channel.outbound.matrix",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-matrix",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "matrix",
			"channel_id":   out.ChannelID,
			"text":         out.Text,
			"priority":     string(out.Priority),
		},
	})
}


// sendMedia uploads an attachment to the media repo, then posts an m.audio /
// m.image / m.file room event referencing the returned mxc:// URI.
// Extracted from matrix.go during the Day-208 god-file split.
// Public API unchanged.

// Send implements channel.Channel (used by the Pulse→Matrix sink and any
// out-of-band sender). Journaled under no correlation.
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	return c.send(ctx, out, "")
}

// send PUTs m.room.message (chunked to the platform limit) and journals
// channel.outbound under corr.
func (c *Channel) send(ctx context.Context, out channel.Outbound, corr string) error {
	if strings.TrimSpace(out.Text) == "" && len(out.Attachments) == 0 {
		return nil // empty/whitespace is a no-op, not a failed send
	}
	for _, chunk := range channel.SplitText(out.Text, matrixMaxChars) {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		// A fresh transaction id per chunk makes the PUT idempotent (a retried
		// delivery with the same txn id is deduplicated by the homeserver).
		txn := ulid.New()
		endpoint := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
			c.base, url.PathEscape(out.ChannelID), url.PathEscape(txn))
		body, _ := json.Marshal(map[string]any{"msgtype": "m.text", "body": chunk})
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.token)
		resp, err := c.client.Do(req)
		if err != nil {
			return c.scrubToken(err)
		}
		err = func() error {
			defer resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				return fmt.Errorf("matrix send: status %d", resp.StatusCode)
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
			return nil
		}()
		if err != nil {
			return err
		}
	}
	for _, att := range out.Attachments {
		if err := c.sendMedia(ctx, out.ChannelID, att); err != nil {
			return err
		}
	}
	c.emitOutbound(out, corr)
	return nil
}

// fetchMXC downloads an mxc:// content URI from the media repo and returns it as
// an inline data: URL. Best-effort: returns "" on any failure. The mxc form is
// mxc://<server>/<mediaId>.
func (c *Channel) fetchMXC(ctx context.Context, mxc, mime string) string {
	rest := strings.TrimPrefix(mxc, "mxc://")
	server, mediaID, ok := strings.Cut(rest, "/")
	if !ok || server == "" || mediaID == "" {
		return ""
	}
	endpoint := fmt.Sprintf("%s/_matrix/media/v3/download/%s/%s",
		c.base, url.PathEscape(server), url.PathEscape(mediaID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20+1))
	if err != nil || len(data) == 0 || len(data) > 16<<20 {
		return ""
	}
	if mime == "" {
		mime = resp.Header.Get("Content-Type")
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// sendMedia uploads an attachment to the media repo, then posts an m.audio /
// m.image / m.file room event referencing the returned mxc:// URI.
